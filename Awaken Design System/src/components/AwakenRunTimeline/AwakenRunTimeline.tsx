import "./AwakenRunTimeline.css";
import type { ReactNode } from "react";
import type { RunStepItem } from "../../types";
import { AwakenRunStep } from "../AwakenRunStep";

export type RunTimelineStatus = "ready" | "running" | "done" | "error";

export type AwakenRunTimelineProps = {
  title: string;
  status: RunTimelineStatus;
  steps: RunStepItem[];
  children?: ReactNode;
};

const statusPillStyles: Record<RunTimelineStatus, React.CSSProperties> = {
  ready: {
    background: "var(--aw-bg-fill-light)",
    color: "var(--aw-text-tertiary)",
    borderColor: "var(--aw-border-main)",
  },
  running: {
    background: "color-mix(in srgb, var(--aw-text-blue) 12%, transparent)",
    color: "var(--aw-text-blue)",
    borderColor: "color-mix(in srgb, var(--aw-text-blue) 24%, transparent)",
  },
  done: {
    background: "var(--aw-success-soft-bg)",
    color: "var(--aw-success)",
    borderColor: "var(--aw-success-soft-border)",
  },
  error: {
    background: "var(--aw-error-soft-bg)",
    color: "var(--aw-error)",
    borderColor: "var(--aw-error-soft-border)",
  },
};

const statusLabels: Record<RunTimelineStatus, string> = {
  ready: "就绪",
  running: "运行中",
  done: "已完成",
  error: "出错",
};

export function AwakenRunTimeline({ title, status, steps, children }: AwakenRunTimelineProps) {
  const isVisible = status !== "ready";

  return (
    <div
      style={{
        display: isVisible ? "block" : "block",
        animation: isVisible ? "panel-in 220ms ease-out" : "none",
      }}
    >
      <div
        style={{
          background: "var(--aw-bg-white)",
          border: "1px solid var(--aw-border-main)",
          borderRadius: "var(--aw-radius-lg)",
          boxShadow: "var(--aw-shadow-menu)",
          padding: "16px",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "12px",
            marginBottom: steps.length > 0 ? "8px" : "0",
            minWidth: 0,
          }}
        >
          <h3
            className="aw-truncate"
            style={{
              margin: 0,
              fontSize: "var(--aw-font-size-base)",
              lineHeight: "var(--aw-line-height-base)",
              fontWeight: "var(--aw-font-weight-bold)",
              color: "var(--aw-text-primary)",
            }}
            title={title}
          >
            {title}
          </h3>
          <span
            style={{
              flex: "0 0 auto",
              display: "inline-flex",
              alignItems: "center",
              height: "24px",
              padding: "0 10px",
              border: "1px solid",
              borderRadius: "var(--aw-radius-sm)",
              fontSize: "var(--aw-font-size-xs)",
              fontWeight: "var(--aw-font-weight-heavy)",
              whiteSpace: "nowrap",
              ...statusPillStyles[status],
            }}
          >
            {statusLabels[status]}
          </span>
        </div>

        {steps.length === 0 ? (
          <div
            style={{
              padding: "24px 0",
              textAlign: "center",
              color: "var(--aw-text-tertiary)",
              fontSize: "var(--aw-font-size-sm)",
              lineHeight: "var(--aw-line-height-sm)",
            }}
          >
            暂无运行步骤
          </div>
        ) : (
          <div role="list" aria-label={title}>
            {steps.map((step) => (
              <div key={step.id} role="listitem">
                <AwakenRunStep
                  index={step.index}
                  title={step.title}
                  description={step.description}
                  tag={step.tag}
                  status={step.status}
                />
              </div>
            ))}
          </div>
        )}

        {children ? (
          <div style={{ marginTop: "16px" }}>{children}</div>
        ) : null}
      </div>
    </div>
  );
}
