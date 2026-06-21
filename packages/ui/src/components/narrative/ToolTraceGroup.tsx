import {
  ToolTraceLine,
  type ToolTraceLineProps,
  type ToolTraceStatus,
} from "./ToolTraceLine";

export type ToolTraceAction = Omit<ToolTraceLineProps, "onSelect"> & {
  id: string;
  status: ToolTraceStatus;
};

type ToolTraceGroupProps = {
  actions: ToolTraceAction[];
  label?: string;
  onSelectAction?: (action: ToolTraceAction) => void;
};

export function ToolTraceGroup({
  actions,
  label,
  onSelectAction,
}: ToolTraceGroupProps) {
  return (
    <article className="tool-trace-group">
      {label ? <div className="tool-trace-group-label">{label}</div> : null}
      <div className="tool-trace-group-lines">
        {actions.map((action) => (
          <ToolTraceLine
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
