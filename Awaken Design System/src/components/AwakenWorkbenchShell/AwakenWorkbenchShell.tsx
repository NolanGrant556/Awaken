import "./AwakenWorkbenchShell.css";
import type { HTMLAttributes, ReactNode } from "react";

export type AwakenWorkbenchShellProps = {
  stage: ReactNode;
  inspector?: ReactNode;
  inspectorCollapsed?: boolean;
  inspectorExpanded?: boolean;
  inspectorSurfaceActive?: boolean;
  inspectorResizeHandleProps?: HTMLAttributes<HTMLDivElement>;
  collapsedInspectorControl?: ReactNode;
};

export function AwakenWorkbenchShell({
  stage,
  inspector,
  inspectorCollapsed,
  inspectorExpanded,
  inspectorSurfaceActive = true,
  inspectorResizeHandleProps,
  collapsedInspectorControl,
}: AwakenWorkbenchShellProps) {
  const shellClassName = `aw-workbench-shell${
    inspectorCollapsed ? " aw-workbench-shell-inspector-collapsed" : ""
  }${
    inspectorExpanded ? " aw-workbench-shell-inspector-expanded" : ""
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
      {inspector &&
      inspectorResizeHandleProps &&
      !inspectorCollapsed &&
      !inspectorExpanded ? (
        <div
          {...inspectorResizeHandleProps}
          className={inspectorResizeHandleClass}
          style={inspectorResizeHandleProps.style}
        />
      ) : null}
      {inspector ? (
        <aside
          className={`aw-workbench-inspector${
            inspectorExpanded && !inspectorSurfaceActive
              ? " aw-workbench-inspector-surface-hidden"
              : ""
          }`}
          aria-label="Inspector"
          aria-hidden={
            inspectorExpanded && !inspectorSurfaceActive ? true : undefined
          }
          inert={inspectorExpanded && !inspectorSurfaceActive}
        >
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
