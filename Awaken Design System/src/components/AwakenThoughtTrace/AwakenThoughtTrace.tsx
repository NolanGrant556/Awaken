import "./AwakenThoughtTrace.css";
import { AwakenIcon } from "../AwakenIcon";
import { memo, useId, useState } from "react";
import {
  AwakenThinkingOrbStatus,
  type AwakenThinkingOrbTheme,
} from "../AwakenThinkingOrb";
import { useAwakenDisclosurePresence } from "../../hooks/useAwakenDisclosurePresence";

type ThoughtTraceStatus = "active" | "done" | "error";

export type AwakenThoughtTraceProps = {
  text: string;
  status?: ThoughtTraceStatus;
  label?: string;
  durationLabel?: string;
  defaultOpen?: boolean;
  theme?: AwakenThinkingOrbTheme;
  animateEntry?: boolean;
};

export const AwakenThoughtTrace = memo(function AwakenThoughtTrace({
  text,
  status = "active",
  label,
  durationLabel,
  defaultOpen,
  theme = "auto",
  animateEntry = true,
}: AwakenThoughtTraceProps) {
  const bodyId = useId();
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const autoExpanded = status === "active";
  const expanded = manualExpanded ?? autoExpanded;
  const { present: bodyPresent, prepareOpen } = useAwakenDisclosurePresence(
    expanded,
    400,
  );
  const statusLabel =
    label ??
    (status === "active"
      ? "思考中"
      : status === "error"
        ? "思考已中断"
        : "思考过程");
  const heading = durationLabel ? `${statusLabel} ${durationLabel}` : statusLabel;

  return (
    <section
      className={`thought-trace thought-trace-${status}${
        expanded ? " thought-trace-open" : ""
      }`}
      data-state={status}
      data-animate-entry={animateEntry ? "true" : "false"}
    >
      <button
        type="button"
        className="thought-trace-heading"
        aria-expanded={expanded}
        aria-controls={bodyId}
        onClick={() => {
          const nextExpanded = !(manualExpanded ?? autoExpanded);
          if (nextExpanded) prepareOpen();
          setManualExpanded(nextExpanded);
        }}
      >
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
            <span className="thought-trace-disclosure-label">{heading}</span>
          </span>
        )}
        <span className="thought-trace-disclosure-icon" aria-hidden="true">
          <AwakenIcon concept="disclose" size={12} />
        </span>
      </button>
      <div
        id={bodyId}
        className="thought-trace-body"
        data-expanded={expanded ? "true" : "false"}
        aria-hidden={!expanded}
        inert={!expanded}
      >
        {bodyPresent ? (
          <div className="thought-trace-body-inner">
            {text ? <p>{text}</p> : null}
          </div>
        ) : null}
      </div>
    </section>
  );
});
