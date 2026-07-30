import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  AgentPlanAccountSaveRequest,
  AgentPlanCatalogEntryWire,
  AgentPlanCatalogKind,
  AgentPlanAccountWire,
} from "../shared/runtime.js";
import { VOLCENGINE_AGENT_PLAN_ACCOUNT_ID } from "../shared/runtime.js";
import { AwakenEnvStore } from "./awaken-env-store.js";

const ACCOUNT_FILE = "agent-plan-account.json";
const PROVIDER_ID = "volcengine-agent-plan" as const;
const AGENT_PLAN_BASE_URL = "https://ark.cn-beijing.volces.com/api/plan/v3";
const FALLBACK_MODEL_ID = "ark-code-latest";
const BUILTIN_CATALOG_REVISION = 1;
const CATALOG_TIMEOUT_MS = 8_000;

const BUILTIN_AGENT_PLAN_CATALOG: AgentPlanCatalogEntryWire[] = [
  {
    id: FALLBACK_MODEL_ID,
    kind: "llm",
    display_name: "Agent Plan 智能路由",
  },
  {
    id: "doubao-seedream-5.0-lite",
    kind: "image",
    display_name: "Doubao Seedream 5.0 Lite",
  },
  {
    id: "doubao-seedance-2.0",
    kind: "video",
    display_name: "Doubao Seedance 2.0",
  },
  {
    id: "doubao-seedance-2.0-fast",
    kind: "video",
    display_name: "Doubao Seedance 2.0 Fast",
  },
  {
    id: "doubao-seedance-2.0-mini",
    kind: "video",
    display_name: "Doubao Seedance 2.0 Mini",
  },
  {
    id: "doubao-seedance-1.5-pro",
    kind: "video",
    display_name: "Doubao Seedance 1.5 Pro",
  },
];

type AccountDocument = {
  version: 2;
  accountId: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  providerId: typeof PROVIDER_ID;
  builtinCatalogRevision: number;
  keyLast4?: string;
  credentialRevision: number;
  modelIds: string[];
  catalog: AgentPlanCatalogEntryWire[];
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
    version: 2,
    accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    providerId: PROVIDER_ID,
    builtinCatalogRevision: BUILTIN_CATALOG_REVISION,
    credentialRevision: 1,
    modelIds: BUILTIN_AGENT_PLAN_CATALOG.map((entry) => entry.id),
    catalog: BUILTIN_AGENT_PLAN_CATALOG.map((entry) => ({ ...entry })),
  };
}

const MEDIA_IMAGE_ID = /(?:^|[-_.])(seedream|seededit)(?:[-_.]|$)/i;
const MEDIA_VIDEO_ID = /(?:^|[-_.])seedance(?:[-_.]|$)/i;
const UNSUPPORTED_ID =
  /(?:^|[-_.])(embedding|embed|rerank|speech|audio|tts|asr|music|3d)(?:[-_.]|$)/i;
const LLM_ID =
  /^(?:ark-code-latest|doubao-|deepseek-|glm-|kimi-|moonshot-|minimax-|qwen)/i;

function normalizedCatalogKind(
  value: unknown,
): AgentPlanCatalogKind | undefined {
  if (typeof value !== "string") return undefined;
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[\s-]+/g, "_");
  if (
    ["llm", "chat", "text", "chat_completion", "text_generation"].includes(
      normalized,
    )
  ) {
    return "llm";
  }
  if (["image", "image_generation", "text_to_image"].includes(normalized)) {
    return "image";
  }
  if (
    ["video", "video_generation", "text_to_video", "image_to_video"].includes(
      normalized,
    )
  ) {
    return "video";
  }
  if (
    [
      "embedding",
      "rerank",
      "audio",
      "speech",
      "tts",
      "asr",
      "music",
      "3d",
    ].includes(normalized)
  ) {
    return "unsupported";
  }
  return undefined;
}

export function classifyAgentPlanCatalogEntry(
  item: Record<string, unknown>,
): AgentPlanCatalogEntryWire | undefined {
  const id = typeof item.id === "string" ? item.id.trim() : "";
  if (!id) return undefined;

  const declaredKind = [
    item.kind,
    item.type,
    item.model_type,
    item.task_type,
    item.modality,
  ]
    .map(normalizedCatalogKind)
    .find((kind): kind is AgentPlanCatalogKind => kind !== undefined);
  let kind: AgentPlanCatalogKind;
  if (declaredKind) {
    kind = declaredKind;
  } else if (MEDIA_IMAGE_ID.test(id)) {
    kind = "image";
  } else if (MEDIA_VIDEO_ID.test(id)) {
    kind = "video";
  } else if (UNSUPPORTED_ID.test(id)) {
    kind = "unsupported";
  } else if (LLM_ID.test(id)) {
    kind = "llm";
  } else {
    kind = "unknown";
  }
  const displayName = [item.display_name, item.name]
    .find(
      (value): value is string =>
        typeof value === "string" && value.trim() !== "",
    )
    ?.trim();
  return { id, kind, display_name: displayName };
}

function normalizedCatalog(
  catalogValue: unknown,
  legacyModelIds: string[],
): AgentPlanCatalogEntryWire[] {
  const byId = new Map<string, AgentPlanCatalogEntryWire>();
  for (const entry of BUILTIN_AGENT_PLAN_CATALOG) {
    byId.set(entry.id, { ...entry });
  }
  if (Array.isArray(catalogValue)) {
    for (const value of catalogValue) {
      if (!value || typeof value !== "object") continue;
      const entry = classifyAgentPlanCatalogEntry(
        value as Record<string, unknown>,
      );
      if (entry && !byId.has(entry.id)) byId.set(entry.id, entry);
    }
  }
  for (const id of legacyModelIds) {
    if (byId.has(id)) continue;
    const entry = classifyAgentPlanCatalogEntry({ id });
    if (entry) byId.set(id, entry);
  }
  return [...byId.values()];
}

