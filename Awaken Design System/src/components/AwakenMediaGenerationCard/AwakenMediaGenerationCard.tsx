import "./AwakenMediaGenerationCard.css";
import { ChevronRight, LoaderCircle } from "lucide-react";
import { useId, useMemo, useState } from "react";
import {
  AwakenMediaItemCard,
  type AwakenMediaItemKind,
  type AwakenMediaItemStatus,
} from "../AwakenMediaItemCard";

export type AwakenMediaGenerationKind = AwakenMediaItemKind;
export type AwakenMediaGenerationStatus = AwakenMediaItemStatus | "partial";

export type AwakenMediaGenerationSource = {
  id: string;
  index?: number;
  status?: AwakenMediaItemStatus;
  detail?: string;
  src?: string;
  alt?: string;
  aspectRatio?: number;
  onPreview?: () => void;
  /** Compatibility for consumers that previously used the only click action. */
  onOpen?: () => void;
};

export type AwakenMediaGenerationCardProps = {
  kind: AwakenMediaGenerationKind;
  status: AwakenMediaGenerationStatus;
  title?: string;
  detail?: string;
  requestedCount?: number;
  requestedAspectRatio?: number;
  sources?: AwakenMediaGenerationSource[];
  /** Single-source compatibility for simple Gallery consumers. */
  src?: string;
  alt?: string;
  defaultOpen?: boolean;
  onPreview?: () => void;
  onOpen?: () => void;
};

const statusLabel: Record<AwakenMediaGenerationStatus, string> = {
  preparing: "正在准备",
  queued: "已提交，等待生成",
  generating: "正在生成",
  downloading: "正在载入成品",
  ready: "已生成",
  partial: "部分生成完成",
  unknown: "状态待确认",
  error: "生成失败",
  cancelled: "已停止",
};

function normalizedCount(kind: AwakenMediaGenerationKind, requestedCount?: number): number {
  if (kind !== "image") return 1;
  if (typeof requestedCount !== "number" || !Number.isFinite(requestedCount)) return 1;
  return Math.min(15, Math.max(1, Math.floor(requestedCount)));
}

export function AwakenMediaGenerationCard({
  kind,
  status,
  title,
  detail,
  requestedCount,
  requestedAspectRatio,
  sources,
  src,
  alt,
  defaultOpen,
  onPreview,
  onOpen,
}: AwakenMediaGenerationCardProps) {
  const bodyId = useId();
  const mediaSources = useMemo<AwakenMediaGenerationSource[]>(() => {
    if (sources && sources.length > 0) return sources.slice(0, 15);
    if (!src) return [];
    return [{ id: "single", src, alt, onPreview, onOpen }];
  }, [alt, onOpen, onPreview, sources, src]);
  const ready = status === "ready" && mediaSources.length > 0;
  const slotCount = ready
    ? mediaSources.length
    : status === "partial"
      ? Math.max(
          normalizedCount(kind, requestedCount),
          ...mediaSources.map((source, index) => source.index ?? index + 1),
        )
      : normalizedCount(kind, requestedCount);
  const label = title || (kind === "image" ? "图片生成" : "视频生成");
  const transient =
    status === "preparing" ||
    status === "queued" ||
    status === "generating" ||
    status === "downloading";
  const autoExpanded = transient || status !== "ready";
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = manualExpanded ?? autoExpanded;
  const summary = ready
    ? kind === "image"
      ? `已生成 ${mediaSources.length} 张图片`
      : "视频已生成"
    : status === "partial"
      ? detail || `已生成 ${mediaSources.length} 个结果，部分未完成`
      : detail || statusLabel[status];

  return (
    <section
      className={`aw-media-generation${expanded ? " aw-media-generation-open" : ""}`}
      data-status={status}
      data-count={slotCount}
      aria-label={`${label}，${statusLabel[status]}`}
    >
      <button
        type="button"
        className="aw-media-generation-summary"
        aria-expanded={expanded}
        aria-controls={bodyId}
        onClick={() => setManualExpanded((current) => !(current ?? autoExpanded))}
      >
        <span className="aw-media-generation-summary-copy">
          <strong>{label}</strong>
          <span>{summary}</span>
        </span>
        {transient ? (
          <span className="aw-media-generation-summary-progress">
            {kind === "image" && slotCount > 1 ? `最多 ${slotCount} 张` : null}
            <LoaderCircle size={16} strokeWidth={1.9} aria-hidden="true" />
          </span>
        ) : null}
        <span className="aw-media-generation-chevron" aria-hidden="true">
          <ChevronRight size={13} strokeWidth={1.8} />
        </span>
      </button>

      <div
        id={bodyId}
        className="aw-media-generation-disclosure"
        data-expanded={expanded ? "true" : "false"}
        aria-hidden={!expanded}
        inert={!expanded}
      >
        <div className="aw-media-generation-grid" aria-label={`${slotCount} 个独立媒体项目`}>
          {Array.from({ length: slotCount }, (_, slotIndex) => {
            const itemNumber = slotIndex + 1;
            const source = mediaSources.find(
              (candidate, sourceIndex) =>
                (candidate.index ?? sourceIndex + 1) === itemNumber,
            );
            const itemStatus: AwakenMediaItemStatus =
              source?.status ?? (status === "partial" ? "error" : status);
            return (
              <AwakenMediaItemCard
                key={source?.id || `slot-${itemNumber}`}
                kind={kind}
                status={itemStatus}
                title={kind === "image" ? `图片 ${itemNumber}` : "视频"}
                detail={
                  source?.detail ||
                  (itemStatus === "ready" ? "点击预览" : statusLabel[itemStatus])
                }
                src={source?.src}
                alt={source?.alt || `${label} ${itemNumber}`}
                index={slotCount > 1 ? itemNumber : undefined}
                aspectRatio={source?.aspectRatio ?? requestedAspectRatio}
                onPreview={source?.onPreview ?? source?.onOpen}
              />
            );
          })}
        </div>
      </div>
    </section>
  );
}
