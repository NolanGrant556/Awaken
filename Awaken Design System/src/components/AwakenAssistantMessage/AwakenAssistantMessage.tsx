import "./AwakenAssistantMessage.css";
import { memo } from "react";
import { AwakenCopyButton } from "../AwakenCopyButton";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";

type AssistantMessageStatus = "active" | "done" | "error";

export type AwakenAssistantMessageProps = {
  markdown: string;
  status?: AssistantMessageStatus;
  elapsedMs?: number;
  showFooter?: boolean;
};

function formatElapsedMs(elapsedMs: number): string {
  const totalSeconds = Math.max(0, Math.round(elapsedMs / 1000));
  if (totalSeconds < 1) {
    return "<1s";
  }

  const seconds = totalSeconds % 60;
  const totalMinutes = Math.floor(totalSeconds / 60);
  const minutes = totalMinutes % 60;
  const hours = Math.floor(totalMinutes / 60);

  if (hours > 0) {
    return `${hours}h ${String(minutes).padStart(2, "0")}m`;
  }

  if (totalMinutes > 0) {
    return `${totalMinutes}m ${String(seconds).padStart(2, "0")}s`;
  }

  return `${seconds}s`;
}

export const AwakenAssistantMessage = memo(function AwakenAssistantMessage({
  markdown,
  status = "active",
  elapsedMs,
  showFooter,
}: AwakenAssistantMessageProps) {
  const hasElapsedMs = typeof elapsedMs === "number" && Number.isFinite(elapsedMs);
  const shouldShowFooter = status === "done" && (showFooter ?? true);

  const className = `assistant-message assistant-message-${status}${
    shouldShowFooter ? " assistant-message-has-footer" : ""
  }`;

  return (
    <article className={className}>
      <AwakenMarkdownRenderer className="assistant-message-markdown" markdown={markdown} />
      {shouldShowFooter ? (
        <footer className="assistant-message-footer" aria-label="Assistant response actions">
          {hasElapsedMs ? (
            <span className="assistant-message-footer-meta">
              执行时间 {formatElapsedMs(elapsedMs)}
            </span>
          ) : null}
          <span className="assistant-message-footer-copy">
            <AwakenCopyButton
              text={markdown}
              label="Copy final answer"
              copiedLabel="Copied final answer"
              errorLabel="Copy failed"
              iconOnly
              showTooltip={false}
              feedbackDurationMs={2000}
              className="assistant-message-footer-copy-button"
              style={{
                width: "24px",
                height: "24px",
                border: 0,
                borderRadius: "4px",
                background: "transparent",
              }}
            />
          </span>
        </footer>
      ) : null}
    </article>
  );
});
