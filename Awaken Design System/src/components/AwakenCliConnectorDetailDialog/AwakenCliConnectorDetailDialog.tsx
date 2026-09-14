import "./AwakenCliConnectorDetailDialog.css";
import { useEffect, useRef, useState } from "react";
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
  canUninstallManaged?: boolean;
  uninstallMessage?: string | null;
  onUninstall?: (removeOfficialSkills: boolean) => Promise<boolean>;
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
  canUninstallManaged = false,
  uninstallMessage,
  onUninstall,
  onOpenChange,
  onAction,
}: AwakenCliConnectorDetailDialogProps) {
  const [confirmUninstall, setConfirmUninstall] = useState(false);
  const [removeSkills, setRemoveSkills] = useState(false);
  const [uninstalling, setUninstalling] = useState(false);
  const uninstallRequest = useRef(false);
  const actionsRef = useRef<HTMLDivElement>(null);
  const wasConfirming = useRef(false);
  useEffect(() => {
    if (open && (confirmUninstall || wasConfirming.current)) {
      const target = confirmUninstall ? "button" : '[data-action="uninstall"], button';
      const actions = actionsRef.current;
      const button = !confirmUninstall ? actions?.querySelector<HTMLButtonElement>('[data-action="uninstall"]') : null;
      (button ?? actions?.querySelector<HTMLButtonElement>(target))?.focus();
    }
    wasConfirming.current = confirmUninstall;
  }, [confirmUninstall, open]);
  useEffect(() => { setConfirmUninstall(false); setRemoveSkills(false); }, [open, providerId]);
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

  if (confirmUninstall) return (
    <AwakenDialog
      open={open}
      className="aw-cli-connector-detail-dialog"
      title={`卸载${displayName}？`}
      onOpenChange={(next) => { if (!next && !uninstalling) setConfirmUninstall(false); }}
      footer={<div ref={actionsRef} className="aw-cli-connector-detail__uninstall-buttons">
        <AwakenButton disabled={uninstalling} onClick={() => setConfirmUninstall(false)}>取消</AwakenButton>
        <AwakenButton disabled={busy || uninstalling || !canUninstallManaged} onClick={async () => {
          if (!onUninstall || uninstallRequest.current) return;
          uninstallRequest.current = true;
          setUninstalling(true);
          try { if (await onUninstall(removeSkills)) setConfirmUninstall(false); }
          finally { uninstallRequest.current = false; setUninstalling(false); }
        }}>{uninstalling ? "正在卸载…" : "确认卸载"}</AwakenButton>
      </div>}
    >
      <div className="aw-cli-connector-detail__uninstall-copy">
        <p>卸载时会先退出当前账号，再移除 Awaken 管理的运行组件。退出登录失败时，不会删除组件和技能。</p>
        <label><input type="checkbox" checked={removeSkills} disabled={uninstalling} onChange={(event) => setRemoveSkills(event.target.checked)} />同时移除官方技能</label>
        <p>共享技能可能被其他产品使用。只移除可确认由 Awaken 安装且未被修改的技能；旧记录或归属不明的技能会保留。</p>
        <p>其他产品的安装文件不会删除；若共用同一登录信息，也会受退出登录影响。内置卡片保留，之后可重新安装并授权。</p>
        {errorMessage ? <p role="alert">{errorMessage}</p> : null}
      </div>
    </AwakenDialog>
  );

  return (
    <AwakenDialog
      open={open}
      className="aw-cli-connector-detail-dialog"
      title={title}
      titleVisuallyHidden
      onOpenChange={handleOpenChange}
      footer={
        <div ref={actionsRef} className="aw-cli-connector-detail__actions">
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
        {canUninstallManaged && onUninstall ? <AwakenButton
          variant="ghost"
          data-action="uninstall"
          disabled={busy || installing || waiting || readiness.authentication === "in_progress"}
          onClick={() => { setRemoveSkills(false); setConfirmUninstall(true); }}
        >卸载</AwakenButton> : null}
        </div>
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
        {uninstallMessage ? <p className="aw-cli-connector-detail__uninstall-copy" role="status">{uninstallMessage}</p> : null}

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
