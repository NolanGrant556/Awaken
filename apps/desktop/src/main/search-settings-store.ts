import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";

export type SearchProviderId = "exa" | "tavily";

export type SearchSettingsSnapshot = {
  providers: Record<SearchProviderId, { enabled: boolean }>;
};

export type SearchSettingsSaveRequest = {
  provider: SearchProviderId;
  enabled: boolean;
};

type SearchSettingsFile = {
  version: 1;
  providers: Record<SearchProviderId, { enabled: boolean }>;
};

type SearchSettingsStoreDeps = {
  writeSettingsFile?: (value: SearchSettingsFile) => Promise<void>;
};

const DEFAULT_FILE: SearchSettingsFile = {
  version: 1,
  providers: {
    exa: { enabled: true },
    tavily: { enabled: false },
  },
};

function normalizeProvider(value: unknown): SearchProviderId {
  if (value === "exa" || value === "tavily") return value;
  throw new Error("搜索渠道无效。");
}

function normalizeFile(value: unknown): SearchSettingsFile {
  if (!value || typeof value !== "object") {
    throw new Error("搜索配置格式无效。");
  }
  const candidate = value as Record<string, unknown>;
  if (candidate.version !== 1) {
    throw new Error("搜索配置版本不受支持。");
  }
  const rawProviders =
    candidate.providers && typeof candidate.providers === "object"
      ? (candidate.providers as Record<string, unknown>)
      : {};
  const enabled = (provider: SearchProviderId, fallback: boolean) => {
    const entry = rawProviders[provider];
    return entry && typeof entry === "object"
      ? (entry as Record<string, unknown>).enabled === true
      : fallback;
  };
  return {
    version: 1,
    providers: {
      exa: { enabled: enabled("exa", true) },
      tavily: { enabled: enabled("tavily", false) },
    },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isValidExaResponse(body: string): boolean {
  const candidates = body.trimStart().startsWith("{")
    ? [body.trim()]
    : body
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice("data:".length).trim());
  return candidates.some((candidate) => {
    try {
      const payload: unknown = JSON.parse(candidate);
      if (!isRecord(payload) || isRecord(payload.error)) return false;
      const result = payload.result;
      if (!isRecord(result) || !Array.isArray(result.content)) return false;
      return result.content.some(
        (content) =>
          isRecord(content) &&
          content.type === "text" &&
          typeof content.text === "string" &&
          content.text.trim().length > 0,
      );
    } catch {
      return false;
    }
  });
}

function isValidTavilyResponse(body: string): boolean {
  try {
    const payload: unknown = JSON.parse(body);
    return isRecord(payload) && Array.isArray(payload.results);
  } catch {
    return false;
  }
}

export class SearchSettingsStore {
  private readonly filePath: string;
  private mutationQueue: Promise<void> = Promise.resolve();

  constructor(
    configDir: string,
    private readonly deps: SearchSettingsStoreDeps = {},
  ) {
    this.filePath = path.join(configDir, "search.json");
  }

  async snapshot(): Promise<SearchSettingsSnapshot> {
    await this.mutationQueue;
    const file = await this.readFile();
    return { providers: file.providers };
  }

  async save(request: SearchSettingsSaveRequest): Promise<SearchSettingsSnapshot> {
    return this.withMutation(async () => {
      const provider = normalizeProvider(request.provider);
      const previous = await this.readFile();
      const next: SearchSettingsFile = {
        version: 1,
        providers: {
          ...previous.providers,
          [provider]: { enabled: request.enabled === true },
        },
      };
      if (this.deps.writeSettingsFile) {
        await this.deps.writeSettingsFile(next);
      } else {
        await this.writeFile(next);
      }
      return { providers: next.providers };
    });
  }

  async runtimeEnvironment(): Promise<NodeJS.ProcessEnv> {
    const snapshot = await this.snapshot();
    const providers = (["exa", "tavily"] as const).filter(
      (provider) => snapshot.providers[provider].enabled,
    );
    return {
      AWAKEN_WEB_SEARCH_PROVIDERS: providers.join(","),
    };
  }

  async testProvider(
    providerValue: SearchProviderId,
  ): Promise<{ ok: boolean; message: string }> {
    const provider = normalizeProvider(providerValue);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 15_000);
    try {
      const response =
        provider === "exa"
          ? await fetch("https://mcp.exa.ai/mcp?tools=web_search_exa", {
              method: "POST",
              headers: {
                Accept: "application/json, text/event-stream",
                "Content-Type": "application/json",
              },
              body: JSON.stringify({
                jsonrpc: "2.0",
                id: 1,
                method: "tools/call",
                params: {
                  name: "web_search_exa",
                  arguments: {
                    query: "Awaken search provider connectivity test",
                    numResults: 1,
                  },
                },
              }),
              signal: controller.signal,
            })
          : await fetch("https://api.tavily.com/search", {
              method: "POST",
              headers: {
                Accept: "application/json",
                "Content-Type": "application/json",
                "X-Tavily-Access-Mode": "keyless",
              },
              body: JSON.stringify({
                query: "Awaken search provider connectivity test",
                search_depth: "basic",
                max_results: 1,
              }),
              signal: controller.signal,
            });
      if (response.ok) {
        const body = await response.text();
        const valid =
          provider === "exa"
            ? isValidExaResponse(body)
            : isValidTavilyResponse(body);
        if (!valid) {
          return {
            ok: false,
            message: `${provider === "exa" ? "Exa" : "Tavily"} 返回的响应格式无效。`,
          };
        }
        return {
          ok: true,
          message: `${provider === "exa" ? "Exa" : "Tavily"} 连接成功。`,
        };
      }
      if (response.status === 429) {
        return {
          ok: false,
          message: `${provider === "exa" ? "Exa" : "Tavily"} 已达到免 key 速率限制，请稍后重试。`,
        };
      }
      return {
        ok: false,
        message: `${provider === "exa" ? "Exa" : "Tavily"} 返回 HTTP ${response.status}。`,
      };
    } catch (error) {
      return {
        ok: false,
        message:
          (error as Error)?.name === "AbortError"
            ? `${provider === "exa" ? "Exa" : "Tavily"} 连接测试超时。`
            : `${provider === "exa" ? "Exa" : "Tavily"} 连接失败，请检查网络后重试。`,
      };
    } finally {
      clearTimeout(timer);
    }
  }

  private async readFile(): Promise<SearchSettingsFile> {
    try {
      return normalizeFile(JSON.parse(await fs.readFile(this.filePath, "utf8")));
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        return structuredClone(DEFAULT_FILE);
      }
      throw new Error("无法读取 ~/.awaken/config/search.json。", { cause: error });
    }
  }

  private async writeFile(value: SearchSettingsFile): Promise<void> {
    const directory = path.dirname(this.filePath);
    const tempPath = path.join(
      directory,
      `.search.${process.pid}.${randomUUID()}.tmp`,
    );
    try {
      await fs.mkdir(directory, { recursive: true, mode: 0o700 });
      await fs.writeFile(tempPath, `${JSON.stringify(value, null, 2)}\n`, {
        encoding: "utf8",
        mode: 0o600,
      });
      await fs.rename(tempPath, this.filePath);
      await fs.chmod(this.filePath, 0o600);
    } catch (error) {
      throw new Error("无法写入 ~/.awaken/config/search.json。", { cause: error });
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
