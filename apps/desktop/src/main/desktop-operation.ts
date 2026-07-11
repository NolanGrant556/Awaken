import type {
  DesktopOperationIssue,
  DesktopOperationResult,
} from "../shared/runtime.js";
import { SecureCredentialError } from "./secure-credential-store.js";

const SAFE_OPERATION_MESSAGES = new Set([
  "模型配置重复，添加失败",
  "未知模型供应商",
  "请先在设置 > 模型中配置 API key。",
  "请先在设置 > 模型中配置可用模型。",
]);

export function desktopOperationIssue(error: unknown): DesktopOperationIssue {
  if (error instanceof SecureCredentialError) {
    switch (error.code) {
      case "temporarily_unavailable":
        return {
          code: "credential_temporarily_unavailable",
          message: error.message,
          retryable: true,
          actions: ["relaunch"],
        };
      case "encrypt_failed":
        return {
          code: "credential_save_failed",
          message: error.message,
          retryable: error.retryable,
          actions: ["relaunch"],
        };
      case "invalid_ciphertext":
      case "unsupported_version":
      case "decrypt_failed":
        return {
          code: "credential_unreadable",
          message: error.message,
          retryable: false,
          actions: ["reenter"],
        };
    }
  }

  const message = error instanceof Error ? error.message : "";
  return {
    code: "operation_failed",
    message: SAFE_OPERATION_MESSAGES.has(message)
      ? message
      : "Awaken 操作未完成，请稍后重试。",
    retryable: true,
    actions: ["retry"],
  };
}

export async function runDesktopOperation<T>(
  operation: () => Promise<T>,
): Promise<DesktopOperationResult<T>> {
  try {
    return { ok: true, value: await operation() };
  } catch (error) {
    return { ok: false, issue: desktopOperationIssue(error) };
  }
}
