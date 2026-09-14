/**
 * Awaken 概念 → 图标名 的唯一映射。
 *
 * 这张表存在的意义是：每个「X 用哪个图标」的问题只有一个答案，
 * 并且在产品长大之后仍然只有一个答案。没有它，一套图标会慢慢长出
 * 两个表示同一件事的图标，那时候它就不再是系统了。
 *
 * 产品代码应当优先引用概念而不是图形名：概念是稳定的语义 API，
 * 图形名属于图标层，可以在不动调用点的前提下替换。
 */
import type { AwakenIconName } from "./registry";

export const AWAKEN_ICON_CONCEPTS = {
  // 会话与项目
  "thread-new": "chat-bubble-plus",
  project: "folder",
  "project-add": "folder-plus",
  "folder-expanded": "folder-open",

  // 通用动作
  add: "plus",
  close: "x",
  collapse: "minus",
  delete: "trash",
  archive: "archive",
  "more-actions": "dots-horizontal",
  "expand-all": "chevrons-up-down",
  "collapse-all": "chevrons-down-up",
  search: "magnifying-glass",
  refresh: "arrows-cw",
  settings: "cog",
  undo: "undo",

  // 方向与披露
  disclose: "chevron-right",
  dropdown: "chevron-down",

  // Composer
  send: "arrow-up",
  stop: "stop",
  "context-file": "file-list",
  "computer-use": "display-pointer",
  back: "chevron-left",
  copy: "copy",
  "file-comment": "comment-lines",

  // 运行状态。迁移前这一族是散的：错误有四个同义图标，unsupported 与 error
  // 共用一个，loading 有两个。现在每个状态只有一个图形。
  "step-pending": "circle",
  "step-running": "spinner",
  "step-done": "check",
  success: "check-circle",
  failure: "x-circle",
  error: "exclamation-circle",
  warning: "exclamation-triangle",
  unsupported: "question-circle",
  cancelled: "minus-circle",
  denied: "slash-circle",
  interrupted: "stop-circle",
  blocked: "shield-x",
  "select-all": "checks",
  "select-mode": "check-square",

  // 设置页导航
  "settings-model": "chip",
  "settings-memory": "brain",
  "settings-appearance": "color-mode",
  "settings-pets": "paw",
  "settings-acp": "robot",

  // 主题三选
  "theme-light": "sun",
  "theme-dark": "moon",
  "theme-system": "display",

  // 账户与供应商
  user: "person",
  usage: "gauge",
  balance: "credit-card",
  "api-key": "key",
  reveal: "eye",
  conceal: "eye-slash",
  "provider-link": "arrow-right-up",

  // 设置页动作
  "navigate-back": "arrow-left",
  "external-open": "external-link",
  save: "floppy-disc",
  trusted: "shield-check",
  network: "wifi",
  "collapse-up": "chevron-up",
  restart: "arrow-cw",
  "update-available": "arrow-circle-down",
  "import": "upload",
  history: "history",

  // 记忆主题
  "memory-feedback": "chat-bubble",
  "memory-reference": "book-open",

  // ACP runner 品类
  "runner-code": "brackets-square",
  "runner-lightning": "lightning",

  // 文件类型
  "file-generic": "file",
  "file-text": "file-list",
  "file-markdown": "file-markdown",
  "file-code": "file-code",
  "file-data": "file-data",
  "file-image": "file-image",
  "file-archive": "file-archive",
  "file-audio": "file-audio",
  "file-video": "file-video",
  "file-pdf": "file-pdf",
  spreadsheet: "table",
  presentation: "display-play",
  symlink: "symlink",
  "folder-group": "folders",
  "locate-folder": "folder-search",

  // 预览与视图
  "zoom-in": "magnifying-glass-plus",
  "zoom-out": "magnifying-glass-minus",
  "fit-page": "rect-magnifying-glass",
  "expand-full": "corners-out",
  "exit-full": "corners-in",
  "rotate-ccw": "arrow-ccw",
  "rotate-cw": "arrow-cw",
  edit: "pencil",
  "wrap-text": "paragraph",
  forward: "arrow-right",
  filter: "funnel",

  // 布局与窗口
  "toggle-left-panel": "layout-sidebar-left",
  "toggle-right-panel": "layout-sidebar-right",
  "open-with": "window",
  browser: "globe",

  // 媒体
  "media-image": "image",
  "media-play": "play",
  "media-video": "video-camera",

  // 连接器与接管
  "mcp-server": "server",
  "end-takeover": "plug-slash",
  conversation: "comment",
  "capability-package": "package",

  // 能力与来源
  tool: "wrench",
  "tool-terminal": "terminal",
  skill: "book-open",
  "skill-active": "book-open",
  plugin: "at-sign",
  "plugin-default": "at-sign",
  hook: "hook",
  connector: "mcp-mark",
  subagent: "subagent",
  "scheduled-task": "clock",

  /** MCP 分类、卡片与工具轨迹统一使用同一官方标记。 */
  "mcp-brand": "mcp-mark",
  "mcp-source": "mcp-mark",
} as const satisfies Record<string, AwakenIconName>;

export type AwakenIconConcept = keyof typeof AWAKEN_ICON_CONCEPTS;

export function iconForConcept(concept: AwakenIconConcept): AwakenIconName {
  return AWAKEN_ICON_CONCEPTS[concept];
}

export const AWAKEN_ICON_CONCEPT_NAMES = Object.keys(
  AWAKEN_ICON_CONCEPTS,
).sort() as AwakenIconConcept[];
