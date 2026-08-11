import "./AwakenProfileBalancePanel.css";
import { useState, type CSSProperties } from "react";
import { AlertCircle, Loader2, Wallet } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import {
  AwakenProviderLogo,
  type AwakenProviderBrand,
} from "../AwakenProviderLogo";

export type AwakenProfileBalanceStatus =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported"
  | "requires_setup";

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
  unit?: "requests" | "percent";
  unlimited?: boolean;
  resetAt?: string;
};

export type AwakenProfileBalanceDetail =
  | { kind: "amount"; amounts: AwakenProfileBalanceAmount[] }
  | { kind: "quota"; quotas: AwakenProfileBalanceQuota[] }
  | { kind: "usage"; metrics: Array<{ name: string; value: number; unit: "tokens" | "requests" | "usd" }>; consoleURL?: string };

export type AwakenProfileBalance = {
  profileId: string;
  providerId: string;
  providerBrand?: AwakenProviderBrand;
  modelId: string;
  displayName?: string;
  status: AwakenProfileBalanceStatus;
  fetchedAt?: string;
  detail?: AwakenProfileBalanceDetail;
  error?: string;
  credentialIssue?: "unreadable";
  setupURL?: string;
  setupLabel?: string;
};

export type AwakenProfileBalancePanelProps = {
  id?: string;
  balances?: AwakenProfileBalance[];
  loading?: boolean;
  error?: string | null;
  selectedProfileId?: string | null;
  defaultSelectedProfileId?: string | null;
  ariaLabel?: string;
  className?: string;
  style?: CSSProperties;
  onSelectedProfileIdChange?: (profileId: string) => void;
  onOpenSettings?: () => void;
};

function formatCurrencyAmount(amount: AwakenProfileBalanceAmount): string {
  return formatCurrencyValue(amount.currency, amount.total);
}

function formatCurrencyValue(currency: string, value: number): string {
  const symbol = currency === "USD" ? "$" : currency === "CNY" ? "¥" : "";
  return `${symbol}${value.toFixed(2)} ${symbol ? "" : currency}`.trim();
}

