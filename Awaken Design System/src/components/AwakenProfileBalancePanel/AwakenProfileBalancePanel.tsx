import "./AwakenProfileBalancePanel.css";
import { useState, type AriaRole, type CSSProperties, type ReactNode } from "react";
import { AlertCircle, ChevronRight, Loader2, Settings, Wallet } from "lucide-react";
import { AwakenButton } from "../AwakenButton";

export type AwakenProfileBalanceStatus =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported";

export type AwakenProfileBalanceAmount = {
  currency: string;
  total: number;
  granted?: number;
  toppedUp?: number;
};

export type AwakenProfileBalanceQuota = {
  name: string;
  limit: number;
  used: number;
  remaining: number;
  resetAt?: string;
};

export type AwakenProfileBalanceDetail =
  | { kind: "amount"; amounts: AwakenProfileBalanceAmount[] }
  | { kind: "quota"; quotas: AwakenProfileBalanceQuota[] };

export type AwakenProfileBalance = {
  profileId: string;
  providerId: string;
  modelId: string;
  displayName?: string;
  status: AwakenProfileBalanceStatus;
  fetchedAt?: string;
  detail?: AwakenProfileBalanceDetail;
  error?: string;
  credentialIssue?: "unreadable";
};

export type AwakenProfileBalancePanelProps = {
  id?: string;
  balances?: AwakenProfileBalance[];
  loading?: boolean;
  error?: string | null;
  selectedProfileId?: string | null;
  defaultSelectedProfileId?: string | null;
  settingsLabel?: ReactNode;
  ariaLabel?: string;
  role?: AriaRole;
  className?: string;
  style?: CSSProperties;
  onSelectedProfileIdChange?: (profileId: string) => void;
  onOpenSettings?: () => void;
};

function formatCurrencyAmount(amount: AwakenProfileBalanceAmount): string {
  const { currency, total } = amount;
  const symbol = currency === "USD" ? "$" : currency === "CNY" ? "¥" : "";
  return `${symbol}${total.toFixed(2)} ${symbol ? "" : currency}`.trim();
}

function formatUsedPercentage(used: number, limit: number): string {
  if (limit <= 0) return "0%";
  return `${Math.round((used / limit) * 100)}%`;
}

function formatBalanceSummary(balance: AwakenProfileBalance): string {
  if (balance.credentialIssue === "unreadable") return "需要重新输入 API key";
  if (balance.status === "missing_key") return "未配置 key";
  if (balance.status === "unsupported") return "暂不支持";
  if (balance.status === "error") return balance.error || "查询失败";
  if (!balance.detail) return "—";
  if (balance.detail.kind === "amount") {
    const primary = balance.detail.amounts[0];
    return primary ? formatCurrencyAmount(primary) : "—";
  }
  const primary = balance.detail.quotas[0];
  return primary ? formatUsedPercentage(primary.used, primary.limit) : "—";
}

function formatResetTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const diffMs = date.getTime() - Date.now();
  if (diffMs <= 0) return "即将重置";
  const hours = Math.ceil(diffMs / (1000 * 60 * 60));
  return hours < 24 ? `${hours} 小时后` : `${Math.ceil(hours / 24)} 天后`;
}

export function getAwakenProfileBalanceCredentialRecovery(
  balances: AwakenProfileBalance[],
): "unreadable" | null {
  return balances.some((balance) => balance.credentialIssue === "unreadable")
    ? "unreadable"
    : null;
}

function BalanceAmountView({ amounts }: { amounts: AwakenProfileBalanceAmount[] }) {
  const primary = amounts[0];
  if (!primary) return <span className="aw-profile-balance-panel__empty-value">—</span>;
  return (
    <div className="aw-profile-balance-panel__amount">
      <div className="aw-profile-balance-panel__amount-primary">
        {formatCurrencyAmount(primary)}
      </div>
      {amounts.length > 1 ? (
        <div className="aw-profile-balance-panel__amount-secondary">
          {amounts.map((amount) => (
            <span key={amount.currency}>
              {amount.currency}: {formatCurrencyAmount(amount)}
            </span>
          ))}
        </div>
      ) : (
        <div className="aw-profile-balance-panel__amount-breakdown">
          {typeof primary.granted === "number" && primary.granted > 0 ? (
            <span>赠送 {primary.granted.toFixed(2)}</span>
          ) : null}
          {typeof primary.toppedUp === "number" && primary.toppedUp > 0 ? (
            <span>余额 {primary.toppedUp.toFixed(2)}</span>
          ) : null}
        </div>
      )}
    </div>
  );
}

function QuotaView({ quota }: { quota: AwakenProfileBalanceQuota }) {
  const progress = quota.limit > 0 ? quota.used / quota.limit : 0;
  const percentage = Math.min(100, Math.max(0, progress * 100));
  return (
    <div className="aw-profile-balance-panel__quota">
      <div className="aw-profile-balance-panel__quota-header">
        <span className="aw-profile-balance-panel__quota-name">{quota.name}</span>
        <span className="aw-profile-balance-panel__quota-value">
          {formatUsedPercentage(quota.used, quota.limit)}
        </span>
      </div>
      <div className="aw-profile-balance-panel__quota-bar">
        <div
          className="aw-profile-balance-panel__quota-fill"
          style={{ width: `${percentage}%` }}
        />
      </div>
      <div className="aw-profile-balance-panel__quota-meta">
        已用 {quota.used} · 剩余 {quota.remaining}
        {quota.resetAt ? <span> · 重置 {formatResetTime(quota.resetAt)}</span> : null}
      </div>
    </div>
  );
}

