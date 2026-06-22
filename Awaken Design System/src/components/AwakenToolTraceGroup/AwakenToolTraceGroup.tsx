import "./AwakenToolTraceGroup.css";
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
  onSelectAction?: (action: ToolTraceAction) => void;
};

export function AwakenToolTraceGroup({
  actions,
  label,
  onSelectAction,
}: AwakenToolTraceGroupProps) {
  return (
    <article className="tool-trace-group">
      {label ? <div className="tool-trace-group-label">{label}</div> : null}
      <div className="tool-trace-group-lines">
        {actions.map((action) => (
          <AwakenToolTraceLine
            key={action.id}
            title={action.title}
            detail={action.detail}
            status={action.status}
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
    </article>
  );
}
