import type { ComponentProps } from "react";
import type { AwakenModelSelector } from "./AwakenModelSelector";

export type AwakenModelSelectorState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenModelSelector>>;
};

const noop = () => {};

export const AwakenModelSelectorStates: AwakenModelSelectorState[] = [
  {
    id: "selected-model",
    label: "已选模型",
    props: {
      value: { id: "gpt-5-thinking", label: "GPT-5 Thinking" },
      options: [
        { id: "gpt-5-thinking", label: "GPT-5 Thinking" },
        { id: "gpt-5-mini", label: "GPT-5 Mini" },
        { id: "local-small", label: "本地轻量模型", disabled: true },
      ],
      onChange: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
  {
    id: "disabled-option",
    label: "包含禁用项",
    props: {
      value: { id: "gpt-5-mini", label: "GPT-5 Mini" },
      options: [
        { id: "gpt-5-thinking", label: "GPT-5 Thinking" },
        { id: "gpt-5-mini", label: "GPT-5 Mini" },
        { id: "provider-offline", label: "离线 provider 模型", disabled: true },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
  {
    id: "empty-options",
    label: "无可用模型",
    props: {
      value: { id: "none", label: "选择模型" },
      options: [],
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
  {
    id: "selector-disabled",
    label: "禁用选择器",
    props: {
      value: { id: "locked-model", label: "项目默认模型" },
      options: [{ id: "locked-model", label: "项目默认模型" }],
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
  {
    id: "long-model-label",
    label: "长模型名",
    props: {
      value: { id: "long-model", label: "GPT-5 Thinking with extended reasoning budget" },
      options: [
        { id: "long-model", label: "GPT-5 Thinking with extended reasoning budget" },
        { id: "compact-model", label: "快速响应模型" },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
];
