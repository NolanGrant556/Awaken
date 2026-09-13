import type { ComponentProps } from "react";
import type { AwakenThoughtTrace } from "./AwakenThoughtTrace";

export type AwakenThoughtTraceState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenThoughtTrace>>;
};

export const AwakenThoughtTraceStates: AwakenThoughtTraceState[] = [
  {
    id: "waiting-first-output",
    label: "等待首个输出（Desktop 提交后）",
    props: {
      text: "",
      status: "active",
      theme: "light",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "active-open",
    label: "运行中（正文默认收起）",
    props: {
      text: "I’ll inspect the current implementation and compare it with the Gallery state.",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "active-manual-open",
    label: "运行中手动展开",
    props: { text: "用户展开后，这段思考正文会继续流式更新。", status: "active", defaultOpen: true },
  },
  {
    id: "done-collapsed",
    label: "已完成（默认折叠）",
    props: {
      text: "The reasoning summary has completed and remains available in the execution narrative.",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "done-manual-open",
    label: "用户手动展开",
    props: {
      text: "The completed reasoning summary remains available after the user expands it.",
      status: "done",
      defaultOpen: true,
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "long-content",
    label: "长内容",
    props: {
      text:
        "I need to compare the runtime event, the narrative projector, the Desktop renderer props, and the Design System state before deciding whether the Gallery accurately represents the current product behavior.",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
];
