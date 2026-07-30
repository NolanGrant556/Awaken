import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  AgentPlanAccountWire,
  AgentPlanMediaCapability,
  AgentPlanMediaDisplayState,
  AgentPlanMediaModelKind,
  AgentPlanMediaToolConfigurationWire,
  AgentPlanMediaToolSaveRequest,
  AgentPlanMediaToolsWire,
} from "../shared/runtime.js";
import { VOLCENGINE_AGENT_PLAN_ACCOUNT_ID } from "../shared/runtime.js";
import { AgentPlanAccountStore } from "./agent-plan-account-store.js";

const FILE_NAME = "agent-plan-media-tools.json";
const PROVIDER_ID = "volcengine-agent-plan" as const;

type StoredConfiguration = {
  id: string;
  providerId: typeof PROVIDER_ID;
  accountId: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  capability: AgentPlanMediaCapability;
  modelId: string;
  modelKind: AgentPlanMediaModelKind;
  enabled: boolean;
  configurationRevision: number;
};

type StoreDocument = {
  version: 1;
  configurations: StoredConfiguration[];
};

function modelKindForCapability(
  capability: AgentPlanMediaCapability,
): AgentPlanMediaModelKind {
  return capability === "media.image.generate" ? "image" : "video";
}

function configurationId(capability: AgentPlanMediaCapability): string {
  return capability === "media.image.generate"
    ? "volcengine-agent-plan-image"
    : "volcengine-agent-plan-video";
}

function normalizeConfiguration(
  value: unknown,
): StoredConfiguration | undefined {
  if (!value || typeof value !== "object") return undefined;
  const candidate = value as Partial<StoredConfiguration>;
  if (
    candidate.capability !== "media.image.generate" &&
    candidate.capability !== "media.video.generate"
  ) {
    return undefined;
  }
  const modelId =
    typeof candidate.modelId === "string" ? candidate.modelId.trim() : "";
  if (!modelId) return undefined;
  const modelKind = modelKindForCapability(candidate.capability);
  if (candidate.modelKind !== undefined && candidate.modelKind !== modelKind) {
    return undefined;
  }
  return {
    id: configurationId(candidate.capability),
    providerId: PROVIDER_ID,
    accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
    capability: candidate.capability,
    modelId,
    modelKind,
    enabled: candidate.enabled !== false,
    configurationRevision:
      typeof candidate.configurationRevision === "number" &&
      candidate.configurationRevision >= 1
        ? Math.floor(candidate.configurationRevision)
        : 1,
  };
}

function normalizeDocument(value: unknown): StoreDocument {
  const configurations =
    value &&
    typeof value === "object" &&
    Array.isArray((value as StoreDocument).configurations)
      ? (value as StoreDocument).configurations
          .map(normalizeConfiguration)
          .filter((item): item is StoredConfiguration => item !== undefined)
      : [];
  const byCapability = new Map<AgentPlanMediaCapability, StoredConfiguration>();
  for (const configuration of configurations) {
    byCapability.set(configuration.capability, configuration);
  }
  return { version: 1, configurations: [...byCapability.values()] };
}

function displayState(
  configuration: StoredConfiguration,
  account: AgentPlanAccountWire,
): AgentPlanMediaDisplayState {
  if (!configuration.enabled) return "disabled";
  if (!account.has_key) return "needs_auth";
  const catalogEntry = account.catalog.find(
    (entry) => entry.id === configuration.modelId,
  );
  if (!catalogEntry || catalogEntry.kind !== configuration.modelKind)
    return "unavailable";
  if (!account.catalog_updated_at) return "unknown";
  return "ready_unverified";
}

export class AgentPlanMediaToolsStore {
  private readonly filePath: string;
  private mutationQueue: Promise<void> = Promise.resolve();

  constructor(
    awakenHome: string,
    private readonly accountStore: AgentPlanAccountStore,
  ) {
    this.filePath = path.join(awakenHome, "config", FILE_NAME);
  }

  async snapshot(): Promise<AgentPlanMediaToolsWire> {
    await this.mutationQueue;
    const [document, account] = await Promise.all([
      this.readDocument(),
      this.accountStore.getAccount(),
    ]);
    return this.toWire(document, account);
  }

