import {
  type ToolTraceAction,
  type ToolTraceDetailCard,
} from "@awaken/design-system";
import { describeGlobPattern } from "./run-narrative/tool-display";
import type { ToolDisplayState } from "../shared/toolStatus";
import { diffLineStats } from "./workbenchPresentation";

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
    if (description) return `运行 ${compactInlineLabel(description)}`;
    const command = firstStringArgument(args, ["command", "cmd", "script"]);
    if (command) return `运行 ${compactInlineLabel(redactShellText(command))}`;
    return "运行命令";
  }
  if (READ_TOOL_TITLE.test(title)) {
    const path = firstStringArgument(args, ["path", "file_path", "filePath"]);
    const query = firstStringArgument(args, ["query", "pattern", "url"]);
    const glob = firstStringArgument(args, ["glob"]);
    if (/search|grep|glob|find|搜索/i.test(title)) {
      const compactTitle = title.replace(/\s*\{\s*$/, "").trim();
      if (/^搜索/.test(compactTitle) && !/[*?[\]{}]/.test(compactTitle)) {
        return compactTitle;
      }
      const queryLabel = query
        ? /[*?[\]{}]/.test(query)
          ? describeGlobPattern(query)
          : `"${compactInlineLabel(query, 48)}"`
        : undefined;
      const scopeLabel = glob
        ? describeGlobPattern(glob)
        : path
          ? compactPathLabel(path)
          : undefined;
      if (queryLabel && scopeLabel) {
        return `搜索 ${queryLabel} · ${scopeLabel}`;
      }
      if (queryLabel) return `搜索 ${queryLabel}`;
      if (scopeLabel) return `搜索 ${scopeLabel}`;
      return "搜索文件";
    }
    if (path) {
      return `${/list|列出/i.test(title) ? "列出" : "读取"} ${compactPathLabel(path)}`;
    }
    return /list|列出/i.test(title) ? "列出文件" : "读取文件";
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
  return title.replace(/\s*\{\s*$/, "").trim() || "使用工具";
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
    const statusNeedsContext = Boolean(
      options.status && options.status !== "done",
    );
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
        ? { title: options.title.replace(/\s*\{\s*$/, "").trim() }
        : {}),
      detail:
        options.status === "active"
          ? compactToolContext(fallback)
          : statusNeedsContext
            ? compactToolFailureContext(fallback)
            : undefined,
      output: undefined,
      outputLabel: "工具输出",
      detailCard,
      kind: requestedMcpIdentity ? ("mcp" as const) : ("tool" as const),
    };
  }
  const commandOutput = tool.commandOutput?.trim();
  const result = tool.content?.trim();
  const active = !tool.status || tool.status === "active";
  const failed = Boolean(
    tool.status && tool.status !== "active" && tool.status !== "done",
  );
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
  const progress = active
    ? compactToolContext(fallback) ||
      compactToolContext(commandOutput) ||
      compactToolContext(result)
    : compactToolContext(result) ||
      compactToolContext(commandOutput) ||
      compactToolContext(fallback);
  const command = shellTool
    ? firstStringArgument(args, ["command", "cmd", "script"])
    : undefined;
  const knownReadableTool = READ_TOOL_TITLE.test(sourceTitle);
  const knownFileTool =
    FILE_TOOL_TITLE.test(sourceTitle) || Boolean(options.patch);
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
  const errorContent = tool.diagnostic
    ? JSON.stringify(redactSensitiveValue(tool.diagnostic), null, 2)
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
  if (options.patch) {
    if (options.patch.affectedPaths.length > 0) {
      sections.push({
        label: "文件",
        content: options.patch.affectedPaths.join("\n"),
        format: "text",
      });
    }
    sections.push({
      label: "变更",
      content: options.patch.patch,
      format: "diff",
    });
  }
  if (commandOutput) {
    sections.push({
      label: "stdout / stderr",
      content: redactShellText(commandOutput),
      format: "code",
    });
  } else if (result) {
    sections.push({
      label: mcpProvider ? "结果" : knownReadableTool ? "匹配摘要" : "结果",
      content: result,
      format: "text",
    });
  }
  if (errorContent) {
    sections.push({
      label: failed ? "错误与恢复信息" : "执行环境",
      content: errorContent,
      format: "json",
      tone: failed ? "danger" : "muted",
    });
  }
  const detailCard = detailCardFromSections(
    kind === "terminal"
      ? "Shell 详情"
      : mcpProvider
        ? `${providerLabel || "MCP"} 详情`
        : "工具详情",
    sections,
  );
  const patchStats = options.patch
    ? diffLineStats(options.patch.patch)
    : undefined;
  const lineChangeDetail = patchStats
    ? `+${patchStats.added} −${patchStats.removed} 行`
    : undefined;
  const preparationDetail =
    active && /^\+\d+(?: −\d+)? 行$/.test(progress ?? "")
      ? progress
      : undefined;
  return {
    title: naturalToolTitle(sourceTitle, kind, args),
    detail:
      providerLabel ||
      (failed
        ? compactToolFailureContext(progress)
        : lineChangeDetail || preparationDetail),
    output,
    outputLabel:
      kind === "terminal" ? "终端输出" : mcpProvider ? "MCP 输出" : "工具输出",
    outputTruncated: tool.commandOutputTruncated,
    detailCard,
    kind: kind as "tool" | "mcp" | "terminal",
  };
}

export type ActivitySummaryInput = {
  kind: "tool" | "skill" | "terminal" | "mcp";
  title: string;
  status: ToolDisplayState;
  fileCount?: number;
};

export function activityBatchSummary(actions: ActivitySummaryInput[]): {
  label: string;
  detail?: string;
} {
  const active = actions.some((action) => action.status === "active");
  const failed = actions.filter(
    (action) => action.status !== "active" && action.status !== "done",
  ).length;
  const commandCount = actions.filter(
    (action) => action.kind === "terminal",
  ).length;
  const skillActions = actions.filter((action) => action.kind === "skill");
  const checkedCount = actions.filter((action) =>
    READ_TOOL_TITLE.test(action.title),
  ).length;
  const editedFiles = actions.reduce(
    (count, action) => count + (action.fileCount || 0),
    0,
  );
  let label: string;
  if (commandCount === actions.length) {
    label = active
      ? `正在运行 ${commandCount} 条命令`
      : `运行了 ${commandCount} 条命令`;
  } else if (checkedCount === actions.length) {
    label = active
      ? `正在检查 ${checkedCount} 项`
      : `检查了 ${checkedCount} 项`;
  } else if (
    editedFiles > 0 &&
    actions.every((action) => FILE_TOOL_TITLE.test(action.title))
  ) {
    label = active
      ? `正在编辑 ${editedFiles} 个文件`
      : `编辑了 ${editedFiles} 个文件`;
  } else if (skillActions.length > 0) {
    const skillName = skillActions[0].title.replace(/^使用\s*/i, "");
    const toolCount = actions.length - skillActions.length;
    const toolSuffix = toolCount > 0 ? ` · ${toolCount} 个工具` : "";
    label = `${active ? "正在使用" : "使用了"} ${skillName}${toolSuffix}`;
  } else {
    label = active
      ? `正在运行 ${actions.length} 个工具`
      : `使用了 ${actions.length} 个工具`;
  }
  return { label, detail: failed > 0 ? `${failed} 个异常` : undefined };
}
