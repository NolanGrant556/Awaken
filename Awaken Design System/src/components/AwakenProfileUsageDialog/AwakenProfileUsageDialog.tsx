import "./AwakenProfileUsageDialog.css";
import { AwakenDialog } from "../AwakenDialog";
import {
  AwakenProfileBalancePanel,
  type AwakenProfileBalancePanelProps,
} from "../AwakenProfileBalancePanel";

export type AwakenProfileUsageDialogProps = AwakenProfileBalancePanelProps & {
  open: boolean;
  title?: string;
  onOpenChange: (open: boolean) => void;
};

export function AwakenProfileUsageDialog({
  open,
  title = "用量",
  onOpenChange,
  className,
  ...panelProps
}: AwakenProfileUsageDialogProps) {
  return (
    <AwakenDialog
      open={open}
      title={title}
      size="large"
      className="aw-profile-usage-dialog"
      bodyClassName="aw-profile-usage-dialog__body"
      onOpenChange={onOpenChange}
    >
      <AwakenProfileBalancePanel
        {...panelProps}
        className={["aw-profile-usage-dialog__panel", className ?? ""].filter(Boolean).join(" ")}
      />
    </AwakenDialog>
  );
}
