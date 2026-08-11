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
  catalog_models?: string[];
  catalog_entries?: ProviderCatalogEntry[];
  observed_at?: string;
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
      (entry as Partial<ScopedCapabilityOverrides>).models !== null &&
      ((entry as Partial<ScopedCapabilityOverrides>).catalog_models === undefined ||
        Array.isArray((entry as Partial<ScopedCapabilityOverrides>).catalog_models)) &&
      ((entry as Partial<ScopedCapabilityOverrides>).catalog_entries === undefined ||
        Array.isArray((entry as Partial<ScopedCapabilityOverrides>).catalog_entries)),
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
    case "openai_responses":
      return "openai_responses_v1";
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
    merged.set(key, {
      scope: { ...entry.scope },
      models,
      catalog_models: entry.catalog_models ?? existing?.catalog_models,
      catalog_entries: entry.catalog_entries ?? existing?.catalog_entries,
      observed_at: entry.observed_at ?? existing?.observed_at,
    });
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

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined;
}

function firstNumber(
  sources: Array<Record<string, unknown> | undefined>,
  keys: string[],
): number | undefined {
  for (const source of sources) {
    if (!source) continue;
    for (const key of keys) {
      const value = extractNumber(source[key]);
      if (value !== undefined) return value;
    }
  }
  return undefined;
}

function firstBoolean(
  sources: Array<Record<string, unknown> | undefined>,
  keys: string[],
): boolean | undefined {
  for (const source of sources) {
    if (!source) continue;
    for (const key of keys) {
      if (typeof source[key] === "boolean") return source[key] as boolean;
    }
  }
  return undefined;
}

function featureNames(value: unknown): Set<string> {
  return new Set(catalogSignalStrings(value).map((item) => item.replaceAll("-", "_")));
}

function stableReasoningEffort(
  sources: Array<Record<string, unknown> | undefined>,
): ModelCapabilityWire["reasoning_effort"] | undefined {
  for (const source of sources) {
    if (!source) continue;
    const raw = source.reasoning_effort ?? source.reasoningEffort;
    const rawObject = objectValue(raw);
    const levelsValue = rawObject?.levels ?? rawObject?.supported_levels ?? raw;
    const nativeLevels = catalogSignalStrings(levelsValue);
    if (nativeLevels.length === 0) continue;
    const levels = [
      nativeLevels.some((level) => level === "low" || level === "minimal") ? "low" : undefined,
      nativeLevels.some((level) => level === "medium" || level === "high") ? "high" : undefined,
      nativeLevels.some((level) => level === "xhigh" || level === "max") ? "max" : undefined,
    ].filter((level): level is "low" | "high" | "max" => Boolean(level));
    if (levels.length === 0) continue;
    const nativeDefault = String(rawObject?.default_level ?? rawObject?.default ?? "").toLowerCase();
    const defaultLevel = nativeDefault === "low" || nativeDefault === "minimal"
      ? "low"
      : nativeDefault === "medium" || nativeDefault === "high"
        ? "high"
        : levels.includes("max")
          ? "max"
          : levels.at(-1)!;
    const displayLabels: Partial<Record<"low" | "high" | "max", string>> = {};
    const lowLabel = nativeLevels.find((level) => level === "low" || level === "minimal");
    const highLabel = nativeLevels.find((level) => level === "medium" || level === "high");
    const maxLabel = nativeLevels.find((level) => level === "xhigh" || level === "max");
    if (lowLabel) displayLabels.low = lowLabel;
    if (highLabel) displayLabels.high = highLabel;
    if (maxLabel) displayLabels.max = maxLabel;
    return {
      levels,
      default_level: defaultLevel,
      display_labels: Object.keys(displayLabels).length > 0 ? displayLabels : undefined,
    };
  }
  return undefined;
}

function supportedProtocolsFromModelObject(
  sources: Array<Record<string, unknown> | undefined>,
): string[] | undefined {
  const supported = new Set<string>();
  for (const source of sources) {
    if (!source) continue;
    const raw = source.supported_protocols ?? source.supportedProtocols ?? source.protocols;
    for (const value of catalogSignalStrings(raw)) {
      const normalized = value.trim().toLowerCase().replaceAll("-", "_");
      if (normalized === "openai_chat" || normalized === "chat_completions" || normalized === "chat") {
        supported.add("openai_chat");
      } else if (normalized === "openai_responses" || normalized === "responses") {
        supported.add("openai_responses");
      } else if (normalized === "anthropic_messages" || normalized === "messages" || normalized === "anthropic") {
        supported.add("anthropic_messages");
      }
    }
  }
  return supported.size > 0 ? [...supported] : undefined;
}

