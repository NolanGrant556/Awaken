import type { RuntimeRunWire } from "../shared/runtime.js";

export class RuntimeDomainError extends Error {
  constructor(readonly code: string, message: string, readonly status: number, readonly details?: unknown) {
    super(message);
    this.name = "RuntimeDomainError";
  }
}

export async function requireRuntimeSuccess(response: Response): Promise<void> {
  if (response.ok) return;
  const raw = (await response.text()).trim();
  let code = "INTERNAL_ERROR", message = raw || `Runtime request failed (${response.status})`;
  let details: unknown;
  try {
    const parsed = JSON.parse(raw) as { code?: string; error?: string; message?: string; details?: unknown; };
    code = parsed.code ?? parsed.error ?? code;
    message = parsed.message ?? message;
    details = parsed.details;
  } catch {
    // Non-JSON transport responses keep their text for Main diagnostics.
  }
  throw new RuntimeDomainError(code, message, response.status, details);
}

export function createRuntimeRequester(urlFor: (path: string) => string) {
  return {
    async request<T = RuntimeRunWire>(path: string, init?: RequestInit): Promise<T> {
      const response = await fetch(urlFor(path), init);
      await requireRuntimeSuccess(response);
      return await response.json() as T;
    },
    async requestVoid(path: string, init?: RequestInit): Promise<void> {
      const response = await fetch(urlFor(path), init);
      await requireRuntimeSuccess(response);
    },
  };
}
export type RuntimeRequester = ReturnType<typeof createRuntimeRequester>;
