export const INTERNAL_COMPACTION_FAILURE_UI_MESSAGE =
  "本次任务未能继续，请重试。";

function isInternalCompactionFailure(message: string): boolean {
  return /COMPACT_TARGET_UNACHIEVABLE|context compaction|compact continuation failed|insufficient reduction|still above (?:auto|block).*threshold|not enough compactable history|oversized protected tail/i.test(
    message,
  );
}

export function runtimeErrorMessageForDisplay(
  code?: string,
  message?: string,
): string {
  const rawMessage = message || "Runtime returned an error";
  if (isInternalCompactionFailure(rawMessage)) {
    return INTERNAL_COMPACTION_FAILURE_UI_MESSAGE;
  }
  if (code === "PROVIDER_AUTH_MISSING") {
    return "API key 缺失，请在 Settings > Models 配置模型 API key";
  }
  if (code === "PROVIDER_REQUEST_FAILED") {
    return `Provider 请求失败：${rawMessage}`;
  }
  return rawMessage;
}
