import type { ReactNode } from "react";
import { Popover } from "../overlays/Popover";
import type { ModelSelectorPlacement } from "../../types";

type DropdownProps = {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  placement?: ModelSelectorPlacement;
  offset?: number;
};

export function Dropdown({
  trigger,
  children,
  open,
  onOpenChange,
  placement = "bottom-end",
  offset = 6,
}: DropdownProps) {
  if (!children) return <>{trigger}</>;

  return (
    <Popover
      trigger={trigger}
      content={
        <div
          style={{
            background: "var(--aw-bg-white)",
            border: "1px solid var(--aw-border-main)",
            borderRadius: "var(--aw-radius-md)",
            boxShadow: "var(--aw-shadow-menu)",
            padding: "4px",
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
