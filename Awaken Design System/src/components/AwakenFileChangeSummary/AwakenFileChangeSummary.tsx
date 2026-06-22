import "./AwakenFileChangeSummary.css";
import { AlertCircle, Check, FileText, LoaderCircle } from "lucide-react";

export type FileChangeSummaryStatus = "active" | "done" | "error";

export type FileChangeSummaryFile = {
  id: string;
  title: string;
  path?: string;
  description?: string;
  status?: FileChangeSummaryStatus;
};

export type AwakenFileChangeSummaryProps = {
  files: FileChangeSummaryFile[];
  status?: "active" | "done";
  title?: string;
};

const statusColor: Record<FileChangeSummaryStatus, string> = {
  active: "var(--aw-status-running)",
  done: "var(--aw-status-success)",
  error: "var(--aw-status-danger)",
};

function fileIcon(status: FileChangeSummaryStatus) {
  if (status === "active") {
    return <LoaderCircle size={15} strokeWidth={2.2} style={{ animation: "spin 1s linear infinite" }} />;
  }
  if (status === "error") {
    return <AlertCircle size={15} strokeWidth={2.2} />;
  }
  return <Check size={15} strokeWidth={2.4} />;
}

export function AwakenFileChangeSummary({
  files,
  status = "done",
  title = "File changes",
}: AwakenFileChangeSummaryProps) {
  return (
    <article
      style={{
        display: "grid",
        gap: "12px",
        width: "100%",
        maxWidth: "100%",
        boxSizing: "border-box",
        minWidth: 0,
        padding: "16px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-panel)",
        background: "var(--aw-surface-raised)",
        boxShadow: "var(--aw-shadow-raised)",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
          minWidth: 0,
        }}
      >
        <span
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "8px",
            minWidth: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            fontWeight: "var(--aw-font-weight-bold)",
            lineHeight: "var(--aw-line-height-sm)",
          }}
        >
          <FileText size={16} strokeWidth={2} />
          <span className="aw-truncate" title={title}>{title}</span>
        </span>
        <span
          style={{
            flex: "0 0 auto",
            color: status === "active" ? "var(--aw-status-running)" : "var(--aw-status-success)",
            fontSize: "var(--aw-font-size-xs)",
            fontWeight: "var(--aw-font-weight-semibold)",
            lineHeight: "var(--aw-line-height-xs)",
            whiteSpace: "nowrap",
          }}
        >
          {status}
        </span>
      </div>

      <ul
        style={{
          display: "grid",
          gap: "8px",
          margin: 0,
          padding: 0,
          listStyle: "none",
        }}
      >
        {files.length === 0 ? (
          <li
            style={{
              color: "var(--aw-text-subtle)",
              fontSize: "var(--aw-font-size-sm)",
              lineHeight: "var(--aw-line-height-sm)",
            }}
          >
            No changed files reported.
          </li>
        ) : (
          files.map((file) => {
            const fileStatus = file.status ?? "done";
            return (
              <li
                key={file.id}
                style={{
                  display: "grid",
                  gridTemplateColumns: "22px minmax(0, 1fr)",
                  gap: "8px",
                  alignItems: "flex-start",
                  minWidth: 0,
                  padding: "10px",
                  border: "1px solid var(--aw-border-light)",
                  borderRadius: "var(--aw-radius-control)",
                  background: "var(--aw-bg-fill-light)",
                }}
              >
                <span
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    width: "22px",
                    height: "22px",
                    color: statusColor[fileStatus],
                  }}
                >
                  {fileIcon(fileStatus)}
                </span>
                <span
                  style={{
                    display: "grid",
                    gap: "3px",
                    minWidth: 0,
                  }}
                >
                  <strong
                    style={{
                      color: "var(--aw-text-primary)",
                      fontSize: "var(--aw-font-size-sm)",
                      lineHeight: "var(--aw-line-height-sm)",
                      overflowWrap: "anywhere",
                    }}
                  >
                    {file.title}
                  </strong>
                  {file.path ? (
                    <span
                      title={file.path}
                      style={{
                        color: "var(--aw-text-code)",
                        fontFamily: "var(--aw-font-mono)",
                        fontSize: "var(--aw-font-size-xs)",
                        lineHeight: "var(--aw-line-height-xs)",
                        overflowWrap: "anywhere",
                      }}
                    >
                      {file.path}
                    </span>
                  ) : null}
                  {file.description ? (
                    <span
                      style={{
                        color: "var(--aw-text-subtle)",
                        fontSize: "var(--aw-font-size-xs)",
                        lineHeight: "var(--aw-line-height-xs)",
                        overflowWrap: "anywhere",
                      }}
                    >
                      {file.description}
                    </span>
                  ) : null}
                </span>
              </li>
            );
          })
        )}
      </ul>
    </article>
  );
}
