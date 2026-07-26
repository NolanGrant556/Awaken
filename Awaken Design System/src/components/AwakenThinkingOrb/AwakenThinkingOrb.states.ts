import type { AwakenThinkingOrbProps } from "./AwakenThinkingOrb";

export type AwakenThinkingOrbGalleryState = {
  id: string;
  label: string;
  description: string;
  props: AwakenThinkingOrbProps;
};

export const AwakenThinkingOrbStates: AwakenThinkingOrbGalleryState[] = [
  {
    id: "working",
    label: "Working",
    description: "粒子沿倾斜轨道持续运行",
    props: { state: "working", size: 64, "aria-label": "正在工作" },
  },
  {
    id: "searching",
    label: "Searching",
    description: "扫描经线掠过点阵球面",
    props: { state: "searching", size: 64, "aria-label": "正在搜索" },
  },
  {
    id: "solving",
    label: "Solving",
    description: "分带打散后重新归位",
    props: { state: "solving", size: 64, "aria-label": "正在求解" },
  },
  {
    id: "listening",
    label: "Listening",
    description: "波形穿过环形点阵",
    props: { state: "listening", size: 64, "aria-label": "正在聆听" },
  },
  {
    id: "composing",
    label: "Thinking / Composing",
    description: "Thinking 行内状态使用的多带波动，保持选定的 2.4× 速度",
    props: { state: "composing", size: 64, speed: 2.4, "aria-label": "正在思考" },
  },
  {
    id: "shaping",
    label: "Shaping",
    description: "轮廓在圆、三角和方形间变换",
    props: { state: "shaping", size: 64, "aria-label": "正在塑形" },
  },
];
