import "./AwakenDeliverableCard.css";
import { AwakenIcon } from "../AwakenIcon";
import { useState } from "react";
import { AwakenMediaItemCard } from "../AwakenMediaItemCard";
import type { AwakenMediaMetadata } from "../AwakenMediaItemCard";
import { AwakenPopover } from "../AwakenPopover";

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
  onOpenExternal?: () => void;
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
      <AwakenIcon concept="spreadsheet" size={24} className="aw-deliverable-card-type-icon" />
    );
  }
  if (fileType === "powerpoint") {
    return (
      <AwakenIcon concept="presentation" size={24} className="aw-deliverable-card-type-icon" />
    );
  }
  if (fileType === "pdf") {
    return (
      <AwakenIcon concept="file-pdf" size={24} className="aw-deliverable-card-type-icon aw-deliverable-card-pdf-icon" />
    );
  }
  return (
    <AwakenIcon concept="file-text" size={24} className="aw-deliverable-card-type-icon" />
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
  onOpenExternal,
  onReveal,
  localOpenLabel,
  revealLabel,
}: AwakenDeliverableCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const normalizedExtension = extension.replace(/^\./, "").toUpperCase();
  const canOpen = Boolean(onOpen) && !disabled && !opening;

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
        onPreview={onPreview ?? onOpen}
        onMediaError={onMediaError}
        onMetadata={onMediaMetadata}
        onViewInInspector={onViewInInspector}
        onOpenExternal={onOpen}
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
        aria-label={opening ? `正在打开 ${title}` : `在右侧栏预览 ${title}`}
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
          <AwakenIcon concept="step-running" size={20} className="aw-deliverable-card-spinner" />
        ) : null}
      </button>

      {onOpenExternal || onReveal ? (
        <div className="aw-deliverable-card-menu">
          <AwakenPopover
            trigger={
              <button
                type="button"
                className="aw-deliverable-card-menu-trigger"
                aria-label={`更多 ${title} 操作`}
                aria-expanded={menuOpen}
                aria-haspopup="menu"
              >
                <AwakenIcon concept="more-actions" size={20} />
              </button>
            }
            content={
              <div className="aw-deliverable-card-menu-popover">
                {onOpenExternal ? (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setMenuOpen(false);
                      onOpenExternal();
                    }}
                    disabled={disabled || opening}
                  >
                    {localOpenLabel || "使用本地应用打开"}
                  </button>
                ) : null}
                {onReveal ? (
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
                ) : null}
              </div>
            }
            open={menuOpen}
            onOpenChange={setMenuOpen}
            placement="bottom-end"
            offset={4}
            role="menu"
            portalled={false}
          />
        </div>
      ) : null}
    </article>
  );
}
