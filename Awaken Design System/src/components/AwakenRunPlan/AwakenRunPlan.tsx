import "./AwakenRunPlan.css";
import { Check, Circle, LoaderCircle, X } from "../../icons";
import { forwardRef, useId } from "react";

// AwakenRunPlanItemView mirrors the renderer-side RunPlanView item fields the
// component family needs. The Design System must not import from
// renderer/shared, so the narrow DTO is re-declared here and the renderer is
// responsible for projecting only the documented fields.
export type AwakenRunPlanItemStatus = "pending" | "in_progress" | "completed";

export type AwakenRunPlanItemView = {
  step: string;
  activeForm: string;
  status: AwakenRunPlanItemStatus;
};

const statusLabels: Record<AwakenRunPlanItemStatus, string> = {
  completed: "已完成",
  in_progress: "进行中",
  pending: "待处理",
};

function resolveCurrentStepLabel(
  items: AwakenRunPlanItemView[],
): string | null {
  const activeItem = items.find((item) => item.status === "in_progress");
  if (!activeItem) {
    return null;
  }
  const activeForm = activeItem.activeForm.trim();
  return activeForm.length > 0 ? activeForm : activeItem.step;
}

function RunPlanStatusIcon({
  status,
  animated,
}: {
  status: AwakenRunPlanItemStatus;
  animated: boolean;
}) {
  if (status === "completed") {
    return <Check size={16} strokeWidth={2} aria-hidden="true" />;
  }
  if (status === "in_progress") {
    return (
      <LoaderCircle
        size={16}
        strokeWidth={2}
        aria-hidden="true"
        className={animated ? "aw-run-plan-spin" : undefined}
      />
    );
  }
  return <Circle size={16} strokeWidth={1.8} aria-hidden="true" />;
}

// RunPlanItemList is the shared read-only step list used by both the expanded
// panel and the historical variant. It exposes each status as visible text in
// addition to the icon, never by color alone.
function RunPlanItemList({
  items,
  animated,
}: {
  items: AwakenRunPlanItemView[];
  animated: boolean;
}) {
  return (
    <ol className="aw-run-plan-list">
      {items.map((item, index) => (
        <li
          key={`${index}:${item.step}`}
          className={`aw-run-plan-item aw-run-plan-item--${item.status}`}
        >
          <span className="aw-run-plan-item__icon">
            <RunPlanStatusIcon status={item.status} animated={animated} />
          </span>
          <span className="aw-run-plan-item__step">{item.step}</span>
          <span className="aw-run-plan-item__status">
            {statusLabels[item.status]}
          </span>
        </li>
      ))}
    </ol>
  );
}

export type AwakenRunPlanTriggerProps = {
  items: AwakenRunPlanItemView[];
  completedCount: number;
  totalCount: number;
  currentStep: number;
  allCompleted: boolean;
  open: boolean;
  onToggle: () => void;
};

// AwakenRunPlanTrigger is the compact entry inside the ComposerDock stack. It
// is UI-only and controlled: the caller owns `open`, focus restore and
// positioning; the trigger only renders the current snapshot counts and the
// active step label.
export const AwakenRunPlanTrigger = forwardRef<
  HTMLButtonElement,
  AwakenRunPlanTriggerProps
>(function AwakenRunPlanTrigger(
  { items, completedCount, totalCount, currentStep, allCompleted, open, onToggle },
  forwardedRef,
) {
  const currentLabel = allCompleted ? null : resolveCurrentStepLabel(items);
  const summaryText = allCompleted
    ? `${totalCount}/${totalCount} · 计划已完成`
    : `第 ${currentStep}/${totalCount} 步 · ${currentLabel ?? ""}`;
  const accessibleLabel = allCompleted
    ? `运行计划，共 ${totalCount} 步，全部完成`
    : `运行计划，第 ${currentStep} 步，共 ${totalCount} 步，已完成 ${completedCount} 步${
        currentLabel ? `，当前：${currentLabel}` : ""
      }`;
  const iconStatus: AwakenRunPlanItemStatus = allCompleted
    ? "completed"
    : currentLabel !== null
      ? "in_progress"
      : "pending";

  return (
    <button
      ref={forwardedRef}
      type="button"
      className={`aw-run-plan-trigger aw-run-plan-trigger--${iconStatus}`}
      aria-label={accessibleLabel}
      aria-expanded={open}
      aria-haspopup="dialog"
      onClick={onToggle}
    >
      <span className="aw-run-plan-trigger__icon">
        <RunPlanStatusIcon status={iconStatus} animated />
      </span>
      <span className="aw-run-plan-trigger__text">{summaryText}</span>
    </button>
  );
});

export type AwakenRunPlanPanelProps = {
  id?: string;
  items: AwakenRunPlanItemView[];
  explanation?: string;
  onClose: () => void;
};

// AwakenRunPlanPanel is the expanded read-only plan rendered inside
// RunStage.overlay. The outer wrapper is pointer-through while the panel body
// is pointer-auto. Escape / outside-click close and bottom positioning are
// orchestrated by the caller; the panel only renders the snapshot.
export const AwakenRunPlanPanel = forwardRef<
  HTMLDivElement,
  AwakenRunPlanPanelProps
>(function AwakenRunPlanPanel(
  { id, items, explanation, onClose },
  forwardedRef,
) {
  const titleId = useId();

  return (
    <div className="aw-run-plan-panel-wrap">
      <div
        id={id}
        ref={forwardedRef}
        className="aw-run-plan-panel"
        role="dialog"
        aria-modal="false"
        aria-labelledby={titleId}
      >
        <header className="aw-run-plan-panel__header">
          <h2 id={titleId} className="aw-run-plan-panel__title">
            运行计划
          </h2>
          <button
            type="button"
            className="aw-run-plan-panel__close"
            aria-label="关闭运行计划"
            onClick={onClose}
          >
            <X size={15} strokeWidth={1.9} aria-hidden="true" />
          </button>
        </header>
        {explanation ? (
          <p className="aw-run-plan-panel__explanation">{explanation}</p>
        ) : null}
        <div className="aw-run-plan-panel__scroll">
          <RunPlanItemList items={items} animated />
        </div>
      </div>
    </div>
  );
});

export type AwakenRunPlanHistoricalProps = {
  items: AwakenRunPlanItemView[];
  completedCount: number;
  totalCount: number;
};

// AwakenRunPlanHistorical is the read-only terminal snapshot shown inside a
// turn's execution process. It renders the same step list without animation
// or interaction, preserving the real tri-state at run terminal.
export const AwakenRunPlanHistorical = forwardRef<
  HTMLDivElement,
  AwakenRunPlanHistoricalProps
>(function AwakenRunPlanHistorical(
  { items, completedCount, totalCount },
  forwardedRef,
) {
  const titleId = useId();

  return (
    <section
      ref={forwardedRef}
      className="aw-run-plan-historical"
      aria-labelledby={titleId}
    >
      <h3 id={titleId} className="aw-run-plan-historical__title">
        {`执行计划 · ${completedCount}/${totalCount}`}
      </h3>
      <RunPlanItemList items={items} animated={false} />
    </section>
  );
});
