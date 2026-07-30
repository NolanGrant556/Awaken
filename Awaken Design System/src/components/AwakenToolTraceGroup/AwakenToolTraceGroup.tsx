import "./AwakenToolTraceGroup.css";
import { ChevronRight, LoaderCircle, Wrench } from "lucide-react";
import { useId, useState } from "react";
import {
  AwakenToolTraceLine,
  type AwakenToolTraceLineProps,
  type ToolTraceStatus,
} from "../AwakenToolTraceLine";

export type ToolTraceAction = Omit<AwakenToolTraceLineProps, "onSelect"> & {
  id: string;
  status: ToolTraceStatus;
};

export type AwakenToolTraceGroupProps = {
  actions: ToolTraceAction[];
  label?: string;
  defaultOpen?: boolean;
  /** Disable child mount animations when revealing already-finished history. */
  animateItems?: boolean;
  onSelectAction?: (action: ToolTraceAction) => void;
};

export function AwakenToolTraceGroup({
  actions,
  label,
  defaultOpen,
  animateItems = true,
  onSelectAction,
}: AwakenToolTraceGroupProps) {
  const bodyId = useId();
  const active = actions.some((action) => action.status === "active");
  const failed = actions.filter((action) => action.status !== "active" && action.status !== "done").length;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = manualExpanded ?? active;
  const summary = label || `${actions.length} 个工具调用`;
  const detail = active ? "执行中" : failed > 0 ? `${failed} 个异常` : "已完成";

  return (
    <article className={`tool-trace-group${expanded ? " tool-trace-group-open" : ""}`}>
      <button
        type="button"
        className="tool-trace-group-heading"
        aria-expanded={expanded}
        aria-controls={bodyId}
        onClick={() => setManualExpanded((current) => !(current ?? active))}
      >
        <span className="tool-trace-group-icon" aria-hidden="true">
          {active ? <LoaderCircle size={14} /> : <Wrench size={14} />}
        </span>
        <span className="tool-trace-group-label">{summary}</span>
        <span className="tool-trace-group-detail">{detail}</span>
        <span className="tool-trace-group-chevron" aria-hidden="true">
          <ChevronRight size={13} strokeWidth={1.8} />
        </span>
      </button>
      <div
        id={bodyId}
        className="tool-trace-group-disclosure"
        data-expanded={expanded ? "true" : "false"}
        aria-hidden={!expanded}
        inert={!expanded}
      >
        <div className="tool-trace-group-lines">
          {actions.map((action) => (
            <AwakenToolTraceLine
              key={action.id}
              {...action}
              animateEntry={animateItems && action.animateEntry !== false}
              onSelect={
                onSelectAction
                  ? () => {
                      onSelectAction(action);
                    }
                  : undefined
              }
            />
          ))}
        </div>
      </div>
    </article>
  );
}
