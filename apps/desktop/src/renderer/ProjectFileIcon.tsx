import { AwakenIcon, type AwakenIconConcept, type AwakenIconSize } from "@awaken/design-system";

import type { ProjectFileIconKind } from "./projectFiles";

export type ProjectFileVisualIconKind = ProjectFileIconKind;

/**
 * 文件类型 → 概念。图形由 Design System 的文件类型族提供：十一个类型共用同一个
 * 文件外形，只换内标，因此文件树里的每一行都是同一个「文件」。
 * 目录展开不换图标，只靠箭头旋转表达开合。
 */
const concepts: Record<ProjectFileVisualIconKind, AwakenIconConcept> = {
  folder: "project",
  symlink: "symlink",
  word: "file-text",
  excel: "spreadsheet",
  powerpoint: "presentation",
  pdf: "file-pdf",
  markdown: "file-markdown",
  code: "file-code",
  data: "file-data",
  image: "file-image",
  archive: "file-archive",
  audio: "file-audio",
  video: "file-video",
  text: "file-text",
  file: "file-generic",
};

export type ProjectFileIconProps = {
  kind: ProjectFileVisualIconKind;
  size?: AwakenIconSize;
};

export default function ProjectFileIcon({ kind, size = 16 }: ProjectFileIconProps) {
  return <AwakenIcon concept={concepts[kind]} size={size} />;
}
