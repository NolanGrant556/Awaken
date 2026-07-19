import "./AwakenContextUsagePanel.css";
import { X } from "lucide-react";
import { forwardRef, useEffect, useId, useRef } from "react";

export type AwakenContextUsageContribution = {
  id: string;
  label: string;
  estimatedTokens: number;
};

export type AwakenContextUsagePanelProps = {
  id?: string;
  estimatedInputTokens: number;
  effectiveInputWindowTokens: number;
  modelWindowTokens: number;
  outputReserveTokens: number;
  autoCompactThresholdTokens: number;
  contributions?: AwakenContextUsageContribution[];
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

export const AwakenContextUsagePanel = forwardRef<
  HTMLDivElement,
  AwakenContextUsagePanelProps
>(function AwakenContextUsagePanel(
  {
    id,
    estimatedInputTokens,
    effectiveInputWindowTokens,
    contributions = [],
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
          <div className="aw-context-usage-panel__summary-copy">
            <h2 id={titleId} className="aw-context-usage-panel__title">
              上下文用量
            </h2>
            <span>
              {formatContextUsageTokens(estimatedInputTokens)} /{" "}
              {formatContextUsageTokens(effectiveInputWindowTokens)} Token
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
              meterContributions.map((contribution, index) => (
                <span
                  key={contribution.id}
                  className={`aw-context-usage-panel__segment aw-context-usage-panel__segment--${index % 6}`}
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
                className="aw-context-usage-panel__segment aw-context-usage-panel__segment--0"
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
              {contributions.map((contribution, index) => (
                <div
                  key={contribution.id}
                  className="aw-context-usage-panel__row"
                >
                  <dt>
                    <span
                      className={`aw-context-usage-panel__dot aw-context-usage-panel__dot--${index % 6}`}
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

      </div>
    </div>
  );
});
