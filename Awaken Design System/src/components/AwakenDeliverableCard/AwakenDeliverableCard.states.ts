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
  {
    id: "image-ready",
    name: "图片已就绪",
    description: "最终交付图片在卡片中直接预览。",
    props: {
      title: "生成的主视觉.png",
      extension: "png",
      fileType: "image",
      previewSrc: "data:image/svg+xml;charset=utf-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='1200' height='675'%3E%3Cdefs%3E%3ClinearGradient id='g' x1='0' y1='0' x2='1' y2='1'%3E%3Cstop stop-color='%23a9c7ff'/%3E%3Cstop offset='1' stop-color='%232b1f55'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect width='1200' height='675' fill='url(%23g)'/%3E%3C/svg%3E",
      onOpen: () => undefined,
      onReveal: () => undefined,
    },
  },
  {
    id: "video-ready",
    name: "视频已就绪",
    description: "视频交付卡保留原生播放控件和系统打开入口。",
    props: {
      title: "生成的视频.mp4",
      extension: "mp4",
      fileType: "video",
      onOpen: () => undefined,
      onReveal: () => undefined,
    },
  },
];
