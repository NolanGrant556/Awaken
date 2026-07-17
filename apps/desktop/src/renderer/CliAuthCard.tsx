import type { CliAuthSessionView } from "../shared/runtime";

type CliAuthCardProps = {
  session: CliAuthSessionView;
  compact?: boolean;
  onCancel: () => void;
  onRetry: () => void;
};

export function CliAuthCard({ session, compact = false, onCancel, onRetry }: CliAuthCardProps) {
  const canCancel = session.recoveries.includes("cancel");
  const canRetry = session.recoveries.includes("retry") || session.recoveries.includes("refresh_qr");
  return (
    <section className={`cli-auth-card${compact ? " cli-auth-card-compact" : ""}`} aria-live="polite">
      <div className="cli-auth-card-copy">
        <strong>{session.provider === "dingtalk-workspace" ? "钉钉授权" : "飞书授权"}</strong>
        <span>{session.safeMessage ?? authPhaseLabel(session.phase)}</span>
      </div>
      <div className="cli-auth-qr-frame">
        {session.qrImageUrl ? (
          <img src={session.qrImageUrl} alt="授权二维码" width={224} height={224} />
        ) : (
          <div className="cli-auth-qr-placeholder">{session.phase === "failed" ? "二维码不可用" : "等待授权页面"}</div>
        )}
      </div>
      <div className="cli-auth-card-actions">
        {session.verificationUrl ? (
          <a href={session.verificationUrl} target="_blank" rel="noreferrer">打开授权链接</a>
        ) : null}
        {canRetry ? <button type="button" onClick={onRetry}>重新获取</button> : null}
        {canCancel ? <button type="button" onClick={onCancel}>取消</button> : null}
      </div>
    </section>
  );
}

function authPhaseLabel(phase: CliAuthSessionView["phase"]): string {
  switch (phase) {
    case "starting": return "正在准备授权…";
    case "waiting_user": return "等待扫码或浏览器确认";
    case "confirming": return "正在确认授权…";
    case "authorized": return "授权成功";
    case "expired": return "授权已过期";
    case "cancelled": return "授权已取消";
    case "failed": return "授权失败";
  }
}
