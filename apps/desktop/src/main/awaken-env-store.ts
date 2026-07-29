import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";

const PROFILE_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
const ENV_KEY_PREFIX = "AWAKEN_API_KEY_";
const AGENT_PLAN_ENV_KEY_PREFIX = "AWAKEN_AGENT_PLAN_API_KEY_";
const mutationQueuesByPath = new Map<string, Promise<void>>();

export type AwakenEnvStoreErrorCode =
  | "invalid_profile_id"
  | "unreadable"
  | "write_failed";

export class AwakenEnvStoreError extends Error {
  readonly code: AwakenEnvStoreErrorCode;

  constructor(code: AwakenEnvStoreErrorCode, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "AwakenEnvStoreError";
    this.code = code;
  }
}

function keyForProfile(profileId: string): string {
  const normalized = profileId.trim();
  if (!PROFILE_ID_PATTERN.test(normalized)) {
    throw new AwakenEnvStoreError(
      "invalid_profile_id",
      "模型配置 ID 无效，无法访问 API key。",
    );
  }
  return `${ENV_KEY_PREFIX}${normalized}`;
}

function keyForAgentPlanAccount(accountId: string): string {
  const normalized = accountId.trim();
  if (!PROFILE_ID_PATTERN.test(normalized)) {
    throw new AwakenEnvStoreError(
      "invalid_profile_id",
      "Agent Plan 账户 ID 无效，无法访问 API key。",
    );
  }
  return `${AGENT_PLAN_ENV_KEY_PREFIX}${normalized}`;
}

function decodeValue(raw: string): string {
  const trimmed = raw.trim();
  if (trimmed.startsWith('"')) {
    const parsed = JSON.parse(trimmed) as unknown;
    if (typeof parsed !== "string") {
      throw new SyntaxError("quoted .env value must be a string");
    }
    return parsed;
  }
  if (trimmed.startsWith("'") && trimmed.endsWith("'")) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

function parseEnv(raw: string): Map<string, string> {
  const entries = new Map<string, string>();
  for (const line of raw.split(/\r?\n/u)) {
    const trimmed = line.trim();
    if (trimmed === "" || trimmed.startsWith("#")) continue;
    const normalized = trimmed.startsWith("export ") ? trimmed.slice(7).trim() : trimmed;
    const separator = normalized.indexOf("=");
    if (separator <= 0) {
      throw new SyntaxError("invalid .env assignment");
    }
    const key = normalized.slice(0, separator).trim();
    if (!/^[A-Za-z_][A-Za-z0-9._-]*$/u.test(key)) {
      throw new SyntaxError("invalid .env key");
    }
    entries.set(key, decodeValue(normalized.slice(separator + 1)));
  }
  return entries;
}

function serializeEnv(entries: Map<string, string>): string {
  const lines = Array.from(entries.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${JSON.stringify(value)}`);
  return lines.length > 0 ? `${lines.join("\n")}\n` : "";
}

export class AwakenEnvStore {
  private readonly filePath: string;

  constructor(awakenHome: string) {
    this.filePath = path.resolve(awakenHome, ".env");
  }

  async getKey(profileId: string): Promise<string> {
    const key = keyForProfile(profileId);
    return this.getValue(key);
  }

  async setKey(profileId: string, value: string): Promise<void> {
    const key = keyForProfile(profileId);
    await this.withMutation(async () => {
      const entries = await this.readEntries();
      entries.set(key, value);
      await this.writeEntries(entries);
    });
  }

  async deleteKey(profileId: string): Promise<void> {
    const key = keyForProfile(profileId);
    await this.withMutation(async () => {
      const entries = await this.readEntries();
      if (!entries.delete(key)) return;
      await this.writeEntries(entries);
    });
  }

  async getAgentPlanKey(accountId: string): Promise<string> {
    return this.getValue(keyForAgentPlanAccount(accountId));
  }

  async setAgentPlanKey(accountId: string, value: string): Promise<void> {
    return this.setValue(keyForAgentPlanAccount(accountId), value);
  }

  async deleteAgentPlanKey(accountId: string): Promise<void> {
    return this.deleteValue(keyForAgentPlanAccount(accountId));
  }

  private async getValue(key: string): Promise<string> {
    await (mutationQueuesByPath.get(this.filePath) ?? Promise.resolve());
    const entries = await this.readEntries();
    return entries.get(key) ?? "";
  }

  private async setValue(key: string, value: string): Promise<void> {
    await this.withMutation(async () => {
      const entries = await this.readEntries();
      entries.set(key, value);
      await this.writeEntries(entries);
    });
  }

  private async deleteValue(key: string): Promise<void> {
    await this.withMutation(async () => {
      const entries = await this.readEntries();
      if (!entries.delete(key)) return;
      await this.writeEntries(entries);
    });
  }

  private async readEntries(): Promise<Map<string, string>> {
    try {
      return parseEnv(await fs.readFile(this.filePath, "utf8"));
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        return new Map();
      }
      throw new AwakenEnvStoreError(
        "unreadable",
        "无法读取 ~/.awaken/.env 中的 API key。",
        { cause: error },
      );
    }
  }

  private async writeEntries(entries: Map<string, string>): Promise<void> {
    const directory = path.dirname(this.filePath);
    const tempPath = path.join(directory, `.env.${process.pid}.${randomUUID()}.tmp`);
    try {
      await fs.mkdir(directory, { recursive: true, mode: 0o700 });
      await fs.writeFile(tempPath, serializeEnv(entries), { encoding: "utf8", mode: 0o600 });
      await fs.rename(tempPath, this.filePath);
      await fs.chmod(this.filePath, 0o600);
    } catch (error) {
      throw new AwakenEnvStoreError(
        "write_failed",
        "无法写入 ~/.awaken/.env 中的 API key。",
        { cause: error },
      );
    } finally {
      await fs.rm(tempPath, { force: true }).catch(() => undefined);
    }
  }

  private async withMutation<T>(operation: () => Promise<T>): Promise<T> {
    const previous = mutationQueuesByPath.get(this.filePath) ?? Promise.resolve();
    let release: (() => void) | undefined;
    const current = new Promise<void>((resolve) => {
      release = resolve;
    });
    mutationQueuesByPath.set(this.filePath, current);
    await previous;
    try {
      return await operation();
    } finally {
      release?.();
      if (mutationQueuesByPath.get(this.filePath) === current) {
        mutationQueuesByPath.delete(this.filePath);
      }
    }
  }
}
