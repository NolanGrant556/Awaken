import type { ComponentProps } from "react";
import type { AwakenDeliverableList } from "./AwakenDeliverableList";

export type AwakenDeliverableListState = {
  id: string;
  name: string;
  description: string;
  props: Partial<ComponentProps<typeof AwakenDeliverableList>>;
};

const files = [
  { id: "word", title: "分析报告.docx", extension: "docx", fileType: "word" as const, onOpen: () => undefined },
  { id: "sheet", title: "合并汇总.xlsx", extension: "xlsx", fileType: "excel" as const, onOpen: () => undefined },
  { id: "pdf", title: "交付版本.pdf", extension: "pdf", fileType: "pdf" as const, onOpen: () => undefined },
  { id: "slides", title: "汇报材料.pptx", extension: "pptx", fileType: "powerpoint" as const, onOpen: () => undefined },
];

export const AwakenDeliverableListStates: AwakenDeliverableListState[] = [
  { id: "one", name: "单文件", description: "一个最终交付物。", props: { items: files.slice(0, 1) } },
  { id: "pdf", name: "PDF 单文件", description: "显式发布的 PDF 最终交付物。", props: { items: files.slice(2, 3) } },
  { id: "three", name: "三个文件", description: "无需折叠。", props: { items: files.slice(0, 3) } },
  { id: "four", name: "四个文件", description: "默认前三个，可展开。", props: { items: files } },
  { id: "failed", name: "失败后成果", description: "任务失败但已有可用文件。", props: { items: files.slice(0, 1), terminalStatus: "error" } },
];