export function AwakenProfileBalancePanel({
  id,
  balances = [],
  loading = false,
  error = null,
  selectedProfileId,
  defaultSelectedProfileId,
  settingsLabel = "设置",
  ariaLabel = "模型余额",
  role = "dialog",
  className,
  style,
  onSelectedProfileIdChange,
  onOpenSettings,
}: AwakenProfileBalancePanelProps) {
  const [internalSelectedProfileId, setInternalSelectedProfileId] = useState<string | null>(
    defaultSelectedProfileId ?? balances[0]?.profileId ?? null,
  );
  const currentSelectedProfileId =
    selectedProfileId !== undefined
      ? selectedProfileId
      : internalSelectedProfileId ?? balances[0]?.profileId ?? null;
  const selectedBalance = balances.find(
    (balance) => balance.profileId === currentSelectedProfileId,
  );
  const credentialRecovery = getAwakenProfileBalanceCredentialRecovery(balances);

  function selectProfile(profileId: string): void {
    if (selectedProfileId === undefined) setInternalSelectedProfileId(profileId);
    onSelectedProfileIdChange?.(profileId);
  }

  return (
    <div
      id={id}
      className={["aw-profile-balance-panel", className ?? ""].filter(Boolean).join(" ")}
      style={style}
      role={role}
      aria-label={ariaLabel}
    >
      <div className="aw-profile-balance-panel__body">
        {loading ? (
          <div className="aw-profile-balance-panel__message">
            <Loader2
              size={18}
              strokeWidth={2}
              className="aw-profile-balance-panel__spin"
              aria-hidden="true"
            />
            <span>正在查询余额…</span>
          </div>
        ) : error ? (
          <div className="aw-profile-balance-panel__message aw-profile-balance-panel__message--stacked">
            <AlertCircle size={18} strokeWidth={2} aria-hidden="true" />
            <span>{error}</span>
          </div>
        ) : balances.length === 0 ? (
          <div className="aw-profile-balance-panel__message aw-profile-balance-panel__message--stacked">
            <Wallet size={18} strokeWidth={2} aria-hidden="true" />
            <span>没有已启用且有 API key 的模型</span>
          </div>
        ) : (
          <>
            <div className="aw-profile-balance-panel__section-title">剩余用量</div>
            {credentialRecovery ? (
              <div className="aw-profile-balance-panel__recovery" role="alert">
                <div className="aw-profile-balance-panel__recovery-copy">
                  <AlertCircle size={18} strokeWidth={2} aria-hidden="true" />
                  <span>无法读取 ~/.awaken/.env 中的模型密钥，请重新输入。</span>
                </div>
                <AwakenButton variant="primary" size="sm" onClick={onOpenSettings}>
                  在 Awaken 中恢复
                </AwakenButton>
              </div>
            ) : null}
            <div className="aw-profile-balance-panel__list">
              {balances.map((balance) => (
                <button
                  key={balance.profileId}
                  type="button"
                  className={[
                    "aw-profile-balance-panel__item",
                    balance.profileId === currentSelectedProfileId
                      ? "aw-profile-balance-panel__item--active"
                      : "",
                  ].filter(Boolean).join(" ")}
                  aria-pressed={balance.profileId === currentSelectedProfileId}
                  onClick={() => selectProfile(balance.profileId)}
                >
                  <span className="aw-profile-balance-panel__item-icon" aria-hidden="true">
                    {balance.displayName?.slice(0, 1) || balance.providerId.slice(0, 1).toUpperCase()}
                  </span>
                  <span className="aw-profile-balance-panel__item-info">
                    <span className="aw-profile-balance-panel__item-name">
                      {balance.displayName || balance.modelId}
                    </span>
                    <span className="aw-profile-balance-panel__item-summary">
                      {formatBalanceSummary(balance)}
                    </span>
                  </span>
                  <ChevronRight size={14} strokeWidth={2} aria-hidden="true" />
                </button>
              ))}
            </div>

            {selectedBalance ? (
              <div className="aw-profile-balance-panel__detail">
                {selectedBalance.status === "available" && selectedBalance.detail ? (
                  selectedBalance.detail.kind === "amount" ? (
                    <BalanceAmountView amounts={selectedBalance.detail.amounts} />
                  ) : (
                    <div className="aw-profile-balance-panel__quotas">
                      {selectedBalance.detail.quotas.map((quota) => (
                        <QuotaView key={quota.name} quota={quota} />
                      ))}
                    </div>
                  )
                ) : (
                  <div className="aw-profile-balance-panel__detail-empty">
                    <AlertCircle size={18} strokeWidth={2} aria-hidden="true" />
                    <span>
                      {selectedBalance.status === "missing_key"
                        ? "请先配置 API key"
                        : selectedBalance.credentialIssue === "unreadable"
                          ? "请在 Awaken 中重新输入 API key"
                          : selectedBalance.status === "unsupported"
                            ? "暂不支持查询该供应商余额"
                            : selectedBalance.error || "余额查询失败"}
                    </span>
                  </div>
                )}
              </div>
            ) : null}
          </>
        )}
      </div>

      <div className="aw-profile-balance-panel__footer">
        <AwakenButton
          variant="default"
          size="md"
          className="aw-profile-balance-panel__settings"
          onClick={onOpenSettings}
        >
          <Settings size={15} strokeWidth={1.9} aria-hidden="true" />
          {settingsLabel}
        </AwakenButton>
      </div>
    </div>
  );
}
