import type { ComputerReviewItem } from "./runtime/reducer";

type ComputerReviewGateProps = {
  review: ComputerReviewItem;
  resolving?: boolean;
  onResolve: (decision: "approved_once" | "denied") => void;
};

export function ComputerReviewGate({
  review,
  resolving = false,
  onResolve,
}: ComputerReviewGateProps) {
  return (
    <section
      className="external-action-gate cli-auth-card cli-auth-card-compact"
      aria-live="polite"
      data-testid="computer-review-gate"
    >
      <div className="cli-auth-card-copy">
        <strong>计算机操作需要确认</strong>
        {review.targetSummary ? (
          <span>{review.targetSummary}</span>
        ) : (
          <span>Agent 请求执行一项计算机操作</span>
        )}
        {review.effectSummary ? (
          <span className="text-muted-foreground">预期效果：{review.effectSummary}</span>
        ) : null}
        {review.risk ? (
          <span className="text-muted-foreground">风险等级：{review.risk}</span>
        ) : null}
      </div>
      <div className="cli-auth-card-actions">
        <button
          type="button"
          disabled={resolving}
          onClick={() => onResolve("approved_once")}
        >
          允许一次
        </button>
        <button
          type="button"
          disabled={resolving}
          onClick={() => onResolve("denied")}
        >
          拒绝
        </button>
      </div>
    </section>
  );
}
