import { AWAKEN_ICON_SIZES } from "../../icons";
import type { AwakenIconProps } from "./AwakenIcon";

/**
 * Gallery 状态只覆盖「单个图标怎么渲染」；
 * 全套图标网格、光学审计和形状族由 AwakenIcon.demo.tsx 负责。
 */
export const AwakenIconStates = [
  { id: "default", label: "默认 16px", props: { name: "folder" } },
  { id: "concept", label: "按概念取用", props: { concept: "project-add", size: 16 } },
  { id: "min", label: "可读下限 12px", props: { name: "wrench", size: 12 } },
  { id: "compact", label: "紧凑 14px", props: { name: "subagent", size: 14 } },
  { id: "large", label: "24px", props: { name: "book-open", size: 24 } },
  { id: "brand", label: "品牌标记 24px", props: { name: "mcp-mark", size: 24 } },
  { id: "labelled", label: "带无障碍名", props: { name: "trash", label: "删除" } },
] satisfies Array<{ id: string; label: string; props: AwakenIconProps }>;

export const AWAKEN_ICON_DEMO_SIZES = AWAKEN_ICON_SIZES;
