import "./AwakenWorkbenchShell.css";
import type { HTMLAttributes, ReactNode } from "react";

export type AwakenWorkbenchShellProps = {
  stage: ReactNode;
  inspector?: ReactNode;
  inspectorCollapsed?: boolean;
  inspectorResizeHandleProps?: HTMLAttributes<HTMLDivElement>;
  collapsedInspectorControl?: ReactNode;
};

export function AwakenWorkbenchShell({
  stage,
  inspector,
  inspectorCollapsed,
  inspectorResizeHandleProps,
  collapsedInspectorControl,
}: AwakenWorkbenchShellProps) {
  const shellClassName = `aw-workbench-shell${
    inspectorCollapsed ? " aw-workbench-shell-inspector-collapsed" : ""
  }`;
  const inspectorResizeHandleClass = [
    "aw-workbench-inspector-resize-handle",
    inspectorResizeHandleProps?.className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={shellClassName}>
      {stage}
      {inspector && inspectorResizeHandleProps && !inspectorCollapsed ? (
        <div
          {...inspectorResizeHandleProps}
          className={inspectorResizeHandleClass}
          style={inspectorResizeHandleProps.style}
        />
      ) : null}
      {inspector ? (
        <aside className="aw-workbench-inspector" aria-label="Inspector">
          {inspectorCollapsed ? (
            <div className="aw-workbench-inspector-collapsed-slot">
              {collapsedInspectorControl}
            </div>
          ) : (
            inspector
          )}
        </aside>
      ) : null}
    </div>
  );
}
