import "./AwakenContextUsagePanel.css";
import { X } from "lucide-react";
import { forwardRef, useEffect, useId, useRef } from "react";

export type AwakenContextUsageContribution = {
  id: string;
  label: string;
  estimatedTokens: number;
};

const contributionColorIds = new Set([
  "system_prompt",
  "builtin_tools",
  "mcp",
  "skills",
  "memory",
  "conversation",
]);

function contributionColorId(id: string): string {
  return contributionColorIds.has(id) ? id : "unknown";
}

// AwakenContextUsageModelUsage mirrors the renderer-side AguiModelUsageRecord
// fields the panel needs. The Design System component must not import from
// renderer/shared, so the shape is re-declared here and the renderer is
// responsible for projecting only the documented fields.
export type AwakenContextUsageModelUsage = {
  status?: "succeeded" | "failed" | "cancelled" | "interrupted";
  cacheMode?: string;
  cacheKeyApplied?: boolean;
  cachedInputTokens?: number | null;
  uncachedInputTokens?: number | null;
  cacheWriteTokens?: number | null;
  inputTokens?: number | null;
  outputTokens?: number | null;
  reasoningTokens?: number | null;
  totalTokens?: number | null;
  calculatedCostUsd?: number | null;
  currency?: string;
  invalidationReason?: string;
  purpose?: string;
  errorCode?: string;
};

export type AwakenContextUsagePanelProps = {
  id?: string;
  estimatedInputTokens: number;
  effectiveInputWindowTokens: number;
  modelWindowTokens: number;
  outputReserveTokens: number;
  autoCompactThresholdTokens: number;
  contributions?: AwakenContextUsageContribution[];
  latestModelUsage?: AwakenContextUsageModelUsage | null;
  onClose: () => void;
};

export function formatContextUsageTokens(value: number | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "未提供";
  }
  const normalized = Math.max(0, value);
  if (normalized >= 1_000_000) {
    const amount = normalized / 1_000_000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}M`;
  }
  if (normalized >= 1_000) {
    const amount = normalized / 1_000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}K`;
  }
  return String(Math.round(normalized));
}

type CacheState = {
  label: string;
  tone: "hit" | "miss" | "unknown";
};

// resolveCacheState maps the provider cache mode and nullable cached tokens to
// a single user-facing label. `undocumented` (e.g. Volcano Coding Plan) is
// always "未确认" because the provider has not confirmed a cache contract;
// we never claim a hit or miss for it.
function resolveCacheState(
  usage: AwakenContextUsageModelUsage,
): CacheState | null {
  const mode = usage.cacheMode;
  if (!mode) {
    return null;
  }
  if (mode === "undocumented") {
    return { label: "未确认", tone: "unknown" };
  }
  if (usage.cachedInputTokens == null) {
    return { label: "未知", tone: "unknown" };
  }
  if (usage.cachedInputTokens > 0) {
    return { label: "已命中", tone: "hit" };
  }
  return { label: "未命中", tone: "miss" };
}

function formatCostLabel(
  usage: AwakenContextUsageModelUsage,
): string | undefined {
  // calculatedCostUsd === null/undefined means the provider has no price
  // contract (Kimi Coding Plan / Volcano / custom). Show "未知" rather than
  // fabricating a zero.
  if (usage.calculatedCostUsd == null) {
    return "未知";
  }
  const symbol = usage.currency === "USD" ? "$" : "";
  return `${symbol}${usage.calculatedCostUsd.toFixed(4)}`;
}

// hasLastRequestSignal returns true when the usage record carries at least one
// cache, cost or invalidation signal worth showing. A record with only
// status/purpose/inputTokens carries no cache or cost information and would
// just add noise, so it does not trigger the section.
function hasLastRequestSignal(
  usage: AwakenContextUsageModelUsage | null | undefined,
): usage is AwakenContextUsageModelUsage {
  if (!usage) {
    return false;
  }
  return (
    usage.cacheMode !== undefined ||
    usage.cachedInputTokens != null ||
    usage.uncachedInputTokens != null ||
    usage.calculatedCostUsd != null ||
    (usage.invalidationReason !== undefined && usage.invalidationReason !== "")
  );
}

export const AwakenContextUsagePanel = forwardRef<
  HTMLDivElement,
  AwakenContextUsagePanelProps
