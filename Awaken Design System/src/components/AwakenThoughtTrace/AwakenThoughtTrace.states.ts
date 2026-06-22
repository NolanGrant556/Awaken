import type { ComponentProps } from "react";
import type { AwakenThoughtTrace } from "./AwakenThoughtTrace";

export type AwakenThoughtTraceState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenThoughtTrace>>;
};

export const AwakenThoughtTraceStates: AwakenThoughtTraceState[] = [
  {
    id: "active-open",
    label: "展开思考",
    props: {
      text: "先阅读组件实现，再根据 props 判断 demo 和 README 应该描述的真实职责。",
      status: "active",
      label: "思考",
      durationLabel: "12s",
      defaultOpen: true,
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "done-collapsed",
    label: "完成折叠",
    props: {
      text: "已确认该思考段只作为执行流中的可折叠记录展示。",
      status: "done",
      label: "思考",
      durationLabel: "28s",
      defaultOpen: false,
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "error",
    label: "思考异常",
    props: {
      text: "读取运行事件时遇到异常，父级可以将该段标记为 error 并保留文本。",
      status: "error",
      label: "思考",
      durationLabel: "失败",
      defaultOpen: true,
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "long-content",
    label: "长轨迹",
    props: {
      text:
        "这是一段较长的思考轨迹，用于说明模型如何把组件实现、实际引用和 Gallery 展示串起来判断用途。父级仍然负责决定是否展示完整内容，本组件只负责折叠容器和文本排版。",
      status: "active",
      label: "规划",
      durationLabel: "1m 06s",
      defaultOpen: true,
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
];
