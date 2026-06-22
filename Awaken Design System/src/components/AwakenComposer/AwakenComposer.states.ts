import type { ComponentProps } from "react";
import type { AwakenComposer } from "./AwakenComposer";

export type AwakenComposerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposer>>;
};

const noop = () => {};

export const AwakenComposerStates: AwakenComposerState[] = [
  {
    id: "plan-empty",
    label: "计划模式空输入",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      onAddContext: noop,
      mode: "Plan",
      onModeChange: noop,
      model: { id: "gpt-5", label: "GPT-5" },
      models: [
        { id: "gpt-5", label: "GPT-5" },
        { id: "local-coder", label: "Local Coder" },
      ],
      onModelChange: noop,
      placeholder: "描述你想规划的任务...",
      disabled: false,
      isReady: false,
      submitMode: "send",
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
  {
    id: "act-ready",
    label: "执行模式可发送",
    props: {
      value: "检查当前 diff，并修复 states 里的占位 props。",
      mode: "Act",
      isReady: true,
      placeholder: "描述要执行的改动...",
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
  {
    id: "running",
    label: "运行中可停止",
    props: {
      value: "正在运行设计系统文档补全任务。",
      mode: "Act",
      isReady: true,
      submitMode: "stop",
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
  {
    id: "disabled",
    label: "输入禁用",
    props: {
      value: "等待运行时连接后才能继续输入。",
      disabled: true,
      isReady: false,
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
  {
    id: "long-draft",
    label: "长任务草稿",
    props: {
      value: "请阅读这些组件的真实 props 和渲染结构，补全中文用途、使用场景、父子责任边界，并把 Gallery 状态改成能代表实际产品语义的示例。",
      mode: "Plan",
      isReady: true,
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
];
