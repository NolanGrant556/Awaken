import type { ReactNode } from "react";
import { MessageCirclePlus, PanelLeft } from "lucide-react";
import { Tooltip } from "../overlays/Tooltip";

type WorkbenchChromeProps = {
  sidebarCollapsed?: boolean;
  onToggleSidebar?: () => void;
  onNewTask?: () => void;
  right?: ReactNode;
};

function ChromeIconButton({
  label,
  onClick,
  children,
  active,
}: {
  label: string;
  onClick?: () => void;
  children: ReactNode;
  active?: boolean;
}) {
  const button = (
    <button
      type="button"
      className={`workbench-chrome-icon${active ? " is-active" : ""}`}
      aria-label={label}
      title={label}
      onClick={onClick}
    >
      {children}
    </button>
  );

  return <Tooltip label={label}>{button}</Tooltip>;
}

export function WorkbenchChrome({
  sidebarCollapsed,
  onToggleSidebar,
  onNewTask,
  right,
}: WorkbenchChromeProps) {
  return (
    <header
      className={`workbench-chrome${
        sidebarCollapsed ? " workbench-chrome-collapsed" : ""
      }`}
      aria-label="Workbench controls"
    >
      <div className="workbench-chrome-left">
        <div className="workbench-chrome-window-spacer" aria-hidden="true" />
        <ChromeIconButton
          label={sidebarCollapsed ? "Show sidebar" : "Hide sidebar"}
          onClick={onToggleSidebar}
          active
        >
          <PanelLeft size={20} strokeWidth={1.9} />
        </ChromeIconButton>
        {sidebarCollapsed ? (
          <ChromeIconButton label="New Agent" onClick={onNewTask}>
            <MessageCirclePlus size={20} strokeWidth={1.9} />
          </ChromeIconButton>
        ) : null}
      </div>
      <div className="workbench-chrome-right">{right}</div>
    </header>
  );
}
