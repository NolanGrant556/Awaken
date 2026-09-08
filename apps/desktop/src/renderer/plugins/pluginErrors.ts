import { runtimeRequestIssue } from "../../shared/runtime-request";
const PLUGIN_ERROR_MESSAGES: Record<string, string> = {
  PLUGIN_NOT_FOUND: "插件不存在或已经被移除",
  PLUGIN_ALREADY_EXISTS: "已经存在同名插件",
  PLUGIN_DIGEST_CHANGED: "插件安装内容已变化，完整性校验失败",
  PLUGIN_RECORDED_DIGEST_CHANGED: "插件记录的摘要与当前状态不一致",
  PLUGIN_TRASH_UNAVAILABLE: "系统回收站当前不可用，操作已取消",
  BUILTIN_PLUGIN_IMMUTABLE: "内置插件由 Awaken 管理，不能替换、编辑或卸载",
  RUNTIME_GENERATION_IN_USE: "插件正在被运行任务使用，请等待或取消任务后重试",
  COMPONENT_NOT_SWITCHABLE: "此组件当前不可切换",
  MCP_TOOL_NOT_FOUND: "MCP 工具不存在或已经变化",
  MCP_SOURCE_INVALID: "MCP 源文件格式不符合要求",
  PACKAGE_NO_USABLE_COMPONENTS: "插件包没有可运行组件",
  PLUGIN_DISABLED: "插件已停用",
  PLUGIN_INVALID: "插件 ZIP 格式不符合要求",
  PLUGIN_STATE_CORRUPT: "插件状态文件损坏",
  PLUGIN_REQUEST_FAILED: "插件操作失败",
  PLUGIN_SOURCE_CHANGED: "插件来源在检查后已变化，请重新检查",
  PLUGIN_SKILL_ID_UNAVAILABLE: "插件 Skill 身份不可用，请刷新插件后重试",
  INVALID_REQUEST: "请求格式无效",
};

const POSIX_ABSOLUTE_PATH = /(?<!:)(?:\/[\w.@+~-]+){2,}/gu;
const WINDOWS_ABSOLUTE_PATH = /\b[A-Za-z]:\\[^\r\n,，。；;）)\]"']+/g;

export function pluginErrorCode(error: unknown): string | null {
  const code = runtimeRequestIssue(error).code;
  return code === "DESKTOP_REQUEST_FAILED" ? null : code;
}

export function pluginErrorMessage(error: unknown): string {
  const stripped = runtimeRequestIssue(error).message;
  const code = pluginErrorCode(error);
  const mapped = code
    ? PLUGIN_ERROR_MESSAGES[code] ?? (code.startsWith("VARIABLE_") ? "插件变量配置无效" : undefined)
    : undefined;
  if (mapped && code) return `${mapped}（${code}）`;

  if (/lstat|ENOENT|no such file|not exist/i.test(stripped)) {
    return "插件包不存在或已不可访问，请检查来源路径后重试";
  }

  const sanitized = stripped
    .replace(WINDOWS_ABSOLUTE_PATH, "本地路径")
    .replace(POSIX_ABSOLUTE_PATH, "本地路径")
    .trim();
  return sanitized || "插件操作失败";
}
