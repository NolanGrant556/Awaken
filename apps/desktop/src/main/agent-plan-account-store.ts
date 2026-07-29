import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  AgentPlanAccountSaveRequest,
  AgentPlanAccountWire,
} from "../shared/runtime.js";
import { VOLCENGINE_AGENT_PLAN_ACCOUNT_ID } from "../shared/runtime.js";
import { AwakenEnvStore } from "./awaken-env-store.js";

const ACCOUNT_FILE = "agent-plan-account.json";
const PROVIDER_ID = "volcengine-agent-plan" as const;
const AGENT_PLAN_BASE_URL = "https://ark.cn-beijing.volces.com/api/plan/v3";
const FALLBACK_MODEL_ID = "ark-code-latest";
const CATALOG_TIMEOUT_MS = 8_000;

type AccountDocument = {
  version: 1;
  accountId: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  providerId: typeof PROVIDER_ID;
  keyLast4?: string;
  credentialRevision: number;
  modelIds: string[];
  catalogUpdatedAt?: string;
};

export interface AgentPlanKeyStore {
  getAgentPlanKey(accountId: string): Promise<string>;
  setAgentPlanKey(accountId: string, value: string): Promise<void>;
  deleteAgentPlanKey(accountId: string): Promise<void>;
}

export type AgentPlanAccountStoreErrorCode = "unreadable" | "write_failed";

export class AgentPlanAccountStoreError extends Error {
  readonly code: AgentPlanAccountStoreErrorCode;

  constructor(
    code: AgentPlanAccountStoreErrorCode,
    message: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "AgentPlanAccountStoreError";
    this.code = code;
  }
}

type AccountSnapshot = {
  rawDocument?: string;
  apiKey: string;
};

function emptyDocument(): AccountDocument {
  return {
    version: 1,
    accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    providerId: PROVIDER_ID,
    credentialRevision: 1,
    modelIds: [FALLBACK_MODEL_ID],
  };
}

function normalizeDocument(value: unknown): AccountDocument {
  if (!value || typeof value !== "object") return emptyDocument();
  const document = value as Partial<AccountDocument>;
  const modelIds = Array.isArray(document.modelIds)
    ? document.modelIds.filter((model): model is string => typeof model === "string" && model.trim() !== "")
    : [];
  return {
    version: 1,
    accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    providerId: PROVIDER_ID,
    keyLast4:
      typeof document.keyLast4 === "string" ? document.keyLast4.slice(-4) : undefined,
    credentialRevision:
      typeof document.credentialRevision === "number" && document.credentialRevision >= 1
        ? Math.floor(document.credentialRevision)
        : 1,
    modelIds: Array.from(new Set([FALLBACK_MODEL_ID, ...modelIds])),
    catalogUpdatedAt:
      typeof document.catalogUpdatedAt === "string" ? document.catalogUpdatedAt : undefined,
  };
}

function maskKey(last4?: string): string | undefined {
  return last4 ? `•••• ${last4}` : undefined;
}

export class AgentPlanAccountStore {
  private readonly filePath: string;
  private mutationQueue: Promise<void> = Promise.resolve();

  constructor(
    awakenHome: string,
    private readonly keyStore: AgentPlanKeyStore = new AwakenEnvStore(awakenHome),
  ) {
    this.filePath = path.join(awakenHome, "config", ACCOUNT_FILE);
  }

  async getAccount(): Promise<AgentPlanAccountWire> {
    await this.mutationQueue;
    const document = await this.readDocument();
    const key = await this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
    return this.toWire(document, Boolean(key));
  }

  async saveAccount(request: AgentPlanAccountSaveRequest): Promise<AgentPlanAccountWire> {
    return this.withMutation(() => this.saveAccountUnlocked(request));
  }

  async saveAccountWithCompensation(
    request: AgentPlanAccountSaveRequest,
    operation: () => Promise<void>,
  ): Promise<AgentPlanAccountWire> {
    return this.withMutation(async () => {
      const snapshot = await this.captureSnapshot();
      try {
        const account = await this.saveAccountUnlocked(request);
        await operation();
        return account;
      } catch (error) {
        try {
          await this.restoreSnapshot(snapshot);
        } catch (rollbackError) {
          throw new AgentPlanAccountStoreError(
            "write_failed",
            "Agent Plan 账户与模型配置保存失败，且旧凭据状态无法完整恢复。",
            { cause: new AggregateError([error, rollbackError]) },
          );
        }
        throw error;
      }
    });
  }

  async refreshModelCatalog(): Promise<AgentPlanAccountWire> {
    return this.withMutation(async () => {
      const document = await this.readDocument();
      const key = await this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
      if (!key) throw new Error("请先保存火山 Agent Plan API key。");
      return this.refreshModelCatalogUnlocked(document, key);
    });
  }

