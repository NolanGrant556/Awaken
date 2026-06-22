import type { ComponentProps } from "react";
import type { AwakenComposerInput } from "./AwakenComposerInput";

export type AwakenComposerInputState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerInput>>;
};

const noop = () => {};

export const AwakenComposerInputStates: AwakenComposerInputState[] = [
  {
    id: "empty",
    label: "空输入",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      placeholder: "输入任务...",
      minRows: 2,
      maxRows: 6,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
  {
    id: "draft",
    label: "已有草稿",
    props: {
      value: "检查这些组件的 props，并补全用途说明。",
      placeholder: "输入任务...",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
  {
    id: "multiline",
    label: "多行任务",
    props: {
      value: "1. 阅读组件实现\n2. 更新 demo header\n3. 为 README 补用途和边界\n4. 清理 states 的占位 props",
      minRows: 3,
      maxRows: 5,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
  {
    id: "disabled",
    label: "禁用输入",
    props: {
      value: "运行时暂不可编辑，等待当前任务结束。",
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
  {
    id: "long-draft",
    label: "长输入",
    props: {
      value: "请基于真实组件用途写中文说明，不要只根据组件名称猜测；如果组件只是容器，需要明确父级负责传入内容、子级只负责布局。",
      minRows: 2,
      maxRows: 4,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
];
