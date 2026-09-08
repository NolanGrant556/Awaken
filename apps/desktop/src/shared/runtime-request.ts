import type { DesktopOperationResult } from "./runtime.js";

export type RuntimeRequestIssue = {
  code: string;
  message: string;
  status?: number;
  details?: { field?: string; reason?: string; };
};
export type DesktopRequestResult<T> = DesktopOperationResult<T, RuntimeRequestIssue>;

export class RuntimeRequestError extends Error {
  readonly code: string;
  readonly status?: number;
  readonly details?: RuntimeRequestIssue["details"];
  constructor(issue: RuntimeRequestIssue) {
    super(issue.message);
    this.name = "RuntimeRequestError";
    this.code = issue.code;
    this.status = issue.status;
    this.details = issue.details;
  }
}

export async function unwrapRuntimeRequest<T>(reply: Promise<DesktopRequestResult<T>>): Promise<T> {
  const result = await reply;
  if (result.ok) return result.value;
  throw new RuntimeRequestError(result.issue);
}

/** Older persisted errors and third-party messages are decoded only here. */
export function runtimeRequestIssue(error: unknown): RuntimeRequestIssue {
  if (error instanceof RuntimeRequestError) return {
    code: error.code, message: error.message, status: error.status, details: error.details,
  };
  const message = (error instanceof Error ? error.message : String(error ?? ""))
    .replace(/^Error invoking remote method\s+(['"])[^'"]+\1:\s*/i, "")
    .replace(/^(?:Error|RuntimeDomainError):\s*/i, "").trim();
  const match = message.match(/^([A-Z][A-Z0-9_]+):\s*/);
  const code = match?.[1] ?? (/task was modified/i.test(message) ? "VERSION_CONFLICT"
    : /task already has an active execution/i.test(message) ? "TASK_ALREADY_RUNNING"
      : /run_not_found|404 (?:page )?not found/i.test(message) ? "RUN_NOT_FOUND" : "DESKTOP_REQUEST_FAILED");
  const clean = match ? message.slice(match[0].length) : message;
  const field = /^invalid ([a-z_.]+):\s*(.+)$/i.exec(clean);
  return { code, message: clean, ...(field ? { details: { field: field[1], reason: field[2] } } : {}) };
}
