import {
  File,
  FileArchive,
  FileAudio2,
  FileCode2,
  FileImage,
  FileJson2,
  FileSpreadsheet,
  FileText,
  FileType,
  FileType2,
  FileVideo2,
  Folder,
  FolderOpen,
  Link2,
  Presentation,
  type AwakenGlyphComponent,
} from "@awaken/design-system";
import type { ProjectFileIconKind } from "./projectFiles";

export type ProjectFileVisualIconKind = ProjectFileIconKind | "folder-open";

const icons: Record<ProjectFileVisualIconKind, AwakenGlyphComponent> = {
  "folder-open": FolderOpen,
  folder: Folder,
  symlink: Link2,
  word: FileText,
  excel: FileSpreadsheet,
  powerpoint: Presentation,
  pdf: FileType2,
  markdown: FileType,
  code: FileCode2,
  data: FileJson2,
  image: FileImage,
  archive: FileArchive,
  audio: FileAudio2,
  video: FileVideo2,
  text: FileText,
  file: File,
};

export type ProjectFileIconProps = {
  kind: ProjectFileVisualIconKind;
  size?: number;
  strokeWidth?: number;
};

export default function ProjectFileIcon({
  kind,
  size = 15,
  strokeWidth = 1.75,
}: ProjectFileIconProps) {
  const Icon = icons[kind];
  return <Icon size={size} strokeWidth={strokeWidth} aria-hidden="true" />;
}