>(function AwakenContextUsagePanel(
  {
    id,
    estimatedInputTokens,
    effectiveInputWindowTokens,
    contributions = [],
    latestModelUsage,
    onClose,
  },
  forwardedRef,
) {
  const titleId = useId();
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const meterContributions = contributions.filter(
    (contribution) => contribution.estimatedTokens > 0,
  );
  const usedRatio =
    effectiveInputWindowTokens > 0
      ? estimatedInputTokens / effectiveInputWindowTokens
      : 0;
  const usedPercent = Math.max(0, Math.min(100, Math.round(usedRatio * 100)));

  useEffect(() => {
    closeButtonRef.current?.focus();
  }, []);

  const showLastRequest = hasLastRequestSignal(latestModelUsage);
  const cacheState = showLastRequest
    ? resolveCacheState(latestModelUsage)
    : null;
  const costLabel = showLastRequest
    ? formatCostLabel(latestModelUsage)
    : undefined;

  const usageSummary = `~${formatContextUsageTokens(estimatedInputTokens)} / ${formatContextUsageTokens(effectiveInputWindowTokens)} Token`;

  return (
    <div
      id={id}
      ref={forwardedRef}
      className="aw-context-usage-panel"
      role="dialog"
      aria-modal="false"
      aria-labelledby={titleId}
    >
      <header className="aw-context-usage-panel__header">
        <h2 id={titleId} className="aw-context-usage-panel__title">
          上下文用量
        </h2>
        <button
          ref={closeButtonRef}
          type="button"
          className="aw-context-usage-panel__close"
          aria-label="关闭上下文用量"
          onClick={onClose}
        >
          <X size={15} strokeWidth={1.9} aria-hidden="true" />
        </button>
      </header>

      <div className="aw-context-usage-panel__scroll">
        <section
          className="aw-context-usage-panel__summary"
          aria-label="当前上下文用量"
        >
          <div className="aw-context-usage-panel__summary-stats">
            <span className="aw-context-usage-panel__summary-percent">
              {usedPercent}% 已使用
            </span>
            <span className="aw-context-usage-panel__summary-tokens">
              {usageSummary}
            </span>
          </div>
          <div
            className="aw-context-usage-panel__meter"
            role="meter"
            aria-label="可用输入窗口使用比例"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={usedPercent}
          >
            {meterContributions.length > 0 ? (
              meterContributions.map((contribution) => (
                <span
                  key={contribution.id}
                  className={`aw-context-usage-panel__segment aw-context-usage-panel__segment--${contributionColorId(contribution.id)}`}
                  style={{
                    width: `${Math.max(
                      0,
                      (contribution.estimatedTokens /
                        Math.max(1, effectiveInputWindowTokens)) *
                        100,
                    )}%`,
                  }}
                  aria-hidden="true"
                />
              ))
            ) : (
              <span
                className="aw-context-usage-panel__segment aw-context-usage-panel__segment--unknown"
                style={{ width: `${usedPercent}%` }}
                aria-hidden="true"
              />
            )}
          </div>
        </section>

        <section
          className="aw-context-usage-panel__breakdown"
          aria-label="上下文分类明细"
        >
          {contributions.length > 0 ? (
            <dl>
              {contributions.map((contribution) => (
                <div
                  key={contribution.id}
                  className="aw-context-usage-panel__row"
                >
                  <dt>
                    <span
                      className={`aw-context-usage-panel__dot aw-context-usage-panel__dot--${contributionColorId(contribution.id)}`}
                      aria-hidden="true"
                    />
                    {contribution.label}
                  </dt>
                  <dd>
                    {formatContextUsageTokens(contribution.estimatedTokens)}
                  </dd>
                </div>
              ))}
            </dl>
          ) : (
            <p className="aw-context-usage-panel__empty">暂无分类明细</p>
          )}
        </section>

        {showLastRequest ? (
          <section
            className="aw-context-usage-panel__last-request"
            aria-label="上次请求缓存与成本"
          >
            <h3 className="aw-context-usage-panel__subtitle">上次请求</h3>
            <dl>
              {cacheState ? (
                <div className="aw-context-usage-panel__row">
                  <dt>缓存</dt>
                  <dd>
                    <span
                      className={`aw-context-usage-panel__badge aw-context-usage-panel__badge--${cacheState.tone}`}
                    >
                      {cacheState.label}
                    </span>
                  </dd>
                </div>
              ) : null}
              {latestModelUsage.cachedInputTokens != null ? (
                <div className="aw-context-usage-panel__row">
                  <dt>命中 Token</dt>
                  <dd>
                    {formatContextUsageTokens(
                      latestModelUsage.cachedInputTokens,
                    )}
                  </dd>
                </div>
              ) : null}
              {latestModelUsage.uncachedInputTokens != null ? (
                <div className="aw-context-usage-panel__row">
                  <dt>未命中 Token</dt>
                  <dd>
                    {formatContextUsageTokens(
                      latestModelUsage.uncachedInputTokens,
                    )}
                  </dd>
                </div>
              ) : null}
              {latestModelUsage.outputTokens != null ? (
                <div className="aw-context-usage-panel__row">
                  <dt>输出 Token</dt>
                  <dd>
                    {formatContextUsageTokens(latestModelUsage.outputTokens)}
                  </dd>
                </div>
              ) : null}
              {costLabel !== undefined ? (
                <div className="aw-context-usage-panel__row">
                  <dt>成本</dt>
                  <dd>{costLabel}</dd>
                </div>
              ) : null}
              {latestModelUsage.invalidationReason ? (
                <div className="aw-context-usage-panel__row">
                  <dt>失效原因</dt>
                  <dd>{latestModelUsage.invalidationReason}</dd>
                </div>
              ) : null}
            </dl>
          </section>
        ) : null}

      </div>
    </div>
  );
});