function capabilityFromModelObject(item: unknown): ModelCapabilityWire | undefined {
  if (typeof item !== "object" || item === null) return undefined;
  const model = item as Record<string, unknown>;
  const id = catalogModelID(model);
  if (!id) return undefined;

  const cap: ModelCapabilityWire = {};

  const capabilities = objectValue(model.capabilities);
  const features = objectValue(model.features);
  const limits = objectValue(model.limits) ?? objectValue(model.token_limits) ?? objectValue(model.tokenLimits);
  const inputLimits = objectValue(limits?.input) ?? objectValue(capabilities?.input);
  const outputLimits = objectValue(limits?.output) ?? objectValue(capabilities?.output);
  const contextLimits = objectValue(model.context_window) ?? objectValue(capabilities?.context_window);
  const sources = [model, capabilities, features, limits, contextLimits];

  const supportedProtocols = supportedProtocolsFromModelObject(sources);
  if (supportedProtocols) {
    cap.supported_protocols = supportedProtocols;
  }

  const contextWindow =
    firstNumber(sources, ["context_window", "contextWindow", "max_context_length", "context_length", "num_ctx", "max_tokens", "tokens"]);
  if (contextWindow !== undefined) {
    cap.context_window = contextWindow;
  }
  const maxInputTokens =
    firstNumber([...sources, inputLimits], ["max_input_tokens", "maxInputTokens", "inputTokenLimit", "token_limit", "tokens"]);
  if (maxInputTokens !== undefined) {
    cap.max_input_tokens = maxInputTokens;
  }

  const maxOutputTokens =
    firstNumber([...sources, outputLimits], ["max_output_tokens", "maxOutputTokens", "outputTokenLimit", "max_tokens", "token_limit", "tokens"]);
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

  const featureSet = featureNames([model.features, model.capabilities]);
  const nestedVision = firstBoolean([capabilities, features], ["image_input", "vision", "supports_vision", "supports_image_in"]);
  if (nestedVision !== undefined) cap.supports_vision = nestedVision;
  else if (featureSet.has("image_input") || featureSet.has("vision")) cap.supports_vision = true;
  const nestedPDF = firstBoolean([capabilities, features], ["pdf_input", "native_pdf", "supports_native_pdf"]);
  if (nestedPDF !== undefined) cap.supports_native_pdf = nestedPDF;
  else if (featureSet.has("pdf_input") || featureSet.has("native_pdf")) cap.supports_native_pdf = true;
  const nestedVideo = firstBoolean([capabilities, features], ["video_input", "supports_video", "supports_video_in"]);
  if (nestedVideo !== undefined) cap.supports_video = nestedVideo;
  else if (featureSet.has("video_input")) cap.supports_video = true;
  const nestedTools = firstBoolean([capabilities, features], ["tool_use", "tools", "function_calling", "supports_tools"]);
  if (nestedTools !== undefined) cap.supports_tools = nestedTools ? "ready" : "unsupported";
  else if (featureSet.has("tool_use") || featureSet.has("function_calling")) cap.supports_tools = "ready";
  const nestedCache = firstBoolean([capabilities, features], ["prompt_caching", "prompt_cache", "supports_prompt_cache"]);
  if (nestedCache !== undefined) cap.supports_prompt_cache = nestedCache;
  else if (featureSet.has("prompt_caching") || featureSet.has("prompt_cache")) cap.supports_prompt_cache = true;
  const reasoningEffort = stableReasoningEffort([model, capabilities, features]);
  if (reasoningEffort) cap.reasoning_effort = reasoningEffort;

  return Object.keys(cap).length > 0 ? cap : undefined;
}

