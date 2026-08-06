import "./AwakenDeliverableCard.css";
import {
  FileSpreadsheet,
  FileText,
  FileType2,
  LoaderCircle,
  MoreHorizontal,
  Presentation,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { AwakenMediaItemCard } from "../AwakenMediaItemCard";
import type { AwakenMediaMetadata } from "../AwakenMediaItemCard";

export type AwakenDeliverableFileType =
  | "word"
  | "markdown"
  | "excel"
  | "pdf"
  | "powerpoint"
  | "image"
  | "video";

export type AwakenDeliverableCardProps = {
  title: string;
  extension: string;
  fileType: AwakenDeliverableFileType;
  opening?: boolean;
  disabled?: boolean;
  previewSrc?: string;
  previewAlt?: string;
  previewAspectRatio?: number;
  previewMetadata?: AwakenMediaMetadata;
  /** When true, media deliverables show an unavailable terminal state instead of an infinite ready skeleton. */
  previewUnavailable?: boolean;
  onPreview?: () => void;
  onMediaError?: () => void;
  onMediaMetadata?: (metadata: AwakenMediaMetadata) => void;
  onViewInInspector?: () => void;
  onOpen?: () => void;
  onReveal?: () => void;
  localOpenLabel?: string;
  revealLabel?: string;
};

const fileTypeLabel: Record<AwakenDeliverableFileType, string> = {
  word: "Word",
  markdown: "Markdown",
  excel: "Excel",
  pdf: "PDF",
  powerpoint: "PowerPoint",
  image: "图片",
  video: "视频",
};

function FileTypeIcon({ fileType }: { fileType: AwakenDeliverableFileType }) {
  if (fileType === "excel") {
    return (
      <FileSpreadsheet
        className="aw-deliverable-card-type-icon"
        aria-hidden="true"
        size={22}
        strokeWidth={1.65}
      />
    );
  }
  if (fileType === "powerpoint") {
    return (
      <Presentation
        className="aw-deliverable-card-type-icon"
        aria-hidden="true"
        size={22}
        strokeWidth={1.65}
      />
    );
  }
  if (fileType === "pdf") {
    return (
      <FileType2
        className="aw-deliverable-card-type-icon aw-deliverable-card-pdf-icon"
        aria-hidden="true"
        size={22}
        strokeWidth={1.65}
      />
    );
  }
  return (
    <FileText
      className="aw-deliverable-card-type-icon"
      aria-hidden="true"
      size={22}
      strokeWidth={1.65}
    />
  );
}

export function AwakenDeliverableCard({
  title,
  extension,
  fileType,
  opening = false,
  disabled = false,
  previewSrc,
  previewAlt,
  previewAspectRatio,
  previewMetadata,
  previewUnavailable = false,
  onPreview,
  onMediaError,
  onMediaMetadata,
  onViewInInspector,
  onOpen,
  onReveal,
  localOpenLabel,
  revealLabel,
}: AwakenDeliverableCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDetailsElement>(null);
  const normalizedExtension = extension.replace(/^\./, "").toUpperCase();
  const canOpen = Boolean(onOpen) && !disabled && !opening;

  useEffect(() => {
    if (!menuOpen) return;
    const closeOnOutsideInteraction = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutsideInteraction);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideInteraction);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [menuOpen]);

  if (fileType === "image" || fileType === "video") {
    const mediaStatus = previewUnavailable ? "error" : "ready";
    return (
      <AwakenMediaItemCard
        kind={fileType}
        status={mediaStatus}
        title={title}
        detail={
          previewUnavailable
            ? "预览不可用"
            : `${fileTypeLabel[fileType]} · ${normalizedExtension}`
        }
        src={previewUnavailable ? undefined : previewSrc}
        alt={previewAlt || title}
        aspectRatio={previewAspectRatio}
        metadata={previewMetadata}
        formatLabel={normalizedExtension}
        disabled={disabled}
        openingExternal={opening}
        onPreview={previewUnavailable ? undefined : onPreview ?? onOpen}
        onMediaError={onMediaError}
        onMetadata={onMediaMetadata}
        onViewInInspector={onViewInInspector}
        onOpenExternal={onPreview ? onOpen : undefined}
        onReveal={onReveal}
        localOpenLabel={localOpenLabel}
        revealLabel={revealLabel}
        className={`aw-deliverable-card aw-deliverable-card-${fileType} aw-deliverable-card-media`}
      />
    );
  }

  return (
    <article
      className={`aw-deliverable-card aw-deliverable-card-${fileType}`}
      data-file-type={fileType}
      data-menu-open={menuOpen ? "true" : undefined}
      aria-label={`${title}，${fileTypeLabel[fileType]} 文件`}
    >
      <button
        type="button"
        className="aw-deliverable-card-open"
        onClick={onOpen}
        disabled={!canOpen}
        aria-label={opening ? `正在打开 ${title}` : `打开 ${title}`}
      >
        <span className="aw-deliverable-card-document" aria-hidden="true">
          <FileTypeIcon fileType={fileType} />
          <span className="aw-deliverable-card-extension">{normalizedExtension}</span>
        </span>
        <span className="aw-deliverable-card-copy">
          <strong className="aw-deliverable-card-title" title={title}>
            {title}
          </strong>
          <span className="aw-deliverable-card-kind">{fileTypeLabel[fileType]}</span>
        </span>
        {opening ? (
          <LoaderCircle
            className="aw-deliverable-card-spinner"
            aria-hidden="true"
            size={18}
            strokeWidth={1.9}
          />
        ) : null}
      </button>

      {onReveal ? (
        <details
          ref={menuRef}
          className="aw-deliverable-card-menu"
          open={menuOpen}
          onToggle={(event) => setMenuOpen(event.currentTarget.open)}
        >
          <summary aria-label={`更多 ${title} 操作`} aria-expanded={menuOpen}>
            <MoreHorizontal aria-hidden="true" size={20} strokeWidth={1.8} />
          </summary>
          <div className="aw-deliverable-card-menu-popover" role="menu">
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setMenuOpen(false);
                onReveal();
              }}
              disabled={disabled}
            >
              {revealLabel || "在 Finder/资源管理器中显示"}
            </button>
          </div>
        </details>
      ) : null}
    </article>
  );
}
