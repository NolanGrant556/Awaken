import type {
  DesktopOperationIssue,
  DesktopOperationResult,
} from "../shared/runtime.js";
import { AwakenEnvStoreError } from "./awaken-env-store.js";
import { AgentPlanAccountStoreError } from "./agent-plan-account-store.js";

const SAFE_OPERATION_MESSAGES = new Set([
  "模型配置重复，添加失败",
  "未知模型供应商",
  "请先在设置 > 模型中配置 API key。",
  "请先在设置 > 模型中配置可用模型。",
  "当前有任务正在运行，请结束任务后重试。",
  "Agent Plan 账户已保存，但 Core 重启失败。请重启 Awaken 后再使用生成工具。",
]);

function runtimeDomainIssue(error: unknown): DesktopOperationIssue | null {
  if (!(error instanceof Error) || error.name !== "RuntimeDomainError") return null;
  const domainError = error as Error & {
    code?: unknown;
    details?: { comment_id?: unknown; reason?: unknown };
  };
  const code = String(domainError.code ?? "");
  if (code === "FILE_COMMENT_STALE" || code === "INVALID_FILE_COMMENT") {
    const commentId = typeof domainError.details?.comment_id === "string"
      ? domainError.details.comment_id
      : undefined;
    const reason = typeof domainError.details?.reason === "string"
      ? domainError.details.reason
      : undefined;
    return {
      code: code === "FILE_COMMENT_STALE"
        ? "file_comment_stale"
        : "invalid_file_comment",
      message: code === "FILE_COMMENT_STALE"
        ? "文件内容已变化，请重新选择行并添加评论。"
        : "文件评论无效，请检查所选文件和行号。",
      retryable: true,
      actions: ["retry"],
      commentId,
      reason,
    };
  }
  if (code === "INVALID_COMPOSER_TOKEN") {
    return {
      code: "composer_selection_invalid",
      message: "选择的 Skill 已不可用或已经发生变化，请重新选择后再运行。",
      retryable: false,
      actions: [],
    };
  }
  if (code === "INVALID_INPUT") {
    if (/composer Plugin Agent|composer Plugin Command/u.test(error.message)) {
      return {
        code: "plugin_component_unavailable",
        message: "选择的插件组件已停用、已被替换或不再可运行，请重新打开插件后选择。",
        retryable: false,
        actions: [],
      };
    }
    if (/composer Plugin/u.test(error.message)) {
      return {
        code: "plugin_selection_invalid",
        message: "选择的插件已停用、内容已变化或没有可运行组件，请重新选择。",
        retryable: false,
        actions: [],
      };
    }
    return {
      code: "run_input_invalid",
      message: "当前输入无法开始运行，请检查 Composer 中的附件和组件选择。",
      retryable: false,
      actions: [],
    };
  }
  if (
    code === "THREAD_BUSY" ||
    code === "THREAD_ARCHIVED" ||
    code === "THREAD_WORKSPACE_MISMATCH" ||
    code === "PROJECT_OPERATION_CONFLICT"
  ) {
    return {
      code: "run_conflict",
      message:
        code === "THREAD_BUSY"
          ? "当前对话已有任务正在运行，请结束后再试。"
          : "当前对话或项目状态已变化，请刷新后重试。",
      retryable: true,
      actions: ["retry"],
    };
  }
  if (code === "RUN_SERVICE_UNAVAILABLE") {
    return {
      code: "runtime_unavailable",
      message: "Awaken Core 暂时不可用，请重新启动 Awaken 后重试。",
      retryable: true,
      actions: ["relaunch"],
    };
  }
  return null;
}

export function desktopOperationIssue(error: unknown): DesktopOperationIssue {
  const runtimeIssue = runtimeDomainIssue(error);
  if (runtimeIssue) return runtimeIssue;
  if (
    (error instanceof AwakenEnvStoreError || error instanceof AgentPlanAccountStoreError) &&
    error.code === "unreadable"
  ) {
    return {
      code: "credential_unreadable",
      message: error.message,
      retryable: true,
      actions: ["retry", "reenter"],
    };
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
