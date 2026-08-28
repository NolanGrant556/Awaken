import type { ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";

export type PreviewFrameProps = {
  displayName: string;
  formatLabel: string;
  formatActions?: ReactNode;
  loading?: boolean;
  onReload: () => void;
  onOpenExternal: () => void;
  onReveal: () => void;
  revealLabel?: string;
  children: ReactNode;
};

export default function PreviewFrame({
  displayName,
  formatLabel,
  formatActions,
  loading = false,
  onReload,
  onOpenExternal,
  onReveal,
  revealLabel = "在文件管理器中显示",
  children,
}: PreviewFrameProps) {
  return (
    <div className="project-file-preview-host">
      <header className="project-file-preview-heading">
        <div className="project-file-preview-heading__identity">
          <strong>{displayName}</strong>
          <span className="project-file-preview-heading__format">
            {formatLabel}
          </span>
        </div>
        <div className="project-file-preview-heading__actions">
          {formatActions ? (
            <div className="project-file-preview-heading__format-actions">
              {formatActions}
            </div>
          ) : null}
          <div className="project-file-preview-heading__file-actions">
            <button
              type="button"
              aria-label="重新载入文件预览"
              title="重新载入文件预览"
              onClick={onReload}
            >
              <AwakenIcon concept="refresh" size={16} />
            </button>
            <button
              type="button"
              aria-label="使用系统默认应用打开"
              title="使用系统默认应用打开"
              onClick={onOpenExternal}
            >
              <AwakenIcon concept="external-open" size={16} />
            </button>
            <button
              type="button"
              aria-label={revealLabel}
              title={revealLabel}
              onClick={onReveal}
            >
              <AwakenIcon concept="locate-folder" size={16} />
            </button>
          </div>
        </div>
      </header>
      <div className="project-file-preview-content">
        {children}
        {loading ? (
          <div className="project-file-preview-loading-overlay" role="status">
            <span className="project-file-preview-spinner" aria-hidden="true" />
            正在加载…
          </div>
        ) : null}
      </div>
    </div>
  );
}
