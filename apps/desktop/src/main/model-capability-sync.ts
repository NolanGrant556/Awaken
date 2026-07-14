import fs from "node:fs/promises";
import path from "node:path";
import type { ModelCapabilityWire } from "../shared/runtime.js";

const OVERRIDES_FILE = "model-capabilities.json";

export type ProviderCapabilityOverrides = Record<string, ModelCapabilityWire>;

type CapabilityOverridesFile = {
  version: 1;
  providers: Record<string, ProviderCapabilityOverrides>;
};

function isValidOverridesFile(value: unknown): value is CapabilityOverridesFile {
  if (typeof value !== "object" || value === null) return false;
  const file = value as Partial<CapabilityOverridesFile>;
  if (file.version !== 1) return false;
  if (typeof file.providers !== "object" || file.providers === null) return false;
  return true;
}

export async function loadCapabilityOverrides(
  cacheDir: string,
): Promise<Record<string, ProviderCapabilityOverrides>> {
  try {
    const filePath = path.join(cacheDir, OVERRIDES_FILE);
    const raw = await fs.readFile(filePath, "utf8");
    const parsed = JSON.parse(raw) as unknown;
    if (isValidOverridesFile(parsed)) {
      return parsed.providers;
    }
  } catch {
    // Ignore missing or corrupted override file.
  }
  return {};
}

export async function saveCapabilityOverrides(
  cacheDir: string,
  providers: Record<string, ProviderCapabilityOverrides>,
): Promise<void> {
  const filePath = path.join(cacheDir, OVERRIDES_FILE);
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  const tempPath = `${filePath}.${process.pid}.tmp`;
  const file: CapabilityOverridesFile = { version: 1, providers };
  await fs.writeFile(tempPath, `${JSON.stringify(file, null, 2)}\n`, "utf8");
  await fs.rename(tempPath, filePath);
}

export function mergeCapabilityOverrides(
  base: Record<string, ProviderCapabilityOverrides>,
  incoming: Record<string, ProviderCapabilityOverrides>,
): Record<string, ProviderCapabilityOverrides> {
  const merged: Record<string, ProviderCapabilityOverrides> = {};
  for (const providerId of new Set([...Object.keys(base), ...Object.keys(incoming)])) {
    merged[providerId] = { ...base[providerId], ...incoming[providerId] };
  }
  return merged;
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

  if (typeof model.supports_vision === "boolean") {
    cap.supports_vision = model.supports_vision;
  }
  if (typeof model.supports_video === "boolean") {
    cap.supports_video = model.supports_video;
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
