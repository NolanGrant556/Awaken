import "./AwakenThoughtTrace.css";
import { AwakenIcon } from "../AwakenIcon";
import { memo, useId, useState } from "react";
import { type AwakenThinkingOrbTheme } from "../AwakenThinkingOrb";
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
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

export const AwakenThoughtTrace = memo(function AwakenThoughtTrace({
  text,
  status = "active",
  label,
  durationLabel,
  defaultOpen,
  animateEntry = true,
  open,
  onOpenChange,
}: AwakenThoughtTraceProps) {
  const bodyId = useId();
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = Boolean(text.trim()) && (open ?? manualExpanded ?? false);
  const {
    present: bodyPresent,
    prepareOpen,
    onTransitionEnd,
  } = useAwakenDisclosurePresence(expanded, 180);
  const statusLabel =
    label ??
    (status === "active"
      ? "思考中"
      : status === "error"
        ? "思考已中断"
        : "思考过程");
  const heading = durationLabel
    ? `${statusLabel} ${durationLabel}`
    : statusLabel;
  function toggle() {
    if (!expanded) prepareOpen();
    if (open === undefined) setManualExpanded(!expanded);
    onOpenChange?.(!expanded);
  }
  const headingContent = (
    <>
      <span className="thought-trace-disclosure-label">{heading}</span>
      {status === "active" ? (
        <span className="aw-trace-dots" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
      ) : null}
      {text.trim() ? (
        <span className="thought-trace-disclosure-icon" aria-hidden="true">
          <AwakenIcon concept="disclose" size={12} />
        </span>
      ) : null}
    </>
  );

  return (
    <section
      className={`thought-trace thought-trace-${status}${
        expanded ? " thought-trace-open" : ""
      }`}
      data-state={status}
      data-animate-entry={animateEntry ? "true" : "false"}
      data-disclosure-state={
        expanded ? "open" : bodyPresent ? "closing" : "closed"
      }
    >
      {text.trim() ? (
        <button
          type="button"
          className="thought-trace-heading"
          aria-expanded={expanded}
          aria-controls={bodyId}
          onClick={toggle}
        >
          {headingContent}
        </button>
      ) : (
        <div className="thought-trace-heading" role="status">
          {headingContent}
        </div>
      )}
      <div
        id={bodyId}
        className="thought-trace-body"
        data-expanded={expanded ? "true" : "false"}
        aria-hidden={!expanded}
        inert={!expanded}
        onTransitionEnd={onTransitionEnd}
      >
        {bodyPresent ? (
          <div className="thought-trace-body-inner">
            {text ? <p>{text}</p> : null}
            {text.length > 600 ? (
              <button
                className="thought-trace-collapse"
                type="button"
                onClick={(event) => {
                  event.currentTarget.dispatchEvent(
                    new CustomEvent("aw-trace-collapse", { bubbles: true }),
                  );
                  toggle();
                }}
              >
                收起思考正文
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
});
