import {
  type ToolTraceAction,
  type ToolTraceDetailCard,
} from "@awaken/design-system";
import {
  describeGlobPattern,
  readLineRangeLabel,
  searchActionTitle,
} from "./run-narrative/tool-display";
import type { ToolDisplayState } from "../shared/toolStatus";
import { diffLineStats } from "./workbenchPresentation";
import { toolResultPatch } from "./tracePatch";

export function rawToolId(selectionId: string): string {
  if (selectionId.startsWith("tool:")) {
    return selectionId.slice("tool:".length);
  }
  if (selectionId.startsWith("explore:")) {
    return selectionId.slice("explore:".length);
  }
  return selectionId;
}

export function toolRecordIdForNarrativeItem(item: {
  id: string;
  toolCallId?: string;
}): string {
  return item.toolCallId || rawToolId(item.id);
}

export function firstContentLine(value?: string): string | undefined {
  return value
    ?.split("\n")
    .map((line) => line.trim())
    .find(Boolean);
}

export function compactToolContext(value?: string): string | undefined {
  const line = firstContentLine(value);
  if (!line || line === "{" || line === "[" || /^[\[{]\s*"/.test(line))
    return undefined;
  return line;
}

export type ToolTraceRecordView = {
  title?: string;
  content?: string;
  args?: string;
  commandOutput?: string;
  commandOutputTruncated?: boolean;
  status?: ToolDisplayState;
  diagnostic?: Record<string, unknown>;
  providerId?: string;
  providerSource?: string;
};

export type ToolTracePatchView = {
  title?: string;
  patch: string;
  summary?: string;
  affectedPaths: string[];
  truncated?: boolean;
};

export type ToolTraceActionCacheEntry = {
  inputs: readonly unknown[];
  action: ToolTraceAction;
};

export const SENSITIVE_TOOL_KEY =
  /(?:password|token|secret|api[_-]?key|authorization|cookie)/i;

export const SHELL_TOOL_TITLE = /(?:bash|shell|terminal|command|命令)/i;

export const READ_TOOL_TITLE =
  /(?:read|读取|list|列出|search|搜索|grep|glob|find)/i;

export const FILE_TOOL_TITLE =
  /(?:write|edit|patch|create|delete|move|copy|写入|编辑|修改|创建|删除)/i;

export function parseToolArguments(value?: string): unknown {
  if (!value?.trim()) return undefined;
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

export function redactSensitiveValue(value: unknown, key = ""): unknown {
  if (SENSITIVE_TOOL_KEY.test(key)) return "••••";
  if (Array.isArray(value))
    return value.map((entry) => redactSensitiveValue(entry));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(
        ([childKey, childValue]) => [
          childKey,
          redactSensitiveValue(childValue, childKey),
        ],
      ),
    );
  }
  return value;
}

export function redactShellText(value: string): string {
  return value.replace(
    /\b([A-Z0-9_]*(?:PASSWORD|TOKEN|SECRET|API_KEY|AUTHORIZATION|COOKIE)[A-Z0-9_]*)=(?:"[^"]*"|'[^']*'|[^\s]+)/gi,
    "$1=••••",
  );
}

export function argumentRecord(
  value?: string,
): Record<string, unknown> | undefined {
  const parsed = parseToolArguments(value);
  return parsed && typeof parsed === "object" && !Array.isArray(parsed)
    ? (parsed as Record<string, unknown>)
    : undefined;
}

export function firstStringArgument(
  args: Record<string, unknown> | undefined,
  keys: string[],
): string | undefined {
  for (const key of keys) {
    const value = args?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return undefined;
}

export function firstNumberArgument(
  args: Record<string, unknown> | undefined,
  keys: string[],
): number | undefined {
  for (const key of keys) {
    const value = args?.[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim()) {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return undefined;
}

export function thoughtTraceTitle(input: {
  kind?: string;
  status?: string;
  phase?: string;
  text?: string;
  durationMs?: number;
}): string {
  if (input.kind === "planning" && input.phase) {
    return input.text?.trim() || "思考 中";
  }
  if (input.status === "active") return "思考 中";
  if (input.status === "error") return "思考 已中断";
  const durationMs = input.durationMs;
  const seconds =
    typeof durationMs === "number" &&
    Number.isFinite(durationMs) &&
    durationMs >= 1500
      ? Math.max(1, Math.round(durationMs / 1000))
      : undefined;
  return seconds ? `思考 ${seconds} 秒` : "思考 片刻";
}

export function safeFileToolArguments(
  args: Record<string, unknown> | undefined,
): Record<string, string> | undefined {
  const path = firstStringArgument(args, [
    "path",
    "file_path",
    "filePath",
    "target",
  ]);
  return path ? { path } : undefined;
}

export function compactInlineLabel(value: string, maxLength = 72): string {
  const compact = value.replace(/\s+/g, " ").trim();
  if (compact.length <= maxLength) return compact;
  return `${compact.slice(0, Math.max(1, maxLength - 1)).trimEnd()}…`;
}

export function compactPathLabel(value: string): string {
  const normalized = value.trim().replace(/[\\/]+$/, "");
  if (!normalized) return value.trim();
  const segments = normalized.split(/[\\/]/).filter(Boolean);
  return compactInlineLabel(segments.at(-1) || normalized);
}

export function compactToolFailureContext(value?: string): string | undefined {
  const raw = value?.trim();
  if (!raw) return undefined;
  if (/permission|denied|outside writable roots?|sensitive path/i.test(raw)) {
    return "权限被拒绝";
  }
  if (/timed?\s*out|timeout/i.test(raw)) return "执行超时";
  if (/not found|no such file|enoent/i.test(raw)) return "目标不存在";
  if (/cancelled|canceled/i.test(raw)) return "已取消";
  if (/interrupted/i.test(raw)) return "已中断";
  if (/subagent_invalid_input/.test(raw)) return "子任务参数不完整或无效";
  const withoutQuotedPaths = redactShellText(raw).replace(
    /(["'])(?:\/|[A-Za-z]:\\)[^"'\n]+\1/g,
    "$1…$1",
  );
  return compactInlineLabel(withoutQuotedPaths);
}

export function detailCardFromSections(
  title: string,
  sections: ToolTraceDetailCard["sections"],
): ToolTraceDetailCard | undefined {
  const visible = sections.filter((section) => section.content.trim());
  if (visible.length === 0) return undefined;
  return {
    title,
    sections: visible,
    copyText: visible
      .map((section) => `${section.label}\n${section.content}`)
      .join("\n\n"),
  };
}

export function compactUrlLabel(value?: string): string | undefined {
  const raw = value?.trim();
  if (!raw) return undefined;
  try {
    const withScheme = /^[a-z][a-z0-9+.-]*:/i.test(raw)
      ? raw
      : `https://${raw}`;
    const parsed = new URL(withScheme);
    const path =
      parsed.pathname === "/" ? "" : parsed.pathname.replace(/\/$/, "");
    return compactInlineLabel(`${parsed.host}${path}`, 48);
  } catch {
    return compactInlineLabel(raw, 48);
  }
}

function titled(verb: string, fact?: string): string {
  const rest = fact?.trim();
  return rest ? `${verb} ${rest}` : verb;
}

const COMPUTER_ACTION_LABELS: Record<string, string> = {
  click: "点击",
  double_click: "双击",
  scroll: "滚动",
  drag: "拖拽",
  key: "按键",
  hotkey: "快捷键",
  type_text: "输入",
  move_cursor: "移动指针",
  wait: "等待",
};

function namedBuiltinTitle(
  name: string,
  args?: Record<string, unknown>,
): string | undefined {
  const path = firstStringArgument(args, [
    "path",
    "file_path",
    "filePath",
    "target",
  ]);
  switch (name) {
    case "computer_observe": {
      const scope = firstStringArgument(args, ["scope"]);
      return titled(
        "观察",
        scope === "window" ? "窗口" : scope === "desktop" ? "桌面" : "屏幕",
      );
    }
    case "computer_act":
    case "computer_action": {
      const intent = firstStringArgument(args, ["intent"]);
      const action = firstStringArgument(args, ["action"]);
      if (intent) return titled("操作", compactInlineLabel(intent, 32));
      if (action)
        return titled(
          "操作",
          COMPUTER_ACTION_LABELS[action] ?? compactInlineLabel(action, 24),
        );
      return "操作 桌面";
    }
    case "computer_verify":
      return titled(
        "核对",
        firstStringArgument(args, ["expected_effect"]) || "结果",
      );
    case "webfetch":
    case "browser_navigate":
      return titled(
        "打开",
        compactUrlLabel(firstStringArgument(args, ["url", "href"])) || "页面",
      );
    case "browser_snapshot":
      return "查看 页面";
    case "browser_click":
      return titled(
        "点击",
        firstStringArgument(args, ["ref"]) || "页面",
      );
    case "browser_type":
      return titled(
        "输入",
        compactInlineLabel(firstStringArgument(args, ["text"]) ?? "文字", 32),
      );
    case "browser_wait": {
      const seconds = firstNumberArgument(args, ["seconds"]);
      return seconds ? titled("等待", `${seconds} 秒`) : "等待 页面";
    }
    case "browser_extract":
      return "提取 页面";
    case "calculator":
      return titled(
        "计算",
        compactInlineLabel(
          firstStringArgument(args, ["expression"]) ?? "算式",
          32,
        ),
      );
    case "memory":
      return titled(
        "记忆",
        firstStringArgument(args, ["action", "query", "name"]) || "记录",
      );
    case "artifact_publish":
      return titled("发布", path ? compactPathLabel(path) : "文件");
    case "git_remote": {
      const operation = firstStringArgument(args, ["operation"]);
      const url = compactUrlLabel(firstStringArgument(args, ["url"]));
      if (operation === "clone") return titled("克隆", url || "仓库");
      return titled("查看", url || "远程");
    }
    case "trash_file":
      return titled("删除", path ? compactPathLabel(path) : "文件");
    case "tool_find":
      return titled(
        "查找",
        firstStringArgument(args, ["query"]) || "工具",
      );
    case "tool_result_read":
      return "读取 上次结果";
    case "thread_history_read":
      return "读取 会话";
    case "archived_history":
      return "读取 归档";
    case "dependency_install":
      return titled(
        "安装",
        firstStringArgument(args, ["name", "package", "id"]) || "依赖",
      );
    case "configure_hook":
      return "配置 钩子";
    case "configure_acp_runner":
      return "配置 外部助手";
    case "delegate_external_agent":
      return titled(
        "调用",
        firstStringArgument(args, ["runner"]) || "外部助手",
      );
    case "subagent":
      return titled(
        "子任务",
        firstStringArgument(args, ["description", "action"]) || "操作",
      );
    case "plugin":
      return titled(
        "使用",
        firstStringArgument(args, ["name", "plugin"]) || "插件",
      );
    case "skill_install":
      return titled(
        "安装",
        firstStringArgument(args, ["skill_id", "name"]) || "技能",
      );
    case "skill_resource_read":
      return titled("读取", path ? compactPathLabel(path) : "技能文件");
    case "skill_resource_list":
      return "列出 技能文件";
    case "skill_package_export":
      return "发布 技能";
    case "skill":
      return titled(
        "使用",
        firstStringArgument(args, ["skill_id", "name"]) || "技能",
      );
    default:
      return undefined;
  }
}

function mcpToolTitle(title: string): string {
  const compact = title.replace(/\s*\{\s*$/, "").trim();
  if (/^[\u4e00-\u9fff]/.test(compact)) return compact;
  const parts = compact.split("__").filter(Boolean);
  const toolName = parts.at(-1)?.replace(/^mcp$/i, "") || compact;
  return titled("调用", compactInlineLabel(toolName.replace(/_/g, " "), 40));
}

export function naturalToolTitle(
  title: string,
  kind: "tool" | "mcp" | "terminal",
  args?: Record<string, unknown>,
): string {
  if (kind === "terminal") {
    const description = firstStringArgument(args, [
      "description",
      "label",
      "summary",
    ]);
    if (description)
      return `运行 ${compactInlineLabel(description.replace(/^运行\s*/, ""))}`;
    const command = firstStringArgument(args, ["command", "cmd", "script"]);
    if (command) return `运行 ${compactInlineLabel(redactShellText(command))}`;
    return "运行命令";
  }
  const compactName = title.replace(/\s*\{\s*$/, "").trim();
  const named = namedBuiltinTitle(compactName.toLowerCase(), args);
  if (named) return named;
  if (kind === "mcp" || /^mcp__/i.test(compactName)) {
    return mcpToolTitle(compactName);
  }
  if (READ_TOOL_TITLE.test(title)) {
    const compactTitle = title.replace(/\s*\{\s*$/, "").trim();
    const path = firstStringArgument(args, ["path", "file_path", "filePath"]);
    const query = firstStringArgument(args, ["query", "pattern", "url"]);
    const glob = firstStringArgument(args, ["glob"]);
    if (/websearch|网页/i.test(title)) {
      if (query) return `搜索网页 ${compactInlineLabel(query, 48)}`;
      if (/^搜索网页/.test(compactTitle)) return compactTitle;
      return "搜索网页";
    }
    if (/search|grep|glob|find|搜索/i.test(title)) {
      const scopeLabel = glob
        ? describeGlobPattern(glob)
        : path
          ? compactPathLabel(path)
          : undefined;
      if (query || scopeLabel) return searchActionTitle(query, scopeLabel);
      if (/^搜索/.test(compactTitle) && !/[*?[\]{}]/.test(compactTitle)) {
        return compactTitle;
      }
      return "搜索文件";
    }
    const verb = /list|列出/i.test(title) ? "列出" : "读取";
    if (path) {
      const range = readLineRangeLabel(
        firstNumberArgument(args, ["offset"]),
        firstNumberArgument(args, ["limit"]),
      );
      return `${verb} ${compactPathLabel(path)}${range ? ` ${range}` : ""}`;
    }
    if (/^(?:读取|列出)/.test(compactTitle)) return compactTitle;
    return verb === "列出" ? "列出文件" : "读取文件";
  }
  if (FILE_TOOL_TITLE.test(title)) {
    const target = firstStringArgument(args, [
      "path",
      "file_path",
      "filePath",
      "target",
    ]);
    if (
      !target &&
      /^(?:写入|编辑|修改|创建|删除|移动|复制)(?:文件|\s)/.test(title)
    ) {
      return compactInlineLabel(title);
    }
    const action = /delete|删除/i.test(title)
      ? "删除"
      : /move|移动/i.test(title)
        ? "移动"
        : /copy|复制/i.test(title)
          ? "复制"
          : /write|create|写入|创建/i.test(title)
            ? "写入"
            : "编辑";
    if (target) return `${action} ${compactPathLabel(target)}`;
    return `${action}文件`;
  }
  if (title === "subagent") return "子任务 操作";
  if (/^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$/i.test(compactName)) {
    return titled("使用", compactInlineLabel(compactName.replace(/_/g, " "), 40));
  }
  return compactName || "使用工具";
}

export function toolTracePresentation(
  tool: ToolTraceRecordView | undefined,
  fallback?: string,
  options: {
    title?: string;
    patch?: ToolTracePatchView;
    allowLegacyFallbackOutput?: boolean;
    status?: ToolDisplayState;
  } = {},
) {
  const requestedTitle = options.title?.trim() || "";
  const requestedMcpIdentity = /^mcp__.+__/i.test(requestedTitle);
  if (!tool) {
    const legacyContent = fallback?.trim() || undefined;
    const detailCard =
      options.allowLegacyFallbackOutput && legacyContent
        ? detailCardFromSections("工具详情", [
            {
              label:
                options.status &&
                options.status !== "active" &&
                options.status !== "done"
                  ? "错误与恢复信息"
                  : "结果",
              content: legacyContent,
              format: "text",
              ...(options.status &&
              options.status !== "active" &&
              options.status !== "done"
                ? { tone: "danger" as const }
                : {}),
            },
          ])
        : undefined;
    return {
      ...(options.title
        ? {
            title: naturalToolTitle(
              options.title.replace(/\s*\{\s*$/, "").trim(),
              requestedMcpIdentity ? "mcp" : "tool",
            ),
          }
        : {}),
      detail: undefined,
      output: undefined,
      outputLabel: "工具输出",
      detailCard,
      kind: requestedMcpIdentity ? ("mcp" as const) : ("tool" as const),
    };
  }
  const patch = options.patch ?? toolResultPatch(tool);
  const commandOutput = tool.commandOutput?.trim();
  const result =
    tool.content?.trim() === "Tool returned empty content"
      ? undefined
      : tool.content?.trim();
  const active = !tool.status || tool.status === "active";
  const output = active
    ? commandOutput || undefined
    : commandOutput || result || undefined;
  const providerSource = tool.providerSource?.trim().toLowerCase() || "";
  const sourceTitle = options.title || tool.title || "使用工具";
  const providerId = tool.providerId?.trim() || "";
  const mcpProvider =
    providerSource === "mcp" ||
    providerSource === "connector" ||
    /^(?:mcp|connector):/i.test(providerId) ||
    /^mcp__.+__/i.test(sourceTitle);
  const args = argumentRecord(tool.args);
  const shellTool =
    Boolean(commandOutput) || SHELL_TOOL_TITLE.test(sourceTitle);
  const kind = mcpProvider ? "mcp" : shellTool ? "terminal" : "tool";
  const providerLabel = mcpProvider
    ? providerId.replace(/^(mcp|connector):/i, "")
    : undefined;
  const command = shellTool
    ? firstStringArgument(args, ["command", "cmd", "script"])
    : undefined;
  const knownReadableTool = READ_TOOL_TITLE.test(sourceTitle);
  const knownFileTool = FILE_TOOL_TITLE.test(sourceTitle) || Boolean(patch);
  const sanitizedArgs = mcpProvider
    ? args
      ? redactSensitiveValue(args)
      : undefined
    : knownFileTool
      ? safeFileToolArguments(args)
      : args
        ? redactSensitiveValue(args)
        : undefined;
  const argumentContent = sanitizedArgs
    ? JSON.stringify(sanitizedArgs, null, 2)
    : "";
  const sections: ToolTraceDetailCard["sections"] = [];
  if (command) {
    sections.push({
      label: "命令",
      content: redactShellText(command),
      format: "code",
      tone: "command",
    });
  } else if (
    (mcpProvider || knownReadableTool || knownFileTool) &&
    argumentContent
  ) {
    sections.push({
      label: mcpProvider ? "脱敏参数" : "目标与参数",
      content: argumentContent,
      format: "json",
    });
  }
  if (patch) {
    sections.push({
      label: "变更",
      content: patch.patch,
      format: "diff",
    });
  }
  if (commandOutput) {
    sections.push({
      label: "stdout / stderr",
      content: redactShellText(commandOutput),
      format: "code",
    });
  } else if (result && !patch) {
    sections.push({
      label: mcpProvider
        ? "结果"
        : tool.title?.toLowerCase() === "read"
          ? "读取记录"
          : knownReadableTool
            ? "匹配摘要"
            : "结果",
      content: result,
      format: knownFileTool && /^[\[{]/.test(result) ? "json" : "text",
    });
  }
  if (shellTool && !active && !commandOutput && !result) {
    sections.push({
      label: "输出",
      content: "命令已结束，无输出。",
      format: "text",
    });
  }
  // Readable results lead; JSON parameters stay folded on demand.
  if (mcpProvider || patch)
    sections.sort(
      (a, b) => Number(a.format === "json") - Number(b.format === "json"),
    );
  const detailCard = detailCardFromSections(
    kind === "terminal"
      ? "Shell 详情"
      : mcpProvider
        ? `${providerLabel || "MCP"} 详情`
        : "工具详情",
    sections,
  );
  return {
    title: naturalToolTitle(sourceTitle, kind, args),
    detail: providerLabel,
    output,
    outputLabel:
      kind === "terminal" ? "终端输出" : mcpProvider ? "MCP 输出" : "工具输出",
    outputTruncated: tool.commandOutputTruncated || patch?.truncated,
    detailCard,
    kind: kind as "tool" | "mcp" | "terminal",
  };
}

export type ActivitySummaryInput = {
  id?: string;
  kind: "tool" | "skill" | "terminal" | "mcp";
  title: string;
  status: ToolDisplayState;
  toolName?: string;
  paths?: string[];
  patch?: ToolTracePatchView;
  fileCount?: number;
};

export type TraceOperation = "read" | "search" | "edit" | "command" | "other";
export function traceOperation(name?: string, kind?: string): TraceOperation {
  if (kind === "mcp" || kind === "skill") return "other";
  switch (name?.toLowerCase()) {
    case "read":
      return "read";
    case "grep":
    case "glob":
    case "websearch":
    case "webfetch":
      return "search";
    case "write":
    case "edit":
    case "apply_patch":
    case "patch":
      return "edit";
    case "bash":
    case "shell":
    case "command":
      return "command";
    default:
      return "other";
  }
}

/** Lexical identity only. File opening still goes through Main project validation. */
export function normalizeTracePath(path: string): string {
  const value = path.replace(/\\/g, "/");
  const absolute = value.startsWith("/");
  const parts: string[] = [];
  for (const part of value.split("/")) {
    if (!part || part === ".") continue;
    if (part === ".." && parts.length && parts.at(-1) !== "..") parts.pop();
    else parts.push(part);
  }
  return `${absolute ? "/" : ""}${parts.join("/")}`;
}

export function activityBatchSummary(actions: ActivitySummaryInput[]): {
  label: string;
  detail?: string;
  added?: number;
  removed?: number;
} {
  const unique = [
    ...new Map(
      actions.map((action, index) => [action.id ?? `legacy:${index}`, action]),
    ).values(),
  ];
  const completed = unique.filter((action) => action.status === "done");
  const counts = { read: 0, search: 0, edit: 0, command: 0, other: 0 };
  const readPaths = new Set<string>();
  const editPaths = new Set<string>();
  const namedOther = new Map<string, { label: string; count: number }>();
  let added = 0,
    removed = 0,
    completeDiff = true;
  for (const action of completed) {
    const operation = traceOperation(action.toolName, action.kind);
    if (operation === "read" || operation === "edit") {
      const paths = action.paths?.filter(Boolean) ?? [];
      if (paths.length)
        paths.forEach((path) =>
          (operation === "read" ? readPaths : editPaths).add(
            normalizeTracePath(path),
          ),
        );
      else counts[operation]++;
      if (operation === "edit") {
        if (!action.patch || action.patch.truncated) completeDiff = false;
        else {
          const stats = diffLineStats(action.patch.patch);
          added += stats.added;
          removed += stats.removed;
        }
      }
    } else if (
      operation === "other" &&
      (action.toolName || action.kind === "skill")
    ) {
      const key = action.toolName || action.title;
      const previous = namedOther.get(key);
      namedOther.set(key, {
        label: action.title,
        count: (previous?.count ?? 0) + 1,
      });
    } else counts[operation]++;
  }
  const parts = [
    editPaths.size ? `修改 ${editPaths.size} 个文件` : "",
    counts.edit ? `修改 ${counts.edit} 次` : "",
    readPaths.size ? `探索 ${readPaths.size} 个文件` : "",
    counts.read ? `探索 ${counts.read} 次` : "",
    counts.search ? `搜索 ${counts.search} 次` : "",
    counts.command ? `运行 ${counts.command} 条命令` : "",
    ...[...namedOther.values()]
      .slice(0, 3)
      .map(({ label, count }) => `${label} ${count} 次`),
    namedOther.size > 3
      ? `其他 ${[...namedOther.values()].slice(3).reduce((sum, item) => sum + item.count, 0)} 次活动`
      : "",
    counts.other ? `使用 ${counts.other} 次工具或技能` : "",
  ].filter(Boolean);
  const failed = unique.filter(
    (action) => action.status !== "done" && action.status !== "active",
  ).length;
  const showDiff = completeDiff && (added > 0 || removed > 0);
  return {
    label:
      parts.join("、") ||
      (unique.some((action) => action.status === "active")
        ? "执行过程"
        : "活动记录"),
    added: showDiff ? added : undefined,
    removed: showDiff ? removed : undefined,
    detail: failed ? `${failed} 项异常` : undefined,
  };
}
