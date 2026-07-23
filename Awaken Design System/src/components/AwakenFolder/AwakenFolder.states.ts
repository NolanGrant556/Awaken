import type { AwakenFolderProps } from "./AwakenFolder";

const noop = () => {};

export const AwakenFolderStates = [
  { id: "project", label: "默认", props: { label: "01_财务部", meta: "4 Skills", onOpen: noop } },
  { id: "memory", label: "紧凑", props: { label: "经营管理部经营分析项目", meta: "3 个主题", size: "compact", onOpen: noop } },
  { id: "open", label: "展开", props: { label: "市场部", meta: "2 Skills", open: true, onOpen: noop } },
  { id: "disabled", label: "不可用", props: { label: "已归档项目", meta: "0 个主题", disabled: true, onOpen: noop } },
] satisfies Array<{ id: string; label: string; props: AwakenFolderProps }>;
