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
  "more-actions": "dots-horizontal",
  "expand-all": "chevrons-up-down",
  "collapse-all": "chevrons-down-up",
  search: "magnifying-glass",
  refresh: "arrows-cw",
  settings: "cog",

  // 方向与披露
  disclose: "chevron-right",
  dropdown: "chevron-down",

  // 运行状态
  "step-pending": "circle",
  "step-done": "check",
  "select-all": "checks",
  "select-mode": "check-square",

  // 能力与来源
  tool: "wrench",
  skill: "skill",
  "skill-active": "skill-core",
  hook: "hook",
  connector: "connector",
  subagent: "subagent",
  "scheduled-task": "clock",

  /**
   * MCP 按尺寸分两路：
   * - `mcp-brand` 用官方标记，只在 ≥20px 的连接器卡片、MCP 设置等处使用；
   * - `mcp-source` 用 compact 伴随字形，用于 13/14px 的工具轨迹这类密集行。
   * 官方标记在小尺寸会粘连，这是标记结构决定的，不是缩放方式的问题。
   */
  "mcp-brand": "mcp-mark",
  "mcp-source": "mcp-compact",
} as const satisfies Record<string, AwakenIconName>;

export type AwakenIconConcept = keyof typeof AWAKEN_ICON_CONCEPTS;

export function iconForConcept(concept: AwakenIconConcept): AwakenIconName {
  return AWAKEN_ICON_CONCEPTS[concept];
}

export const AWAKEN_ICON_CONCEPT_NAMES = Object.keys(
  AWAKEN_ICON_CONCEPTS,
).sort() as AwakenIconConcept[];
