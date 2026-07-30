import type { ComputerHandoffItem } from "./runtime/reducer";

type ComputerHandoffIndicatorProps = {
  handoff: ComputerHandoffItem;
  busy?: boolean;
  onReturn: () => void;
};

export function ComputerHandoffIndicator({
  handoff,
  busy = false,
  onReturn,
}: ComputerHandoffIndicatorProps) {
  return (
    <section
      className="external-action-gate cli-auth-card cli-auth-card-compact"
      aria-live="polite"
      data-testid="computer-handoff-indicator"
    >
      <div className="cli-auth-card-copy">
        <strong>
          {handoff.active ? "你已接管计算机控制" : "Agent 已恢复计算机控制"}
        </strong>
        <span>
          {handoff.active
            ? "你可以直接操作系统。完成后再交还控制权，Agent 会继续执行。"
            : "控制权已交还 Agent，继续自动执行。"}
        </span>
      </div>
      {handoff.active ? (
        <div className="cli-auth-card-actions">
          <button type="button" disabled={busy} onClick={() => onReturn()}>
            交还控制权
          </button>
        </div>
      ) : null}
    </section>
  );
}
