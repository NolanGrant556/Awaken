import type { ReactNode } from "react";
import { ExternalLink, FolderSearch2, RefreshCw } from "lucide-react";

export type PreviewFrameProps = {
  displayName: string;
  formatLabel: string;
  formatActions?: ReactNode;
  loading?: boolean;
  onReload: () => void;
  onOpenExternal: () => void;
  onReveal: () => void;
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
              <RefreshCw size={15} aria-hidden="true" />
            </button>
            <button
              type="button"
              aria-label="使用系统默认应用打开"
              title="使用系统默认应用打开"
              onClick={onOpenExternal}
            >
              <ExternalLink size={15} aria-hidden="true" />
            </button>
            <button
              type="button"
              aria-label="在文件管理器中显示"
              title="在文件管理器中显示"
              onClick={onReveal}
            >
              <FolderSearch2 size={15} aria-hidden="true" />
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
