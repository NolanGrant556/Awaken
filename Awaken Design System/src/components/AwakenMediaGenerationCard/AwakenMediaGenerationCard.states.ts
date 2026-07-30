import type { ComponentProps } from "react";
import type { AwakenMediaGenerationCard } from "./AwakenMediaGenerationCard";

export type AwakenMediaGenerationCardState = {
  id: string;
  name: string;
  description: string;
  props: Partial<ComponentProps<typeof AwakenMediaGenerationCard>>;
};

const sampleImage =
  "data:image/svg+xml;charset=utf-8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="750" viewBox="0 0 1200 750"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#c9d8ff"/><stop offset=".55" stop-color="#7257d9"/><stop offset="1" stop-color="#17152a"/></linearGradient></defs><rect width="1200" height="750" fill="url(#g)"/><circle cx="790" cy="290" r="180" fill="#fff" opacity=".34"/><path d="M0 630 330 310 600 560 830 370 1200 700V750H0Z" fill="#111321" opacity=".7"/></svg>',
  );

export const AwakenMediaGenerationCardStates: AwakenMediaGenerationCardState[] = [
  {
    id: "image-generating",
    name: "图片生成中",
    description: "15 个结果分别使用独立紧凑卡片，完成后逐项由真实媒体覆盖。",
    props: { kind: "image", status: "generating", title: "正在生成图片", requestedCount: 15 },
  },
  {
    id: "video-queued",
    name: "视频排队中",
    description: "视频异步任务沿用同一占位容器。",
    props: { kind: "video", status: "queued", title: "正在生成视频" },
  },
  {
    id: "image-ready",
    name: "图片已生成",
    description: "完成后默认折叠为一行摘要，点击后查看成品卡片。",
    props: {
      kind: "image",
      status: "ready",
      title: "生成的主视觉",
      src: sampleImage,
      onOpen: () => undefined,
    },
  },
  {
    id: "image-group-ready",
    name: "组图已生成",
    description: "一个 tool call 的多张结果完成后默认折叠，展开后仍是独立小卡片。",
    props: {
      kind: "image",
      status: "ready",
      title: "产品系列图",
      sources: Array.from({ length: 12 }, (_, index) => ({
        id: `group-${index + 1}`,
        src: sampleImage,
        alt: `产品系列图 ${index + 1}`,
        onOpen: () => undefined,
      })),
    },
  },
];
