import type { ExternalInteractionItem } from "./runtime/reducer";

type ExternalActionGateProps = {
  interaction: ExternalInteractionItem;
  compact?: boolean;
  resolving?: boolean;
  onConfirmDone: () => void;
  onCancel: () => void;
  onRetry?: () => void;
};

export function ExternalActionGate({
  interaction,
  compact = false,
  resolving = false,
  onConfirmDone,
  onCancel,
  onRetry,
}: ExternalActionGateProps) {
  const canCancel = interaction.recoveries.includes("cancel");
  const canConfirm = interaction.recoveries.includes("confirm_done");
  const canRetry =
    interaction.recoveries.includes("retry") ||
    interaction.recoveries.includes("refresh_qr");
  const title =
    interaction.title?.trim() ||
    (interaction.provider === "dingtalk-workspace"
      ? "钉钉授权"
      : interaction.provider === "lark-cli"
        ? "飞书授权"
        : "需要完成外部操作");

  return (
    <section
      className={`external-action-gate cli-auth-card${compact ? " cli-auth-card-compact" : ""}`}
      aria-live="polite"
      data-testid="external-action-gate"
    >
      <div className="cli-auth-card-copy">
        <strong>{title}</strong>
        <span>{interaction.safeMessage ?? "请完成外部操作后继续"}</span>
      </div>
      <div className="cli-auth-qr-frame">
        {interaction.qrImageUrl ? (
          <img src={interaction.qrImageUrl} alt="授权二维码" width={224} height={224} />
        ) : (
          <div className="cli-auth-qr-placeholder">
            {interaction.verificationUrl ? "使用下方链接完成授权" : "等待授权信息"}
          </div>
        )}
      </div>
      <div className="cli-auth-card-actions">
        {interaction.verificationUrl ? (
          <a href={interaction.verificationUrl} target="_blank" rel="noreferrer">
            打开授权链接
          </a>
        ) : null}
        {canConfirm ? (
          <button type="button" disabled={resolving} onClick={onConfirmDone}>
            我已完成
          </button>
        ) : null}
        {canRetry && onRetry ? (
          <button type="button" disabled={resolving} onClick={onRetry}>
            重新获取
          </button>
        ) : null}
        {canCancel ? (
          <button type="button" disabled={resolving} onClick={onCancel}>
            取消
          </button>
        ) : null}
      </div>
    </section>
  );
}
