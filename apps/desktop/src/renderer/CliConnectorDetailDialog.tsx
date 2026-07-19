import { Check } from "lucide-react";
import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import type {
  CliAuthSessionView,
  CliConnectorReadiness,
  CliInstallProgress,
  RuntimeConnector,
} from "../shared/runtime";
import awakenLogo from "./assets/brands/awaken.png";
import { ConnectorBrandIcon } from "./ConnectorBrandIcon";
import { cliConnectorProductCopy } from "./cliConnectorProduct";

export type CliConnectorDialogAction =
  | "install"
  | "cancel_install"
  | "authorize"
  | "cancel_authorization"
  | "retry_authorization"
  | "logout"
  | "unsupported";

export function cliConnectorDialogAction(
  readiness: CliConnectorReadiness,
  authSession?: CliAuthSessionView,
): CliConnectorDialogAction {
  if (readiness.install === "unsupported") return "unsupported";
  if (readiness.install === "installing") return "cancel_install";
  if (readiness.install !== "installed" || readiness.executable !== "available") return "install";
  if (readiness.authentication === "authorized") return "logout";
  if (authSession && ["failed", "cancelled"].includes(authSession.phase)) {
    return "retry_authorization";
  }
  if (authSession) return "cancel_authorization";
  return "retry_authorization";
}

type CliConnectorDetailDialogProps = {
  open: boolean;
  connector: RuntimeConnector | null;
  readiness: CliConnectorReadiness | null;
  authSession?: CliAuthSessionView;
  installProgress?: CliInstallProgress;
  loadingProfile?: boolean;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onAction: (action: Exclude<CliConnectorDialogAction, "unsupported">) => void;
};

const actionLabels: Record<CliConnectorDialogAction, string> = {
  install: "安装",
  cancel_install: "取消安装",
  authorize: "点击授权",
  cancel_authorization: "取消授权",
  retry_authorization: "重新授权",
  logout: "已连接",
  unsupported: "当前平台不支持",
};

export function CliConnectorDetailDialog({
  open,
  connector,
  readiness,
  authSession,
  installProgress,
  loadingProfile = false,
  busy = false,
  onOpenChange,
  onAction,
}: CliConnectorDetailDialogProps) {
  if (!connector || !readiness) return null;

  const product = cliConnectorProductCopy(connector.id);
  const displayName = product?.displayName || connector.displayName;
  const description = product?.description || connector.description?.trim() || "官方 CLI 连接器";
  const action = cliConnectorDialogAction(readiness, authSession);
  const connected = action === "logout";
  const installing = action === "cancel_install";
  const waiting = action === "cancel_authorization";
  const name = readiness.account?.displayName?.trim() || (loadingProfile && connected ? "加载中…" : "—");
  const organization = readiness.account?.organization?.trim() || (loadingProfile && connected ? "加载中…" : "—");
  const title = connected
    ? `已连接到 ${displayName}`
    : installing
      ? `正在安装 ${displayName}`
      : readiness.install === "installed"
        ? `授权 ${displayName}`
        : `连接到 ${displayName}`;
  const status = connected
    ? "已连接"
    : installing
      ? installProgress?.label || "正在准备安装"
      : waiting
        ? "请在已打开的官方网页完成授权"
        : readiness.install === "installed"
          ? readiness.safeError?.message || "CLI 与官方 Skills 已安装，当前未授权"
          : readiness.safeError?.message || "尚未安装";
  const progress = Math.max(0, Math.min(100, installProgress?.percent ?? 0));
  const actionable = action !== "unsupported";
  const handleOpenChange = (nextOpen: boolean): void => {
    if (!nextOpen && action === "cancel_authorization") {
      onAction("cancel_authorization");
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog
      open={open}
      className="cli-connector-detail-dialog"
      title={title}
      titleVisuallyHidden
      onOpenChange={handleOpenChange}
      footer={
        <Button
          className={[
            "cli-connector-detail-action",
            action === "cancel_install" || action === "cancel_authorization" ? "cli-connector-detail-action-cancel" : "",
            action === "logout" ? "cli-connector-detail-action-connected" : "",
          ].filter(Boolean).join(" ")}
          variant={action === "install" || action === "authorize" || action === "retry_authorization" ? "primary" : "default"}
          size="md"
          disabled={busy || !actionable}
          aria-label={action === "logout" ? `取消登录 ${displayName}` : actionLabels[action]}
          onClick={() => {
            if (actionable) onAction(action);
          }}
        >
          {action === "logout" ? (
            <>
              <span className="cli-connector-connected-label"><Check size={15} strokeWidth={2.4} />已连接</span>
              <span className="cli-connector-logout-label">取消登录</span>
            </>
          ) : (
            actionLabels[action]
          )}
        </Button>
      }
    >
      <div className="cli-connector-detail">
        <div className="cli-connector-detail-brands" aria-hidden="true">
          <span className="cli-connector-detail-awaken">
            <img src={awakenLogo} alt="" draggable={false} />
          </span>
          <span className={["cli-connector-detail-link", connected ? "is-connected" : ""].filter(Boolean).join(" ")}>
            {connected ? <Check size={14} strokeWidth={2.4} /> : <span>·</span>}
          </span>
          <ConnectorBrandIcon providerId={connector.id} />
        </div>
        <h2 className="cli-connector-detail-title">{title}</h2>
        <p className="cli-connector-detail-description">{description}</p>

        {installing ? (
          <div className="cli-connector-install-progress" aria-live="polite">
            <div className="cli-connector-install-progress-copy">
              <span>{status}</span>
              <strong>{progress}%</strong>
            </div>
            <div
              className="cli-connector-install-progress-track"
              role="progressbar"
              aria-label={`${displayName} 安装进度`}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={progress}
            >
              <span style={{ width: `${progress}%` }} />
            </div>
            <p>将同步安装官方 CLI 与官方 Skills</p>
          </div>
        ) : connected ? (
          <div className="cli-connector-detail-account">
            <div className="cli-connector-detail-status">
              <span className="cli-connector-detail-status-dot" />
              {status}
            </div>
            <div className="cli-connector-detail-row"><span>姓名</span><strong>{name}</strong></div>
            <div className="cli-connector-detail-row"><span>组织</span><strong>{organization}</strong></div>
          </div>
        ) : (
          <div className="cli-connector-detail-state">
            <span className={["cli-connector-detail-state-dot", waiting ? "is-waiting" : ""].filter(Boolean).join(" ")} />
            <span>{status}</span>
          </div>
        )}
      </div>
    </Dialog>
  );
}
