import "./AwakenThoughtTrace.css";
import { ChevronRight } from "lucide-react";
import {
  AwakenThinkingOrbStatus,
  type AwakenThinkingOrbTheme,
} from "../AwakenThinkingOrb";

type ThoughtTraceStatus = "active" | "done" | "error";

export type AwakenThoughtTraceProps = {
  text: string;
  status?: ThoughtTraceStatus;
  label?: string;
  durationLabel?: string;
  defaultOpen?: boolean;
  theme?: AwakenThinkingOrbTheme;
};

export function AwakenThoughtTrace({
  text,
  status = "active",
  label,
  durationLabel,
  defaultOpen = true,
  theme = "auto",
}: AwakenThoughtTraceProps) {
  const statusLabel =
    label ??
    (status === "active"
      ? "Thinking"
      : status === "error"
        ? "Thinking interrupted"
        : "Thought");
  const heading = durationLabel ? `${statusLabel} ${durationLabel}` : statusLabel;

  return (
    <details className={`thought-trace thought-trace-${status}`} open={defaultOpen}>
      <summary className="thought-trace-heading">
        {status === "active" ? (
          <AwakenThinkingOrbStatus
            className="thought-trace-status"
            label={heading}
            state="composing"
            size={20}
            speed={2.4}
            showEllipsis
            theme={theme}
          />
        ) : (
          <span className="thought-trace-disclosure">
            <span className="thought-trace-disclosure-icon" aria-hidden="true">
              <ChevronRight size={12} strokeWidth={1.8} />
            </span>
            <span className="thought-trace-disclosure-label">{heading}</span>
          </span>
        )}
      </summary>
      {text ? <p>{text}</p> : null}
    </details>
  );
}
