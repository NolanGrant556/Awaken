import "./AwakenCliConnectorDetailDialog.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenButton } from "../AwakenButton";
import { AwakenConnectorBrandIcon } from "../AwakenConnectorBrandIcon";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenProductLogo } from "../AwakenProductLogo";
import type {
  AwakenCliAuthSession,
  AwakenCliConnectorAction,
  AwakenCliConnectorReadiness,
  AwakenCliInstallProgress,
} from "../AwakenConnectors/AwakenConnectors.types";

export function awakenCliConnectorDialogAction(
  readiness: AwakenCliConnectorReadiness,
  authSession?: AwakenCliAuthSession,
): AwakenCliConnectorAction {
  if (readiness.install === "unsupported") return "unsupported";
  if (readiness.install === "installing") return "cancel_install";
  if (readiness.install !== "installed" || readiness.executable !== "available") return "install";
  if (readiness.authentication === "authorized") return "logout";
  if (authSession && ["failed", "cancelled"].includes(authSession.phase)) return "retry_authorization";
  if (authSession) return "cancel_authorization";
  return "retry_authorization";
}

export type AwakenCliConnectorDetailDialogProps = {
  open: boolean;
  providerId: string;
  displayName: string;
  description: string;
  readiness: AwakenCliConnectorReadiness | null;
  authSession?: AwakenCliAuthSession;
  installProgress?: AwakenCliInstallProgress;
  brandLogoSrc?: string;
  brandTone?: "dingtalk" | "lark" | "neutral";
  loadingProfile?: boolean;
  busy?: boolean;
  errorMessage?: string | null;
  onOpenChange: (open: boolean) => void;
  onAction: (action: Exclude<AwakenCliConnectorAction, "unsupported">) => void;
};

const actionLabels: Record<AwakenCliConnectorAction, string> = {
  install: "安装",
  cancel_install: "取消安装",
  authorize: "点击授权",
  cancel_authorization: "取消授权",
  retry_authorization: "重新授权",
  logout: "已连接",
  unsupported: "当前平台不支持",
};

export function AwakenCliConnectorDetailDialog({
  open,
  providerId,
  displayName,
  description,
  readiness,
  authSession,
  installProgress,
  brandLogoSrc,
  brandTone = "neutral",
  loadingProfile = false,
  busy = false,
  errorMessage,
  onOpenChange,
  onAction,
}: AwakenCliConnectorDetailDialogProps) {
  if (!readiness) return (
    <AwakenDialog open={open} title={`连接到 ${displayName}`} onOpenChange={onOpenChange}>
      <p role={errorMessage ? "alert" : "status"}>{errorMessage || (loadingProfile ? "正在读取连接状态…" : "连接状态暂不可用，请关闭后重试。")}</p>
    </AwakenDialog>
  );

  const action = awakenCliConnectorDialogAction(readiness, authSession);
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
          ? readiness.safeErrorMessage || "运行组件与官方技能已安装，当前未授权"
          : readiness.safeErrorMessage || "尚未安装";
  const progress = Math.max(0, Math.min(100, installProgress?.percent ?? 0));
  const actionable = action !== "unsupported";

  function handleOpenChange(nextOpen: boolean): void {
    if (!nextOpen && action === "cancel_authorization") {
      onAction("cancel_authorization");
    }
    onOpenChange(nextOpen);
  }

  return (
    <AwakenDialog
      open={open}
      className="aw-cli-connector-detail-dialog"
      title={title}
      titleVisuallyHidden
      onOpenChange={handleOpenChange}
      footer={
        <AwakenButton
          className="aw-cli-connector-detail__action"
          data-action={action}
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
              <span className="aw-cli-connector-detail__connected-label"><AwakenIcon concept="step-done" size={16} />已连接</span>
              <span className="aw-cli-connector-detail__logout-label">取消登录</span>
            </>
          ) : actionLabels[action]}
        </AwakenButton>
      }
    >
      <div className="aw-cli-connector-detail">
        <div className="aw-cli-connector-detail__brands" aria-hidden="true">
          <AwakenProductLogo size="md" variant="icon" decorative />
          <span className="aw-cli-connector-detail__link" data-connected={connected ? "true" : "false"}>
            {connected ? <AwakenIcon concept="step-done" size={14} /> : <span>·</span>}
          </span>
          <AwakenConnectorBrandIcon
            providerId={providerId}
            imageSrc={brandLogoSrc}
            label={displayName}
            tone={brandTone}
            size="sm"
          />
        </div>
        <h2 className="aw-cli-connector-detail__title">{title}</h2>
        <p className="aw-cli-connector-detail__description">{description}</p>
        {errorMessage ? <p role="alert">{errorMessage}</p> : null}

        {installing ? (
          <div className="aw-cli-connector-detail__progress" aria-live="polite">
            <div className="aw-cli-connector-detail__progress-copy">
              <span>{status}</span>
              <strong>{progress}%</strong>
            </div>
            <div
              className="aw-cli-connector-detail__progress-track"
              role="progressbar"
              aria-label={`${displayName} 安装进度`}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={progress}
            >
              <span style={{ width: `${progress}%` }} />
            </div>
            <p>将同步安装所需组件与官方技能</p>
          </div>
        ) : connected ? (
          <div className="aw-cli-connector-detail__account">
            <div className="aw-cli-connector-detail__status"><span />{status}</div>
            <div className="aw-cli-connector-detail__row"><span>姓名</span><strong>{name}</strong></div>
            <div className="aw-cli-connector-detail__row"><span>组织</span><strong>{organization}</strong></div>
          </div>
        ) : (
          <div className="aw-cli-connector-detail__state">
            <span data-waiting={waiting ? "true" : "false"} />
            <span>{status}</span>
          </div>
        )}
      </div>
    </AwakenDialog>
  );
}
