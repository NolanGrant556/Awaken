import type { ComponentProps } from "react";
import type { AwakenSelect } from "./AwakenSelect";

export type AwakenSelectState = {
  id: string;
  label: string;
  props: ComponentProps<typeof AwakenSelect>;
};

const options = [
  { value: "ark-code-latest", label: "ark-code-latest" },
  { value: "doubao-seed-code", label: "doubao-seed-code" },
  { value: "doubao-seed-2.0-code", label: "doubao-seed-2.0-code" },
];

export const AwakenSelectStates: AwakenSelectState[] = [
  {
    id: "selected",
    label: "已选择",
    props: {
      value: "ark-code-latest",
      options,
      ariaLabel: "模型",
      onValueChange: () => undefined,
    },
  },
  {
    id: "placeholder",
    label: "待选择",
    props: {
      value: "",
      options,
      placeholder: "请选择一个模型",
      ariaLabel: "模型",
      onValueChange: () => undefined,
    },
  },
  {
    id: "disabled",
    label: "不可用",
    props: {
      value: "",
      options: [],
      placeholder: "当前没有可用模型",
      ariaLabel: "模型",
      disabled: true,
      onValueChange: () => undefined,
    },
  },
];
