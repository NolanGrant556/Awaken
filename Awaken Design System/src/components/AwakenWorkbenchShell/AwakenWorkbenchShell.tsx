import "./AwakenWorkbenchShell.css";
import type { ReactNode } from "react";

export type AwakenWorkbenchShellProps = {
  stage: ReactNode;
  inspector?: ReactNode;
};

export function AwakenWorkbenchShell({ stage, inspector }: AwakenWorkbenchShellProps) {
  return (
    <div className="aw-workbench-shell">
      {stage}
      {inspector ? (
        <aside className="aw-workbench-inspector" aria-label="Inspector">
          {inspector}
        </aside>
      ) : null}
    </div>
  );
}