function normalizeDocument(value: unknown): AccountDocument {
  if (!value || typeof value !== "object") return emptyDocument();
  const document = value as Partial<AccountDocument>;
  const modelIds = Array.isArray(document.modelIds)
    ? document.modelIds.filter(
        (model): model is string =>
          typeof model === "string" && model.trim() !== "",
      )
    : [];
  const catalog = normalizedCatalog(document.catalog, modelIds);
  return {
    version: 2,
    accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    providerId: PROVIDER_ID,
    builtinCatalogRevision:
      typeof document.builtinCatalogRevision === "number" &&
      document.builtinCatalogRevision >= 0
        ? Math.floor(document.builtinCatalogRevision)
        : 0,
    keyLast4:
      typeof document.keyLast4 === "string"
        ? document.keyLast4.slice(-4)
        : undefined,
    credentialRevision:
      typeof document.credentialRevision === "number" &&
      document.credentialRevision >= 1
        ? Math.floor(document.credentialRevision)
        : 1,
    modelIds: catalog.map((entry) => entry.id),
    catalog,
    catalogUpdatedAt:
      typeof document.catalogUpdatedAt === "string"
        ? document.catalogUpdatedAt
        : undefined,
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
    private readonly keyStore: AgentPlanKeyStore = new AwakenEnvStore(
      awakenHome,
    ),
  ) {
    this.filePath = path.join(awakenHome, "config", ACCOUNT_FILE);
  }

  async getAccount(): Promise<AgentPlanAccountWire> {
    return this.withMutation(async () => {
      let document = await this.readDocument();
      if (document.builtinCatalogRevision < BUILTIN_CATALOG_REVISION) {
        document = {
          ...document,
          builtinCatalogRevision: BUILTIN_CATALOG_REVISION,
        };
        await this.writeDocument(document);
      }
      const key = await this.keyStore.getAgentPlanKey(
        VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      );
      return this.toWire(document, Boolean(key));
    });
  }

  async saveAccount(
    request: AgentPlanAccountSaveRequest,
  ): Promise<AgentPlanAccountWire> {
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
      const key = await this.keyStore.getAgentPlanKey(
        VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      );
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
    if (isReferenced)
      throw new Error(
        "Agent Plan 账户仍被模型配置或生成工具配置引用，无法删除。",
      );
    await this.withMutation(async () => {
      const previousKey = await this.keyStore.getAgentPlanKey(
        VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      );
      await this.keyStore.deleteAgentPlanKey(VOLCENGINE_AGENT_PLAN_ACCOUNT_ID);
      try {
        await fs.rm(this.filePath, { force: true });
      } catch (error) {
        if (previousKey) {
          await this.keyStore.setAgentPlanKey(
            VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
            previousKey,
          );
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
      if (response.status === 404) {
        const nextDocument: AccountDocument = {
          ...document,
          builtinCatalogRevision: BUILTIN_CATALOG_REVISION,
          modelIds: document.catalog.map((entry) => entry.id),
          catalogUpdatedAt: new Date().toISOString(),
        };
        await this.writeDocument(nextDocument);
        return this.toWire(nextDocument, true);
      }
      if (!response.ok)
        throw new Error(`Agent Plan 模型目录刷新失败：HTTP ${response.status}`);
      const payload = (await response.json()) as { data?: unknown[] };
      const catalog = (payload.data ?? [])
        .map((item) =>
          item && typeof item === "object"
            ? classifyAgentPlanCatalogEntry(item as Record<string, unknown>)
            : undefined,
        )
        .filter(
          (item): item is AgentPlanCatalogEntryWire => item !== undefined,
        );
      const exactIds = catalog.map((item) => item.id);
      const nextDocument: AccountDocument = {
        ...document,
        builtinCatalogRevision: BUILTIN_CATALOG_REVISION,
        modelIds: normalizedCatalog(catalog, exactIds).map((entry) => entry.id),
        catalog: normalizedCatalog(catalog, exactIds),
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
    const previousKey = await this.keyStore.getAgentPlanKey(
      VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    );
    const nextKey = request.apiKey?.trim() || previousKey;
    if (!nextKey) throw new Error("请填写火山 Agent Plan API key。");
    const rotated = previousKey !== "" && previousKey !== nextKey;
    const nextDocument: AccountDocument = {
      ...previousDocument,
      builtinCatalogRevision: BUILTIN_CATALOG_REVISION,
      keyLast4: nextKey.slice(-4),
      credentialRevision: rotated
        ? previousDocument.credentialRevision + 1
        : previousDocument.credentialRevision,
    };
    await this.keyStore.setAgentPlanKey(
      VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      nextKey,
    );
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

  private toWire(
    document: AccountDocument,
    hasKey: boolean,
  ): AgentPlanAccountWire {
    return {
      account_id: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
      provider_id: PROVIDER_ID,
      has_key: hasKey,
      masked_key: hasKey ? maskKey(document.keyLast4) : undefined,
      credential_revision: document.credentialRevision,
      model_ids: [...document.modelIds],
      catalog: document.catalog.map((entry) => ({ ...entry })),
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
    const tempPath = path.join(
      directory,
      `.${ACCOUNT_FILE}.${process.pid}.${randomUUID()}.tmp`,
    );
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
      await this.keyStore.setAgentPlanKey(
        VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
        apiKey,
      );
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