  async resolveKey(): Promise<{ apiKey: string; credentialRevision: number }> {
    await this.mutationQueue;
    const [document, apiKey] = await Promise.all([
      this.readDocument(),
      this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID),
    ]);
    return { apiKey, credentialRevision: document.credentialRevision };
  }

  async deleteAccount(isReferenced: boolean): Promise<void> {
    if (isReferenced) throw new Error("Agent Plan 账户仍被模型配置引用，无法删除。");
    await this.withMutation(async () => {
      const previousKey = await this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
      await this.keyStore.deleteAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
      try {
        await fs.rm(this.filePath, { force: true });
      } catch (error) {
        if (previousKey) {
          await this.keyStore.setAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID, previousKey);
        }
        throw error;
      }
    });
  }

  private async refreshModelCatalogUnlocked(
    document: AccountDocument,
    apiKey: string,
  ): Promise<AgentPlanAccountWire> {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), CATALOG_TIMEOUT_MS);
    try {
      const response = await fetch(`${AGENT_PLAN_BASE_URL}/models`, {
        headers: { Authorization: `Bearer ${apiKey}` },
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`Agent Plan 模型目录刷新失败：HTTP ${response.status}`);
      const payload = (await response.json()) as { data?: Array<{ id?: unknown }> };
      const exactIds = (payload.data ?? [])
        .map((item) => (typeof item.id === "string" ? item.id.trim() : ""))
        .filter(Boolean);
      const nextDocument: AccountDocument = {
        ...document,
        modelIds: Array.from(new Set([FALLBACK_MODEL_ID, ...exactIds])),
        catalogUpdatedAt: new Date().toISOString(),
      };
      await this.writeDocument(nextDocument);
      return this.toWire(nextDocument, true);
    } finally {
      clearTimeout(timeout);
    }
  }

  private async saveAccountUnlocked(
    request: AgentPlanAccountSaveRequest,
  ): Promise<AgentPlanAccountWire> {
    const previousDocument = await this.readDocument();
    const previousKey = await this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
    const nextKey = request.apiKey?.trim() || previousKey;
    if (!nextKey) throw new Error("请填写火山 Agent Plan API key。");
    const rotated = previousKey !== "" && previousKey !== nextKey;
    const nextDocument: AccountDocument = {
      ...previousDocument,
      keyLast4: nextKey.slice(-4),
      credentialRevision: rotated
        ? previousDocument.credentialRevision + 1
        : previousDocument.credentialRevision,
    };
    await this.keyStore.setAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID, nextKey);
    try {
      await this.writeDocument(nextDocument);
    } catch (error) {
      await this.restoreKey(previousKey);
      throw error;
    }
    try {
      return await this.refreshModelCatalogUnlocked(nextDocument, nextKey);
    } catch {
      return this.toWire(nextDocument, true);
    }
  }

  private toWire(document: AccountDocument, hasKey: boolean): AgentPlanAccountWire {
    return {
      account_id: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      provider_id: PROVIDER_ID,
      has_key: hasKey,
      masked_key: hasKey ? maskKey(document.keyLast4) : undefined,
      credential_revision: document.credentialRevision,
      model_ids: [...document.modelIds],
      catalog_updated_at: document.catalogUpdatedAt,
    };
  }

  private async readDocument(): Promise<AccountDocument> {
    return (await this.readOptionalDocument()) ?? emptyDocument();
  }

  private async readOptionalDocument(): Promise<AccountDocument | undefined> {
    return (await this.readOptionalDocumentFile())?.document;
  }

  private async readOptionalDocumentFile(): Promise<
    { document: AccountDocument; raw: string } | undefined
  > {
    try {
      const raw = await fs.readFile(this.filePath, "utf8");
      return { document: normalizeDocument(JSON.parse(raw)), raw };
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") return undefined;
      throw new AgentPlanAccountStoreError(
        "unreadable",
        "无法读取 ~/.awaken/config/agent-plan-account.json 中的 Agent Plan 凭据元数据。",
        { cause: error },
      );
    }
  }

  private async writeDocument(document: AccountDocument): Promise<void> {
    await this.writeDocumentRaw(`${JSON.stringify(document, null, 2)}\n`);
  }

  private async writeDocumentRaw(raw: string): Promise<void> {
    const directory = path.dirname(this.filePath);
    const tempPath = path.join(directory, `.${ACCOUNT_FILE}.${process.pid}.${randomUUID()}.tmp`);
    try {
      await fs.mkdir(directory, { recursive: true, mode: 0o700 });
      await fs.writeFile(tempPath, raw, {
        encoding: "utf8",
        mode: 0o600,
      });
      await fs.rename(tempPath, this.filePath);
      await fs.chmod(this.filePath, 0o600);
    } catch (error) {
      throw new AgentPlanAccountStoreError(
        "write_failed",
        "无法写入 ~/.awaken/config/agent-plan-account.json。",
        { cause: error },
      );
    } finally {
      await fs.rm(tempPath, { force: true }).catch(() => undefined);
    }
  }

  private async captureSnapshot(): Promise<AccountSnapshot> {
    const [file, apiKey] = await Promise.all([
      this.readOptionalDocumentFile(),
      this.keyStore.getAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID),
    ]);
    return { rawDocument: file?.raw, apiKey };
  }

  private async restoreSnapshot(snapshot: AccountSnapshot): Promise<void> {
    const failures: unknown[] = [];
    try {
      await this.restoreKey(snapshot.apiKey);
    } catch (error) {
      failures.push(error);
    }
    try {
      if (snapshot.rawDocument !== undefined) {
        await this.writeDocumentRaw(snapshot.rawDocument);
      } else {
        await fs.rm(this.filePath, { force: true });
      }
    } catch (error) {
      failures.push(error);
    }
    if (failures.length > 0) {
      throw new AggregateError(failures, "Agent Plan account rollback failed");
    }
  }

  private async restoreKey(apiKey: string): Promise<void> {
    if (apiKey) {
      await this.keyStore.setAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID, apiKey);
    } else {
      await this.keyStore.deleteAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
    }
  }

  private async withMutation<T>(operation: () => Promise<T>): Promise<T> {
    const previous = this.mutationQueue;
    let release: (() => void) | undefined;
    this.mutationQueue = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    try {
      return await operation();
    } finally {
      release?.();
    }
  }
}
