import type { ReactNode } from "react";

type WorkbenchShellProps = {
  stage: ReactNode;
  inspector?: ReactNode;
};

export function WorkbenchShell({ stage, inspector }: WorkbenchShellProps) {
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
