import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  AlertCircle,
  ChevronRight,
  Loader2,
  Settings,
  Wallet,
} from "lucide-react";
import { AwakenButton as Button } from "@awaken/design-system";
import type {
  ModelBalanceAmount,
  ModelBalanceQuota,
  ModelProfileBalance,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";

type ProfileBalancePopoverProps = {
  open: boolean;
  anchorRef: React.RefObject<HTMLElement | null>;
  runtimeClient: RuntimeClient;
  onOpenSettings: () => void;
  onClose: () => void;
};

function formatCurrencyAmount(amount: ModelBalanceAmount): string {
  const { currency, total } = amount;
  const symbol = currency === "USD" ? "$" : currency === "CNY" ? "¥" : "";
  return `${symbol}${total.toFixed(2)} ${symbol ? "" : currency}`.trim();
}

function formatUsedPercentage(used: number, limit: number): string {
  if (limit <= 0) return "0%";
  return `${Math.round((used / limit) * 100)}%`;
}

function formatBalanceSummary(balance: ModelProfileBalance): string {
  if (balance.status === "missing_key") return "未配置 key";
  if (balance.status === "unsupported") return "暂不支持";
  if (balance.status === "error") return balance.error || "查询失败";
  if (!balance.detail) return "—";
  if (balance.detail.kind === "amount") {
    const primary = balance.detail.amounts[0];
    return primary ? formatCurrencyAmount(primary) : "—";
  }
  const weekly = balance.detail.quotas[0];
  return weekly ? formatUsedPercentage(weekly.used, weekly.limit) : "—";
}

function BalanceAmountView({ amounts }: { amounts: ModelBalanceAmount[] }) {
  const primary = amounts[0];
  if (!primary) return <span className="balance-empty">—</span>;
  return (
    <div className="balance-amount-view">
      <div className="balance-amount-primary">
        {formatCurrencyAmount(primary)}
      </div>
      {amounts.length > 1 ? (
        <div className="balance-amount-secondary">
          {amounts.map((amount) => (
            <span key={amount.currency}>
              {amount.currency}: {formatCurrencyAmount(amount)}
            </span>
          ))}
        </div>
      ) : (
        <div className="balance-amount-breakdown">
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

function QuotaBar({ quota }: { quota: ModelBalanceQuota }) {
  const progress = quota.limit > 0 ? quota.used / quota.limit : 0;
  const percentage = Math.min(100, Math.max(0, progress * 100));
  return (
    <div className="balance-quota">
      <div className="balance-quota-header">
        <span className="balance-quota-name">{quota.name}</span>
        <span className="balance-quota-value">
          {formatUsedPercentage(quota.used, quota.limit)}
        </span>
      </div>
      <div className="balance-quota-bar">
        <div
          className="balance-quota-fill"
          style={{ width: `${percentage}%` }}
        />
      </div>
      <div className="balance-quota-meta">
        已用 {quota.used} · 剩余 {quota.remaining}
        {quota.resetAt ? (
          <span> · 重置 {formatResetTime(quota.resetAt)}</span>
        ) : null}
      </div>
    </div>
  );
}

function formatResetTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const now = Date.now();
  const diffMs = date.getTime() - now;
  if (diffMs <= 0) return "即将重置";
  const hours = Math.ceil(diffMs / (1000 * 60 * 60));
  if (hours < 24) return `${hours} 小时后`;
  const days = Math.ceil(hours / 24);
  return `${days} 天后`;
}

export function ProfileBalancePopover({
  open,
  anchorRef,
  runtimeClient,
  onOpenSettings,
  onClose,
}: ProfileBalancePopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const [balances, setBalances] = useState<ModelProfileBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedProfileId, setSelectedProfileId] = useState<string | null>(
    null,
  );
  const [position, setPosition] = useState<{ left: number; bottom: number }>({
    left: 0,
    bottom: 0,
  });

  const selectedBalance = balances.find(
    (balance) => balance.profileId === selectedProfileId,
  );

  useLayoutEffect(() => {
    if (!open || !anchorRef.current) return;
    const anchor = anchorRef.current;
    const rect = anchor.getBoundingClientRect();
    const gap = 8;
    setPosition({
      left: rect.left,
      bottom: window.innerHeight - rect.top + gap,
    });
  }, [open, anchorRef]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    setBalances([]);
    setSelectedProfileId(null);

    async function load(): Promise<void> {
      try {
        const result = await runtimeClient.getModelProfileBalances();
        if (cancelled) return;
        setBalances(result.balances);
        setSelectedProfileId(
          result.balances[0]?.profileId ?? null,
        );
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "查询余额失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [open, runtimeClient]);

  useEffect(() => {
    if (!open) return;
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        onClose();
      }
    }
    function handlePointerDown(event: MouseEvent): void {
      const target = event.target as Node;
      if (
        popoverRef.current &&
        !popoverRef.current.contains(target) &&
        !anchorRef.current?.contains(target)
      ) {
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("pointerdown", handlePointerDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [open, onClose, anchorRef]);

  const handleOpenSettings = useCallback(() => {
    onClose();
    onOpenSettings();
  }, [onClose, onOpenSettings]);

  if (!open) return null;

  return (
    <div
      ref={popoverRef}
      className="profile-balance-popover"
      style={{
        left: position.left,
        bottom: position.bottom,
      }}
      role="dialog"
      aria-label="模型余额"
    >
      <div className="profile-balance-body">
        {loading ? (
          <div className="profile-balance-loading">
            <Loader2 size={18} strokeWidth={2} className="balance-spin" />
            <span>正在查询余额…</span>
          </div>
        ) : error ? (
          <div className="profile-balance-empty">
            <AlertCircle size={18} strokeWidth={2} />
            <span>{error}</span>
          </div>
        ) : balances.length === 0 ? (
          <div className="profile-balance-empty">
            <Wallet size={18} strokeWidth={2} />
            <span>没有已启用且有 API key 的模型</span>
          </div>
        ) : (
          <>
            <div className="profile-balance-section-title">剩余用量</div>
            <div className="profile-balance-list">
              {balances.map((balance) => (
                <button
                  key={balance.profileId}
                  type="button"
                  className={`profile-balance-item${
                    balance.profileId === selectedProfileId
                      ? " profile-balance-item-active"
                      : ""
                  }`}
                  onClick={() => setSelectedProfileId(balance.profileId)}
                >
                  <span className="profile-balance-item-icon">
                    {balance.displayName?.slice(0, 1) ||
                      balance.providerId.slice(0, 1).toUpperCase()}
                  </span>
                  <span className="profile-balance-item-info">
                    <span className="profile-balance-item-name">
                      {balance.displayName || balance.modelId}
                    </span>
                    <span className="profile-balance-item-summary">
                      {formatBalanceSummary(balance)}
                    </span>
                  </span>
                  <ChevronRight size={14} strokeWidth={2} />
                </button>
              ))}
            </div>

            {selectedBalance ? (
              <div className="profile-balance-detail">
                {selectedBalance.status === "available" &&
                selectedBalance.detail ? (
                  selectedBalance.detail.kind === "amount" ? (
                    <BalanceAmountView amounts={selectedBalance.detail.amounts} />
                  ) : (
                    <div className="balance-quotas">
                      {selectedBalance.detail.quotas.map((quota) => (
                        <QuotaBar key={quota.name} quota={quota} />
                      ))}
                    </div>
                  )
                ) : (
                  <div className="profile-balance-detail-empty">
                    <AlertCircle size={18} strokeWidth={2} />
                    <span>
                      {selectedBalance.status === "missing_key"
                        ? "请先配置 API key"
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

      <div className="profile-balance-footer">
        <Button
          variant="default"
          size="md"
          className="profile-balance-settings-btn"
          onClick={handleOpenSettings}
        >
          <Settings size={15} strokeWidth={1.9} />
          设置
        </Button>
      </div>
    </div>
  );
}
