import { Check } from "lucide-react";
import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import type { CliConnectorReadiness, RuntimeConnector } from "../shared/runtime";
import awakenLogo from "./assets/brands/awaken.png";
import { ConnectorBrandIcon } from "./ConnectorBrandIcon";
import { cliConnectorProductCopy } from "./cliConnectorProduct";

type CliConnectorDetailDialogProps = {
  open: boolean;
  connector: RuntimeConnector | null;
  readiness: CliConnectorReadiness | null;
  loadingProfile?: boolean;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onLogout: () => void;
};

export function CliConnectorDetailDialog({
  open,
  connector,
  readiness,
  loadingProfile = false,
  busy = false,
  onOpenChange,
  onLogout,
}: CliConnectorDetailDialogProps) {
  if (!connector || !readiness) {
    return null;
  }

  const product = cliConnectorProductCopy(connector.id);
  const displayName = product?.displayName || connector.displayName;
  const description =
    product?.description || connector.description?.trim() || "官方 CLI 连接器";
  const name = readiness.account?.displayName?.trim() || (loadingProfile ? "加载中…" : "—");
  const organization =
    readiness.account?.organization?.trim() || (loadingProfile ? "加载中…" : "—");

  return (
    <Dialog
      open={open}
      className="cli-connector-detail-dialog"
      title={`已连接到 ${displayName}`}
      titleVisuallyHidden
      onOpenChange={onOpenChange}
      footer={
        <Button
          className="cli-connector-logout-button"
          variant="default"
          size="md"
          disabled={busy || loadingProfile}
          onClick={onLogout}
        >
          退出登录
        </Button>
      }
    >
      <div className="cli-connector-detail">
        <div className="cli-connector-detail-brands" aria-hidden="true">
          <span className="cli-connector-detail-awaken">
            <img src={awakenLogo} alt="" draggable={false} />
          </span>
          <span className="cli-connector-detail-link">
            <Check size={14} strokeWidth={2.4} />
          </span>
          <ConnectorBrandIcon providerId={connector.id} />
        </div>
        <h2 className="cli-connector-detail-title">已连接到 {displayName}</h2>
        <p className="cli-connector-detail-description">{description}</p>
        <div className="cli-connector-detail-account">
          <div className="cli-connector-detail-status">
            <span className="cli-connector-detail-status-dot" />
            已连接
          </div>
          <div className="cli-connector-detail-row">
            <span>姓名</span>
            <strong>{name}</strong>
          </div>
          <div className="cli-connector-detail-row">
            <span>组织</span>
            <strong>{organization}</strong>
          </div>
        </div>
      </div>
    </Dialog>
  );
}
