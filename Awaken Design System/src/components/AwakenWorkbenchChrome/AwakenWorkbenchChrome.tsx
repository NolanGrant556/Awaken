import "./AwakenWorkbenchChrome.css";
import type { ReactNode } from "react";
import type { PlatformKind } from "../../types";
import { MessageCirclePlus, PanelLeft } from "lucide-react";

export type AwakenWorkbenchChromeProps = {
  sidebarCollapsed?: boolean;
  onToggleSidebar?: () => void;
  onNewTask?: () => void;
  center?: ReactNode;
  right?: ReactNode;
  platform?: PlatformKind;
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
  return (
    <button
      type="button"
      className={`workbench-chrome-icon${active ? " is-active" : ""}`}
      aria-label={label}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

export function AwakenWorkbenchChrome({
  sidebarCollapsed,
  onToggleSidebar,
  onNewTask,
  center,
  right,
  platform,
}: AwakenWorkbenchChromeProps) {
  return (
    <header
      className={`workbench-chrome${
        sidebarCollapsed ? " workbench-chrome-collapsed" : ""
      }${platform !== "macos" ? " workbench-chrome-no-spacer" : ""}`}
      aria-label="Workbench controls"
    >
      <div className="workbench-chrome-left">
        {platform === "macos" ? (
          <div className="workbench-chrome-window-spacer" aria-hidden="true" />
        ) : null}
        <ChromeIconButton
          label={sidebarCollapsed ? "Show sidebar" : "Hide sidebar"}
          onClick={onToggleSidebar}
        >
          <PanelLeft size={14} strokeWidth={1.9} />
        </ChromeIconButton>
        {sidebarCollapsed ? (
          <ChromeIconButton label="New Agent" onClick={onNewTask}>
            <MessageCirclePlus size={14} strokeWidth={1.9} />
          </ChromeIconButton>
        ) : null}
      </div>
      <div className="workbench-chrome-center">{center}</div>
      <div className="workbench-chrome-right">{right}</div>
    </header>
  );
}
