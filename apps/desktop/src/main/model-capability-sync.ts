import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type { ModelCapabilityWire } from "../shared/runtime.js";

const OVERRIDES_FILE = "model-capabilities.json";
const mutationQueuesByPath = new Map<string, Promise<void>>();

export type ProviderCapabilityOverrides = Record<string, ModelCapabilityWire>;

export type CapabilitySelectorKind = "model" | "endpoint";

export type CapabilityCacheScope = {
  provider_id: string;
  normalized_base_url: string;
  protocol_family: string;
  profile_id: string;
  credential_revision: number;
  selector_kind: CapabilitySelectorKind;
  endpoint_contract_id: string;
};

export type ScopedCapabilityOverrides = {
  scope: CapabilityCacheScope;
  models: ProviderCapabilityOverrides;
};

export type CapabilityResolution = {
  scope: CapabilityCacheScope;
  selector: string;
};

type CapabilityOverridesFile = {
  version: 2;
  scopes: ScopedCapabilityOverrides[];
};

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isValidScope(value: unknown): value is CapabilityCacheScope {
  if (typeof value !== "object" || value === null) return false;
  const scope = value as Partial<CapabilityCacheScope>;
  return (
    isString(scope.provider_id) &&
    isString(scope.normalized_base_url) &&
    isString(scope.protocol_family) &&
    isString(scope.profile_id) &&
    typeof scope.credential_revision === "number" &&
    Number.isInteger(scope.credential_revision) &&
    scope.credential_revision >= 1 &&
    (scope.selector_kind === "model" || scope.selector_kind === "endpoint") &&
    isString(scope.endpoint_contract_id)
  );
}

function isValidOverridesFile(value: unknown): value is CapabilityOverridesFile {
  if (typeof value !== "object" || value === null) return false;
  const file = value as Partial<CapabilityOverridesFile>;
  if (file.version !== 2 || !Array.isArray(file.scopes)) return false;
  return file.scopes.every(
    (entry) =>
      typeof entry === "object" &&
      entry !== null &&
      isValidScope((entry as Partial<ScopedCapabilityOverrides>).scope) &&
      typeof (entry as Partial<ScopedCapabilityOverrides>).models === "object" &&
      (entry as Partial<ScopedCapabilityOverrides>).models !== null,
  );
}

export function normalizeCapabilityBaseURL(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "");
  try {
    const parsed = new URL(trimmed);
    parsed.protocol = parsed.protocol.toLowerCase();
    parsed.hostname = parsed.hostname.toLowerCase();
    parsed.search = "";
    parsed.hash = "";
    parsed.pathname = parsed.pathname.replace(/\/+$/, "");
    return parsed.toString().replace(/\/+$/, "");
  } catch {
    return trimmed;
  }
}

export function capabilityEndpointContract(protocol: string): string {
  switch (protocol.trim().toLowerCase()) {
    case "minimax_chat":
      return "minimax_chatcompletion_v2";
    case "anthropic_messages":
      return "anthropic_messages_v1";
    default:
      return "openai_chat_completions_v1";
  }
}

export function capabilityScopeKey(scope: CapabilityCacheScope): string {
  return JSON.stringify([
    scope.provider_id.trim().toLowerCase(),
    normalizeCapabilityBaseURL(scope.normalized_base_url),
    scope.protocol_family.trim().toLowerCase(),
    scope.profile_id.trim(),
    Math.max(1, Math.floor(scope.credential_revision)),
    scope.selector_kind,
    scope.endpoint_contract_id.trim().toLowerCase(),
  ]);
}

export async function loadCapabilityOverrides(
  cacheDir: string,
): Promise<ScopedCapabilityOverrides[]> {
  const filePath = path.resolve(cacheDir, OVERRIDES_FILE);
  await (mutationQueuesByPath.get(filePath) ?? Promise.resolve());
  return readCapabilityOverrides(filePath);
}

async function readCapabilityOverrides(
  filePath: string,
): Promise<ScopedCapabilityOverrides[]> {
  try {
    const raw = await fs.readFile(filePath, "utf8");
    const parsed = JSON.parse(raw) as unknown;
    if (isValidOverridesFile(parsed)) {
      return parsed.scopes;
    }
    // Version 1 was provider-wide and cannot prove endpoint/profile authority.
    // Ignore it as a cold cache instead of applying it to an unrelated scope.
  } catch {
    // Ignore missing or corrupted override files.
  }
  return [];
}