function formatUsedPercentage(used: number, limit: number): string {
  if (limit <= 0) return "0%";
  return `${Math.round((used / limit) * 100)}%`;
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
  const breakdown = amounts.length > 1
    ? amounts.slice(1).map((amount) => ({
        label: amount.currency,
        value: formatCurrencyAmount(amount),
      }))
    : [
        typeof primary.granted === "number" && primary.granted > 0
          ? {
              label: "赠送额度",
              value: formatCurrencyValue(primary.currency, primary.granted),
            }
          : null,
        typeof primary.toppedUp === "number" && primary.toppedUp > 0
          ? {
              label: "充值余额",
              value: formatCurrencyValue(primary.currency, primary.toppedUp),
            }
          : null,
      ].filter((item): item is { label: string; value: string } => item !== null);

  return (
    <div className="aw-profile-balance-panel__amount">
      <span className="aw-profile-balance-panel__summary-label">可用余额</span>
      <div className="aw-profile-balance-panel__amount-primary">
        {formatCurrencyAmount(primary)}
      </div>
      {breakdown.length > 0 ? (
        <div className="aw-profile-balance-panel__amount-breakdown">
          {breakdown.map((item) => (
            <div className="aw-profile-balance-panel__amount-metric" key={item.label}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function QuotaView({ quota }: { quota: AwakenProfileBalanceQuota }) {
  const progress = !quota.unlimited && quota.limit > 0 ? quota.used / quota.limit : 0;
  const percentage = Math.min(100, Math.max(0, progress * 100));
  return (
    <div className="aw-profile-balance-panel__quota">
      <div className="aw-profile-balance-panel__quota-header">
        <span className="aw-profile-balance-panel__quota-name">{quota.name}</span>
        <span className="aw-profile-balance-panel__quota-value">
          {quota.unlimited ? "∞" : formatUsedPercentage(quota.used, quota.limit)}
        </span>
      </div>
      <div className="aw-profile-balance-panel__quota-bar">
        <div
          className="aw-profile-balance-panel__quota-fill"
          style={{ width: `${percentage}%` }}
        />
      </div>
      <div className="aw-profile-balance-panel__quota-meta">
        {quota.unlimited ? (
          "无限制"
        ) : (
          <>
            已用 {quota.used}{quota.unit === "percent" ? "%" : ""} · 剩余 {quota.remaining}{quota.unit === "percent" ? "%" : ""}
            {quota.resetAt ? <span> · 重置 {formatResetTime(quota.resetAt)}</span> : null}
          </>
        )}
      </div>
    </div>
  );
}

function UsageView({
  detail,
}: {
  detail: Extract<AwakenProfileBalanceDetail, { kind: "usage" }>;
}) {
  return (
    <div className="aw-profile-balance-panel__amount-breakdown">
      {detail.metrics.map((metric) => (
        <div className="aw-profile-balance-panel__amount-metric" key={`${metric.name}-${metric.unit}`}>
          <span>{metric.name}</span>
          <strong>{metric.unit === "usd" ? `$${metric.value.toFixed(2)}` : metric.value.toLocaleString()}</strong>
        </div>
      ))}
      {detail.consoleURL ? <a href={detail.consoleURL} target="_blank" rel="noreferrer">打开供应商控制台</a> : null}
    </div>
  );
}

function BalanceDetail({
  balance,
  onOpenSettings,
}: {
  balance: AwakenProfileBalance;
  onOpenSettings?: () => void;
}) {
  const officialDetail = balance.status === "available" && balance.detail
    ? balance.detail.kind === "amount" ? (
      <BalanceAmountView amounts={balance.detail.amounts} />
    ) : balance.detail.kind === "usage" ? (
      <UsageView detail={balance.detail} />
    ) : (
      <div className="aw-profile-balance-panel__quotas">
        {balance.detail.quotas.map((quota) => (
          <QuotaView key={quota.name} quota={quota} />
        ))}
      </div>
    )
    : null;

  const unreadable = balance.credentialIssue === "unreadable";
  const message =
    balance.status === "missing_key"
      ? "请先配置 API key"
      : unreadable
        ? "无法读取模型密钥，请在设置中重新输入。"
        : balance.status === "requires_setup"
          ? balance.error || "需要配置独立的供应商用量管理授权。"
          : balance.status === "unsupported"
            ? balance.error || "当前供应商暂不支持查询用量"
          : balance.error || "用量查询失败";

  return (
    <div className="aw-profile-balance-panel__detail-sections">
      {officialDetail ?? (
        <div className="aw-profile-balance-panel__detail-empty" role={unreadable ? "alert" : "status"}>
          <AlertCircle size={20} strokeWidth={1.9} aria-hidden="true" />
          <span>{message}</span>
          {unreadable ? (
            <AwakenButton variant="default" size="sm" onClick={onOpenSettings}>
              前往设置
            </AwakenButton>
          ) : null}
          {balance.setupURL ? (
            <a
              className="aw-profile-balance-panel__setup-link"
              href={balance.setupURL}
              target="_blank"
              rel="noreferrer"
            >
              {balance.setupLabel || "打开供应商说明"}
            </a>
          ) : null}
        </div>
      )}
    </div>
  );
}

function BalanceProviderMark({
  balance,
  detail = false,
}: {
  balance: AwakenProfileBalance;
  detail?: boolean;
}) {
  const name = balance.displayName || balance.modelId || balance.providerId;
  const className = [
    "aw-profile-balance-panel__provider-mark",
    detail ? "aw-profile-balance-panel__provider-mark--detail" : "",
  ].filter(Boolean).join(" ");

  return balance.providerBrand ? (
    <span className={className} aria-hidden="true">
      <AwakenProviderLogo brand={balance.providerBrand} name={name} />
    </span>
  ) : (
    <span
      className={`${className} aw-profile-balance-panel__provider-mark--fallback`}
      aria-hidden="true"
    >
      {name.trim().slice(0, 1).toUpperCase() || "M"}
    </span>
  );
}

export function AwakenProfileBalancePanel({
  id,
  balances = [],
  loading = false,
  error = null,
  selectedProfileId,
  defaultSelectedProfileId,
  ariaLabel = "模型用量",
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
  ) ?? balances[0];

  function selectProfile(profileId: string): void {
    if (selectedProfileId === undefined) setInternalSelectedProfileId(profileId);
    onSelectedProfileIdChange?.(profileId);
  }

  return (
    <div
      id={id}
      className={["aw-profile-balance-panel", className ?? ""].filter(Boolean).join(" ")}
      style={style}
      role="region"
      aria-label={ariaLabel}
    >
      {loading ? (
        <div className="aw-profile-balance-panel__message">
          <Loader2
            size={20}
            strokeWidth={1.9}
            className="aw-profile-balance-panel__spin"
            aria-hidden="true"
          />
          <span>正在查询用量…</span>
        </div>
      ) : error ? (
        <div className="aw-profile-balance-panel__message" role="alert">
          <AlertCircle size={20} strokeWidth={1.9} aria-hidden="true" />
          <span>{error}</span>
        </div>
      ) : balances.length === 0 ? (
        <div className="aw-profile-balance-panel__message">
          <Wallet size={20} strokeWidth={1.9} aria-hidden="true" />
          <span>没有已启用的模型 API</span>
        </div>
      ) : (
        <>
          <aside className="aw-profile-balance-panel__nav" aria-label="已配置 API">
            <div className="aw-profile-balance-panel__nav-title">API / Plan</div>
            <div className="aw-profile-balance-panel__list" role="tablist" aria-orientation="vertical">
              {balances.map((balance) => {
                const active = balance.profileId === selectedBalance?.profileId;
                return (
                  <button
                    key={balance.profileId}
                    type="button"
                    className={[
                      "aw-profile-balance-panel__item",
                      active ? "aw-profile-balance-panel__item--active" : "",
                    ].filter(Boolean).join(" ")}
                    role="tab"
                    aria-selected={active}
                    onClick={() => selectProfile(balance.profileId)}
                  >
                    <BalanceProviderMark balance={balance} />
                    <span className="aw-profile-balance-panel__item-info">
                      <span className="aw-profile-balance-panel__item-name">
                        {balance.displayName || balance.modelId}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          </aside>
          <section
            className="aw-profile-balance-panel__detail"
            role="tabpanel"
            aria-label={selectedBalance?.displayName || selectedBalance?.modelId}
          >
            {selectedBalance ? (
              <>
                <header className="aw-profile-balance-panel__detail-head">
                  <div className="aw-profile-balance-panel__detail-identity">
                    <BalanceProviderMark balance={selectedBalance} detail />
                    <div className="aw-profile-balance-panel__detail-copy">
                      <h3>{selectedBalance.displayName || selectedBalance.modelId}</h3>
                    </div>
                  </div>
                </header>
                <div className="aw-profile-balance-panel__detail-body">
                  <BalanceDetail balance={selectedBalance} onOpenSettings={onOpenSettings} />
                </div>
              </>
            ) : null}
          </section>
        </>
      )}
    </div>
  );
}
