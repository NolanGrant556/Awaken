import "./AwakenDropdown.css";
import type { ReactNode } from "react";
import type { Placement } from "@floating-ui/react";
import { AwakenPopover } from "../AwakenPopover";

export type AwakenDropdownProps = {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  placement?: Placement;
  offset?: number;
};

export function AwakenDropdown({
  trigger,
  children,
  open,
  onOpenChange,
  placement = "bottom-end",
  offset = 6,
}: AwakenDropdownProps) {
  if (!children) return <>{trigger}</>;

  return (
    <AwakenPopover
      trigger={trigger}
      content={
        <div
          style={{
            background: "var(--aw-bg-white)",
            border: "1px solid var(--aw-border-main)",
            borderRadius: "var(--aw-radius-panel)",
            boxShadow: "var(--aw-shadow-menu)",
            padding: "6px",
            minWidth: "180px",
          }}
        >
          {children}
        </div>
      }
      open={open}
      onOpenChange={onOpenChange}
      placement={placement}
      offset={offset}
      matchTriggerWidth={false}
    />
  );
}