export async function saveCapabilityOverrides(
  cacheDir: string,
  scopes: ScopedCapabilityOverrides[],
): Promise<ScopedCapabilityOverrides[]> {
  const filePath = path.resolve(cacheDir, OVERRIDES_FILE);
  return withPathMutation(filePath, async () => {
    const merged = mergeCapabilityOverrides(
      await readCapabilityOverrides(filePath),
      scopes,
    );
    await fs.mkdir(path.dirname(filePath), { recursive: true });
    const tempPath = `${filePath}.${process.pid}.${randomUUID()}.tmp`;
    const file: CapabilityOverridesFile = { version: 2, scopes: merged };
    try {
      await fs.writeFile(tempPath, `${JSON.stringify(file, null, 2)}\n`, "utf8");
      await fs.rename(tempPath, filePath);
    } finally {
      await fs.rm(tempPath, { force: true }).catch(() => undefined);
    }
    return merged;
  });
}

async function withPathMutation<T>(
  filePath: string,
  operation: () => Promise<T>,
): Promise<T> {
  const previous = mutationQueuesByPath.get(filePath) ?? Promise.resolve();
  let release: (() => void) | undefined;
  const current = new Promise<void>((resolve) => {
    release = resolve;
  });
  mutationQueuesByPath.set(filePath, current);
  await previous;
  try {
    return await operation();
  } finally {
    release?.();
    if (mutationQueuesByPath.get(filePath) === current) {
      mutationQueuesByPath.delete(filePath);
    }
  }
}

export function mergeCapabilityOverrides(
  base: ScopedCapabilityOverrides[],
  incoming: ScopedCapabilityOverrides[],
): ScopedCapabilityOverrides[] {
  const merged = new Map<string, ScopedCapabilityOverrides>();
  for (const entry of [...base, ...incoming]) {
    const key = capabilityScopeKey(entry.scope);
    const existing = merged.get(key);
    const models: ProviderCapabilityOverrides = { ...(existing?.models ?? {}) };
    for (const [selector, capability] of Object.entries(entry.models)) {
      models[selector] = {
        ...models[selector],
        ...capability,
      };
    }
    merged.set(key, { scope: { ...entry.scope }, models });
  }
  return [...merged.values()].sort((left, right) =>
    capabilityScopeKey(left.scope).localeCompare(capabilityScopeKey(right.scope)),
  );
}

function extractNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}

function capabilityFromModelObject(item: unknown): ModelCapabilityWire | undefined {
  if (typeof item !== "object" || item === null) return undefined;
  const model = item as Record<string, unknown>;
  if (typeof model.id !== "string" || model.id.trim() === "") return undefined;

  const cap: ModelCapabilityWire = {};

  const contextWindow =
    extractNumber(model.context_window) ??
    extractNumber(model.max_context_length) ??
    extractNumber(model.context_length) ??
    extractNumber(model.num_ctx);
  if (contextWindow !== undefined) {
    cap.context_window = contextWindow;
  }

  const maxOutputTokens =
    extractNumber(model.max_output_tokens) ?? extractNumber(model.max_tokens);
  if (maxOutputTokens !== undefined) {
    cap.max_output_tokens = maxOutputTokens;
  }

  const defaultMaxTokens = extractNumber(model.default_max_tokens);
  if (defaultMaxTokens !== undefined) {
    cap.default_max_tokens = defaultMaxTokens;
  }

  const defaultTemperature =
    extractNumber(model.default_temperature) ?? extractNumber(model.temperature);
  if (defaultTemperature !== undefined) {
    cap.default_temperature = defaultTemperature;
  }

  const supportsVision = model.supports_vision ?? model.supports_image_in;
  if (typeof supportsVision === "boolean") {
    cap.supports_vision = supportsVision;
  }
  if (typeof model.supports_native_pdf === "boolean") {
    cap.supports_native_pdf = model.supports_native_pdf;
  }
  const supportsVideo = model.supports_video ?? model.supports_video_in;
  if (typeof supportsVideo === "boolean") {
    cap.supports_video = supportsVideo;
  }

  return Object.keys(cap).length > 0 ? cap : undefined;
}

export function parseProviderCapabilities(body: unknown): ProviderCapabilityOverrides {
  const list = Array.isArray(body)
    ? body
    : Array.isArray((body as Record<string, unknown> | undefined)?.data)
      ? (body as Record<string, unknown>).data
      : undefined;

  if (!Array.isArray(list)) {
    throw new Error("Unexpected /models response shape");
  }

  const overrides: ProviderCapabilityOverrides = {};
  for (const item of list) {
    const cap = capabilityFromModelObject(item);
    if (cap) {
      const id = (item as Record<string, unknown>).id as string;
      overrides[id] = cap;
    }
  }
  return overrides;
}

export async function fetchProviderCapabilities(
  baseURL: string,
  apiKey: string,
  signal?: AbortSignal,
): Promise<ProviderCapabilityOverrides> {
  const url = `${trimTrailingSlash(baseURL)}/models`;
  const response = await fetch(url, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${apiKey.trim()}`,
    },
    signal,
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return parseProviderCapabilities(await response.json());
}

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}