  async save(
    request: AgentPlanMediaToolSaveRequest,
  ): Promise<AgentPlanMediaToolsWire> {
    return this.withMutation(async () => {
      const modelId = request.modelId.trim();
      if (!modelId) throw new Error("请选择一个图片或视频生成模型。");
      const account = await this.accountStore.getAccount();
      if (!account.has_key)
        throw new Error("请先保存火山 Agent Plan API key。");
      const modelKind = modelKindForCapability(request.capability);
      const document = await this.readDocument();
      const existing = document.configurations.find(
        (configuration) => configuration.capability === request.capability,
      );
      const catalogEntry = account.catalog.find(
        (entry) => entry.id === modelId,
      );
      const disablingExisting =
        request.enabled === false && existing?.modelId === modelId;
      if (
        !disablingExisting &&
        (!catalogEntry || catalogEntry.kind !== modelKind)
      ) {
        throw new Error(
          modelKind === "image"
            ? "所选模型不在当前 Agent Plan 图片模型目录中。"
            : "所选模型不在当前 Agent Plan 视频模型目录中。",
        );
      }
      const enabled = request.enabled !== false;
      const unchanged =
        existing?.modelId === modelId && existing.enabled === enabled;
      const next: StoredConfiguration = {
        id: configurationId(request.capability),
        providerId: PROVIDER_ID,
        accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
        capability: request.capability,
        modelId,
        modelKind,
        enabled,
        configurationRevision: unchanged
          ? existing.configurationRevision
          : (existing?.configurationRevision ?? 0) + 1,
      };
      const configurations = document.configurations.filter(
        (configuration) => configuration.capability !== request.capability,
      );
      configurations.push(next);
      const nextDocument: StoreDocument = { version: 1, configurations };
      if (!unchanged) await this.writeDocument(nextDocument);
      return this.toWire(nextDocument, account);
    });
  }

  async delete(
    capability: AgentPlanMediaCapability,
  ): Promise<AgentPlanMediaToolsWire> {
    return this.withMutation(async () => {
      const document = await this.readDocument();
      const configurations = document.configurations.filter(
        (configuration) => configuration.capability !== capability,
      );
      if (configurations.length !== document.configurations.length) {
        await this.writeDocument({ version: 1, configurations });
      }
      return this.toWire(
        { version: 1, configurations },
        await this.accountStore.getAccount(),
      );
    });
  }

  async hasAccountReferences(): Promise<boolean> {
    await this.mutationQueue;
    return (await this.readDocument()).configurations.length > 0;
  }

  private toWire(
    document: StoreDocument,
    account: AgentPlanAccountWire,
  ): AgentPlanMediaToolsWire {
    return {
      configurations: document.configurations.map(
        (configuration): AgentPlanMediaToolConfigurationWire => ({
          id: configuration.id,
          provider_id: configuration.providerId,
          account_id: configuration.accountId,
          capability: configuration.capability,
          model_id: configuration.modelId,
          model_kind: configuration.modelKind,
          enabled: configuration.enabled,
          configuration_revision: configuration.configurationRevision,
          credential_revision: account.credential_revision,
          display_state: displayState(configuration, account),
        }),
      ),
    };
  }

  private async readDocument(): Promise<StoreDocument> {
    try {
      return normalizeDocument(
        JSON.parse(await fs.readFile(this.filePath, "utf8")),
      );
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        return { version: 1, configurations: [] };
      }
      throw new Error(
        "无法读取 ~/.awaken/config/agent-plan-media-tools.json。",
        {
          cause: error,
        },
      );
    }
  }

  private async writeDocument(document: StoreDocument): Promise<void> {
    const directory = path.dirname(this.filePath);
    const tempPath = path.join(
      directory,
      `.${FILE_NAME}.${process.pid}.${randomUUID()}.tmp`,
    );
    try {
      await fs.mkdir(directory, { recursive: true, mode: 0o700 });
      await fs.writeFile(tempPath, `${JSON.stringify(document, null, 2)}\n`, {
        encoding: "utf8",
        mode: 0o600,
      });
      await fs.rename(tempPath, this.filePath);
      await fs.chmod(this.filePath, 0o600);
    } finally {
      await fs.rm(tempPath, { force: true }).catch(() => undefined);
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
