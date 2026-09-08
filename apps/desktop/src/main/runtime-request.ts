import { ipcMain, type IpcMainInvokeEvent } from "electron";
import type { DesktopRequestResult, RuntimeRequestIssue } from "../shared/runtime-request.js";
import { runtimeRequestIssue } from "../shared/runtime-request.js";
import { RuntimeDomainError } from "./runtime-http.js";

function safeMessage(message: string): string {
  return message
    .replace(/\b(?:Bearer\s+|(?:token|api[_-]?key|password|secret)\s*[=:]\s*)[^\s,;]+/gi, "[redacted]")
    .replace(/\b[A-Za-z]:\\[^\r\n,，。；;）)\]"']+/g, "本地路径")
    .replace(/(?<!:)(?:\/[\w.@+~-]+){2,}/gu, "本地路径");
}

export async function runRuntimeRequest<T>(operation: () => T | Promise<T>): Promise<DesktopRequestResult<T>> {
  try { return { ok: true, value: await operation() }; }
  catch (error) {
    const legacy = runtimeRequestIssue(error);
    const issue: RuntimeRequestIssue = error instanceof RuntimeDomainError
      ? { code: error.code, status: error.status, message: safeMessage(error.message), details: legacy.details }
      : { ...legacy, message: safeMessage(legacy.message) || "操作未完成，请稍后重试。" };
    if (issue.details?.reason) issue.details.reason = safeMessage(issue.details.reason);
    return { ok: false, issue };
  }
}

export function registerRuntimeRequest<Args extends unknown[], Result>(
  channel: string,
  handler: (event: IpcMainInvokeEvent, ...args: Args) => Result | Promise<Result>,
): void {
  ipcMain.handle(channel, (event, ...args) => runRuntimeRequest(() => handler(event, ...args as Args)));
}
