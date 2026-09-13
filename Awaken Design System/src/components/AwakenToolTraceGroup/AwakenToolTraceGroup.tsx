import "./AwakenToolTraceGroup.css";
import { AwakenIcon } from "../AwakenIcon";
import { Fragment, memo, useId, useState, type ReactNode } from "react";
import {
  AwakenToolTraceLine,
  type AwakenToolTraceLineProps,
  type ToolTraceStatus,
} from "../AwakenToolTraceLine";
import { useAwakenDisclosurePresence } from "../../hooks/useAwakenDisclosurePresence";

export type ToolTraceAction = Omit<AwakenToolTraceLineProps, "onSelect"> & {
  id: string;
  status: ToolTraceStatus;
};

export type AwakenToolTraceGroupProps = {
  actions: ToolTraceAction[];
  label?: string;
  detail?: string;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  icon?: ReactNode;
  /** Disable child mount animations when revealing already-finished history. */
  animateItems?: boolean;
  onSelectAction?: (action: ToolTraceAction) => void;
  renderAction?: (
    action: ToolTraceAction,
    lineProps: AwakenToolTraceLineProps,
  ) => ReactNode;
  children?: ReactNode;
  entryCount?: number;
  currentActivity?: string;
  attention?: string;
  activityStatus?: "active" | "done";
};

export const AwakenToolTraceGroup = memo(function AwakenToolTraceGroup({
  actions,
  label,
  detail: detailProp,
  defaultOpen,
  open,
  onOpenChange,
  animateItems = true,
  onSelectAction,
  renderAction,
  children,
  entryCount,
  currentActivity,
  attention,
  activityStatus,
}: AwakenToolTraceGroupProps) {
  const bodyId = useId();
  const active =
    activityStatus === "active" ||
    actions.some((action) => action.status === "active");
  const failed = actions.filter(
    (action) => action.status !== "active" && action.status !== "done",
  ).length;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const singleton = entryCount !== undefined && entryCount <= 1;
  const expanded = singleton || (open ?? manualExpanded ?? false);
  const {
    present: linesPresent,
    prepareOpen,
    onTransitionEnd,
  } = useAwakenDisclosurePresence(expanded, 180);
  const summary = label || `${actions.length} 个工具调用`;
  const detail = detailProp ?? (failed > 0 ? `${failed} 个异常` : undefined);

  function toggleOpen() {
    const nextOpen = !(open ?? manualExpanded ?? false);
    if (nextOpen) prepareOpen();
    if (open === undefined) setManualExpanded(nextOpen);
    onOpenChange?.(nextOpen);
  }

  return (
    <article
      className={`tool-trace-group${expanded ? " tool-trace-group-open" : ""}${active ? " tool-trace-group-active" : ""}`}
      data-disclosure-state={
        expanded ? "open" : linesPresent ? "closing" : "closed"
      }
    >
      {!singleton ? (
        <button
          type="button"
          className="tool-trace-group-heading"
          aria-expanded={expanded}
          aria-controls={bodyId}
          onClick={toggleOpen}
        >
          <span className="tool-trace-group-label">{summary}</span>
          {detail ? (
            <span className="tool-trace-group-detail">{detail}</span>
          ) : null}
          <span className="tool-trace-group-chevron" aria-hidden="true">
            <AwakenIcon concept="disclose" size={14} />
          </span>
        </button>
      ) : null}
      <div
        id={bodyId}
        className="tool-trace-group-disclosure"
        data-expanded={expanded ? "true" : "false"}
        aria-hidden={!expanded}
        inert={!expanded}
        onTransitionEnd={onTransitionEnd}
      >
        {linesPresent ? (
          <div className="tool-trace-group-lines">
            {children ??
              actions.map((action) => {
                const { id, ...actionProps } = action;
                const lineProps: AwakenToolTraceLineProps = {
                  ...actionProps,
                  animateEntry: animateItems && action.animateEntry !== false,
                  onSelect: onSelectAction
                    ? () => {
                        onSelectAction(action);
                      }
                    : undefined,
                };
                return renderAction ? (
                  <Fragment key={id}>
                    {renderAction(action, lineProps)}
                  </Fragment>
                ) : (
                  <AwakenToolTraceLine key={id} {...lineProps} />
                );
              })}
          </div>
        ) : null}
      </div>
      {!expanded && currentActivity ? (
        <div className="tool-trace-current" role="status">
          <span>{currentActivity}</span>
          <span className="aw-trace-dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
        </div>
      ) : null}
      {!expanded && attention ? (
        <div className="tool-trace-attention">{attention}</div>
      ) : null}
    </article>
  );
});
