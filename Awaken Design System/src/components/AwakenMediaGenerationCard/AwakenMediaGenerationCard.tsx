import "./AwakenMediaGenerationCard.css";
import { AlertCircle, Image as ImageIcon, LoaderCircle, Play, Video } from "lucide-react";
import { useEffect, useMemo, useState, type CSSProperties } from "react";

export type AwakenMediaGenerationKind = "image" | "video";

export type AwakenMediaGenerationStatus =
  | "preparing"
  | "queued"
  | "generating"
  | "downloading"
  | "ready"
  | "error"
  | "cancelled";

export type AwakenMediaGenerationSource = {
  id: string;
  src?: string;
  alt?: string;
  onOpen?: () => void;
};

export type AwakenMediaGenerationCardProps = {
  kind: AwakenMediaGenerationKind;
  status: AwakenMediaGenerationStatus;
  title?: string;
  detail?: string;
  requestedCount?: number;
  sources?: AwakenMediaGenerationSource[];
  /** Single-source compatibility for simple Gallery consumers. */
  src?: string;
  alt?: string;
  onOpen?: () => void;
};

const GRID_CELL_COUNT = 40;
const GRID_CELLS = Array.from({ length: GRID_CELL_COUNT }, (_, index) => index);

const statusLabel: Record<AwakenMediaGenerationStatus, string> = {
  preparing: "正在准备",
  queued: "已提交，等待生成",
  generating: "正在生成",
  downloading: "正在载入成品",
  ready: "已生成",
  error: "生成失败",
  cancelled: "已停止",
};

function cellStyle(index: number): CSSProperties {
  return {
    "--aw-media-cell-index": index,
    "--aw-media-cell-column": index % 8,
    "--aw-media-cell-row": Math.floor(index / 8),
  } as CSSProperties;
}

function galleryColumnCount(count: number): number {
  if (count >= 13) return 5;
  if (count >= 9) return 4;
  if (count >= 5) return 3;
  if (count >= 2) return 2;
  return 1;
}

export function AwakenMediaGenerationCard({
  kind,
  status,
  title,
  detail,
  requestedCount,
  sources,
  src,
  alt,
  onOpen,
}: AwakenMediaGenerationCardProps) {
  const [loadedSourceIds, setLoadedSourceIds] = useState<Set<string>>(() => new Set());
  const mediaSources = useMemo<AwakenMediaGenerationSource[]>(() => {
    if (sources && sources.length > 0) return sources.slice(0, 15);
    if (!src) return [];
    return [{ id: "single", src, alt, onOpen }];
  }, [alt, onOpen, sources, src]);
  const sourceSignature = mediaSources.map((source) => `${source.id}:${source.src ?? ""}`).join("|");
  const ready = status === "ready" && mediaSources.length > 0;
  const group = kind === "image" && mediaSources.length > 1;
  const label = title || (kind === "image" ? "图片生成" : "视频生成");
  const transient = status !== "ready" && status !== "error" && status !== "cancelled";
  const resultDetail =
    ready && kind === "image" && mediaSources.length > 1
      ? `已生成 ${mediaSources.length} 张图片`
      : detail || statusLabel[status];

  useEffect(() => {
    setLoadedSourceIds(new Set());
  }, [sourceSignature, kind]);

  const markLoaded = (id: string) => {
    setLoadedSourceIds((current) => {
      const next = new Set(current);
      next.add(id);
      return next;
    });
  };

  return (
    <article
      className={`aw-media-generation-card aw-media-generation-card-${kind}`}
      data-status={status}
      data-group={group ? "true" : "false"}
      aria-label={`${label}，${statusLabel[status]}`}
    >
      <div
        className={`aw-media-generation-card-viewport${ready ? " is-ready" : ""}${group ? " is-group" : ""}`}
        style={
          group
            ? ({ "--aw-media-gallery-columns": galleryColumnCount(mediaSources.length) } as CSSProperties)
            : undefined
        }
      >
        {ready && kind === "image" ? (
          <div className="aw-media-generation-card-gallery" aria-label={`${mediaSources.length} 张生成图片`}>
            {mediaSources.map((source, index) => {
              const loaded = loadedSourceIds.has(source.id);
              const content = (
                <>
                  {source.src ? (
                    <img
                      className="aw-media-generation-card-media"
                      src={source.src}
                      alt={source.alt || `${label} ${index + 1}`}
                      onLoad={() => markLoaded(source.id)}
                      onError={() => setLoadedSourceIds((current) => {
                        const next = new Set(current);
                        next.delete(source.id);
                        return next;
                      })}
                    />
                  ) : null}
                  {!loaded ? (
                    <span className="aw-media-generation-card-item-loader" aria-hidden="true">
                      <LoaderCircle size={18} strokeWidth={1.8} />
                    </span>
                  ) : null}
                  {group ? <span className="aw-media-generation-card-index">{index + 1}</span> : null}
                </>
              );
              return source.onOpen ? (
                <button
                  key={source.id}
                  type="button"
                  className="aw-media-generation-card-item"
                  data-loaded={loaded ? "true" : "false"}
                  onClick={source.onOpen}
                  aria-label={`打开${label} ${index + 1}`}
                >
                  {content}
                </button>
              ) : (
                <span
                  key={source.id}
                  className="aw-media-generation-card-item"
                  data-loaded={loaded ? "true" : "false"}
                >
                  {content}
                </span>
              );
            })}
          </div>
        ) : null}

        {ready && kind === "video" ? (
          <video
            className="aw-media-generation-card-media aw-media-generation-card-video"
            src={mediaSources[0]?.src}
            controls
            playsInline
            preload="metadata"
            aria-label={mediaSources[0]?.alt || alt || label}
            onLoadedData={() => markLoaded(mediaSources[0]!.id)}
          />
        ) : null}

        {!ready ? (
          <div className="aw-media-generation-card-loading-surface">
            <div className="aw-media-generation-card-grid" aria-hidden="true">
              {GRID_CELLS.map((index) => (
                <span key={index} style={cellStyle(index)} />
              ))}
            </div>
            {status === "error" || status === "cancelled" ? (
              <div className="aw-media-generation-card-terminal" aria-hidden="true">
                <AlertCircle size={22} strokeWidth={1.7} />
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      <div className="aw-media-generation-card-footer">
        <span className="aw-media-generation-card-kind" aria-hidden="true">
          {kind === "image" ? (
            <ImageIcon size={16} strokeWidth={1.8} />
          ) : (
            <Video size={16} strokeWidth={1.8} />
          )}
        </span>
        <span className="aw-media-generation-card-copy">
          <strong>{label}</strong>
          <span>{resultDetail}</span>
        </span>
        {transient ? (
          <span className="aw-media-generation-card-progress">
            {kind === "image" && requestedCount && requestedCount > 1 ? `最多 ${Math.min(15, requestedCount)} 张` : null}
            <LoaderCircle
              className="aw-media-generation-card-spinner"
              size={17}
              strokeWidth={1.9}
              aria-hidden="true"
            />
          </span>
        ) : null}
        {status === "ready" && mediaSources.length === 1 && mediaSources[0]?.onOpen ? (
          <button type="button" onClick={mediaSources[0].onOpen} aria-label={`打开${label}`}>
            {kind === "video" ? <Play size={15} fill="currentColor" aria-hidden="true" /> : null}
            打开
          </button>
        ) : null}
      </div>
    </article>
  );
}