export function parseProviderCapabilities(body: unknown): ProviderCapabilityOverrides {
  const list = Array.isArray(body)
    ? body
    : Array.isArray((body as Record<string, unknown> | undefined)?.data)
      ? (body as Record<string, unknown>).data
      : Array.isArray((body as Record<string, unknown> | undefined)?.models)
        ? (body as Record<string, unknown>).models
      : undefined;

  if (!Array.isArray(list)) {
    throw new Error("Unexpected /models response shape");
  }

  const overrides: ProviderCapabilityOverrides = {};
  for (const item of list) {
    const cap = capabilityFromModelObject(item);
    if (cap) {
      const id = catalogModelID(item as Record<string, unknown>);
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

export type ProviderCatalogStrategy = "openai" | "anthropic" | "google" | "none";

export type ProviderCatalogKind = "llm" | "image" | "video" | "audio" | "embedding" | "unsupported" | "unknown";

export type ProviderCatalogEntry = {
  id: string;
  kind: ProviderCatalogKind;
};

export type ProviderCatalogResult = {
  models: string[];
  entries: ProviderCatalogEntry[];
  capabilities: ProviderCapabilityOverrides;
  observedAt: string;
};

function authHeaders(authScheme: string, apiKey: string): Record<string, string> {
  const key = apiKey.trim();
  switch (authScheme.trim().toLowerCase()) {
    case "x-api-key":
      return { "x-api-key": key };
    case "api-key":
      return { "api-key": key };
    case "x-goog-api-key":
      return { "x-goog-api-key": key };
    case "none":
      return {};
    default:
      return { Authorization: `Bearer ${key}` };
  }
}

function modelList(body: unknown): Array<Record<string, unknown>> {
  if (Array.isArray(body)) return body.filter(isObject);
  if (!isObject(body)) return [];
  const output = isObject(body.output) ? body.output : undefined;
  const value = Array.isArray(body.data)
    ? body.data
    : Array.isArray(body.models)
      ? body.models
      : Array.isArray(output?.models)
        ? output.models
      : [];
  return value.filter(isObject);
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function catalogModelID(item: Record<string, unknown>): string {
  const value =
    typeof item.id === "string"
      ? item.id
      : typeof item.model === "string"
        ? item.model
        : typeof item.model_name === "string"
          ? item.model_name
          : typeof item.name === "string"
            ? item.name
            : "";
  return value.trim().replace(/^models\//, "");
}

function catalogSignalStrings(value: unknown, depth = 0): string[] {
  if (depth > 3 || value === null || value === undefined) return [];
  if (typeof value === "string") return [value.toLowerCase()];
  if (Array.isArray(value)) {
    return value.flatMap((item) => catalogSignalStrings(item, depth + 1));
  }
  if (!isObject(value)) return [];
  return Object.entries(value).flatMap(([key, child]) => [
    key.toLowerCase(),
    ...catalogSignalStrings(child, depth + 1),
  ]);
}

function catalogModelKind(item: Record<string, unknown>): ProviderCatalogKind {
  const id = catalogModelID(item).toLowerCase();
  const kindSignals = [
    id,
    ...catalogSignalStrings(item.type),
    ...catalogSignalStrings(item.model_type),
    ...catalogSignalStrings(item.task),
    ...catalogSignalStrings(item.category),
    ...catalogSignalStrings(item.supported_generation_methods),
  ].join(" ");
  const capabilitySignals = [
    ...catalogSignalStrings(item.capabilities),
    ...catalogSignalStrings(item.features),
  ].join(" ");

  // Explicit catalog metadata wins. Name-family checks cover providers whose
  // OpenAI-compatible /models objects expose only id/object/owned_by.
  if (/\b(video|text-to-video|image-to-video)\b/.test(kindSignals) || /(^|[-_.])(happyhorse|veo)([-_.]|$)/.test(id)) {
    return "video";
  }
  if (/\b(image|text-to-image|image-generation)\b/.test(kindSignals) || /(^|[-_.])(imagen|dall-e|gpt-image)([-_.]|$)/.test(id)) {
    return "image";
  }
  if (/\b(audio|speech|tts|asr|transcription)\b/.test(kindSignals) || /(^|[-_.])(whisper|tts)([-_.]|$)/.test(id)) {
    return "audio";
  }
  if (/\b(embedding|rerank|moderation)\b/.test(kindSignals) || /(^|[-_.])(embedding|rerank|moderation)([-_.]|$)/.test(id)) {
    return "embedding";
  }
  if (/\b(chat|llm|language|text-generation|generatecontent|function-calling)\b/.test(`${kindSignals} ${capabilitySignals}`)) {
    return "llm";
  }
  return "unknown";
}

function catalogURL(
  strategy: ProviderCatalogStrategy,
  baseURL: string,
  override?: string,
): string {
  if (override?.trim()) return override.trim();
  if (strategy === "google") {
    return "https://generativelanguage.googleapis.com/v1beta/models";
  }
  const base = trimTrailingSlash(baseURL);
  if (strategy === "anthropic" && !/\/v1$/i.test(base)) return `${base}/v1/models`;
  return `${base}/models`;
}

function catalogHeaders(
  strategy: ProviderCatalogStrategy,
  authScheme: string,
  apiKey: string,
): Record<string, string> {
  if (strategy === "google") return { "x-goog-api-key": apiKey.trim() };
  const headers = authHeaders(authScheme, apiKey);
  if (strategy === "anthropic") headers["anthropic-version"] = "2023-06-01";
  return headers;
}

function nextCatalogURL(
  strategy: ProviderCatalogStrategy,
  currentURL: string,
  body: Record<string, unknown>,
): string | undefined {
  if (strategy === "anthropic" && body.has_more === true && typeof body.last_id === "string") {
    const next = new URL(currentURL);
    next.searchParams.set("after_id", body.last_id);
    return next.toString();
  }
  if (strategy === "google" && typeof body.nextPageToken === "string" && body.nextPageToken) {
    const next = new URL(currentURL);
    next.searchParams.set("pageToken", body.nextPageToken);
    return next.toString();
  }
  return undefined;
}

export async function fetchProviderCatalog(
  strategy: ProviderCatalogStrategy,
  baseURL: string,
  apiKey: string,
  authScheme: string,
  signal?: AbortSignal,
  catalogURLOverride?: string,
): Promise<ProviderCatalogResult> {
  if (strategy === "none") {
    return { models: [], entries: [], capabilities: {}, observedAt: new Date().toISOString() };
  }
  let url: string | undefined = catalogURL(
    strategy,
    baseURL,
    catalogURLOverride,
  );
  const entries: Array<Record<string, unknown>> = [];
  for (let page = 0; url && page < 20; page += 1) {
    const response = await fetch(url, {
      method: "GET",
      headers: catalogHeaders(strategy, authScheme, apiKey),
      signal,
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const body = (await response.json()) as unknown;
    entries.push(...modelList(body));
    url = isObject(body) ? nextCatalogURL(strategy, url, body) : undefined;
  }
  const observedAt = new Date().toISOString();
  const catalogEntries = Array.from(
    new Map(
      entries
        .map((entry) => ({ id: catalogModelID(entry), kind: catalogModelKind(entry) }))
        .filter((entry) => entry.id)
        .map((entry) => [entry.id, entry] as const),
    ).values(),
  ).sort((left, right) => left.id.localeCompare(right.id));
  const admittedIDs = new Set(
    catalogEntries
      .filter((entry) => entry.kind === "llm" || entry.kind === "unknown")
      .map((entry) => entry.id),
  );
  const admittedObjects = entries.filter((entry) => admittedIDs.has(catalogModelID(entry)));
  const capabilities = parseProviderCapabilities(admittedObjects);
  const fields = [
    "supported_protocols",
    "max_input_tokens",
    "context_window",
    "max_output_tokens",
    "supports_vision",
    "supports_native_pdf",
    "supports_video",
    "supports_tools",
    "supports_prompt_cache",
    "reasoning_effort",
  ] as const;
  for (const capability of Object.values(capabilities)) {
    capability.field_sources = {};
    for (const field of fields) {
      if (capability[field] === undefined) continue;
      const publicField = ({
        supported_protocols: "supportedProtocols",
        max_input_tokens: "maxInputTokens",
        context_window: "contextWindow",
        max_output_tokens: "maxOutputTokens",
        supports_vision: "supportsVision",
        supports_native_pdf: "supportsNativePDF",
        supports_video: "supportsVideo",
        supports_tools: "supportsTools",
        supports_prompt_cache: "supportsPromptCache",
        reasoning_effort: "reasoningEffort",
      } as const)[field];
      capability.field_sources[publicField] = { source: "provider_api", observed_at: observedAt };
    }
  }
  const models = [...admittedIDs].sort();
  return { models, entries: catalogEntries, capabilities, observedAt };
}

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}
