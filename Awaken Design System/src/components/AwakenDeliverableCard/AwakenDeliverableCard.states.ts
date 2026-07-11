import type { ComponentProps } from "react";
import type { AwakenDeliverableCard } from "./AwakenDeliverableCard";

export type AwakenDeliverableCardState = {
  id: string;
  name: string;
  description: string;
  props: Partial<ComponentProps<typeof AwakenDeliverableCard>>;
};

export const AwakenDeliverableCardStates: AwakenDeliverableCardState[] = [
  {
    id: "excel-ready",
    name: "Excel 已就绪",
    description: "最终交付的电子表格。",
    props: { title: "合并汇总.xlsx", extension: "xlsx", fileType: "excel", onOpen: () => undefined },
  },
  {
    id: "word-ready",
    name: "Word 已就绪",
    description: "最终交付的 Word 文档。",
    props: { title: "风险分析报告.docx", extension: "docx", fileType: "word", onOpen: () => undefined },
  },
  {
    id: "pdf-ready",
    name: "PDF 已就绪",
    description: "显式发布的 PDF 使用专用图标，并由整张卡片打开。",
    props: {
      title: "风险分析报告.pdf",
      extension: ".pdf",
      fileType: "pdf",
      onOpen: () => undefined,
      onReveal: () => undefined,
    },
  },
  {
    id: "pdf-opening",
    name: "PDF 打开中",
    description: "PDF 正在交给系统默认应用。",
    props: {
      title: "风险分析报告.pdf",
      extension: "pdf",
      fileType: "pdf",
      opening: true,
      onOpen: () => undefined,
    },
  },
];
