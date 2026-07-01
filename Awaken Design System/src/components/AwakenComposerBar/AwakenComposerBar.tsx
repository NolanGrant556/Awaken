import "./AwakenComposerBar.css";
import { useCallback } from "react";
import type { CSSProperties, ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { AwakenComposerInput } from "../AwakenComposerInput";
import { AwakenContextButton } from "../AwakenContextButton";
import { AwakenSendButton } from "../AwakenSendButton";

export type ComposerBarMetaItem = {
  id: string;
  label: string;
  value?: string;
  progress?: number;
  icon?: ReactNode;
};

export type AwakenComposerBarProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onAddContext?: () => void;
  placeholder?: string;
  disabled?: boolean;
  isReady?: boolean;
  submitMode?: "send" | "stop";
  variant?: "center" | "dock";
  metaItems?: ComposerBarMetaItem[];
  focusRequest?: number;
};

export function AwakenComposerBar({
  value,
  onChange,
  onSubmit,
  onAddContext,
  placeholder,
  disabled,
  isReady = false,
  submitMode = "send",
  variant = "dock",
  metaItems = [],
  focusRequest = 0,
}: AwakenComposerBarProps) {
  const canSubmit = submitMode === "stop" || !disabled;
  const toolbarMetaItem =
    variant === "center" ? metaItems.find((item) => item.id === "model") : undefined;
  const topMetaItems =
    variant === "center" && toolbarMetaItem
      ? metaItems.filter((item) => item.id !== toolbarMetaItem.id)
      : variant === "center"
        ? metaItems
        : [];
  const bottomMetaItems = variant === "dock" ? metaItems : [];
  const handleSubmit = useCallback(() => {
    if (canSubmit) {
      onSubmit?.();
    }
  }, [canSubmit, onSubmit]);

  function renderMetaItem(item: ComposerBarMetaItem) {
    const hasProgress = typeof item.progress === "number" && Number.isFinite(item.progress);
    const progressPercent = hasProgress
      ? Math.max(0, Math.min(100, Math.round((item.progress ?? 0) * 100)))
      : undefined;

    return (
      <span
        key={item.id}
        className={`composer-bar-meta-item${
          hasProgress ? " composer-bar-meta-item-meter" : ""
        }`}
      >
        {item.icon ? <span className="composer-bar-meta-icon">{item.icon}</span> : null}
        <span className="composer-bar-meta-label">{item.label}</span>
        {hasProgress ? (
          <span
            className="composer-bar-context-meter"
            role="meter"
            aria-label={item.label}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={progressPercent}
            aria-valuetext={item.value}
          >
            <span
              className="composer-bar-context-meter-fill"
              style={{ "--composer-bar-context-progress": `${progressPercent}%` } as CSSProperties}
            />
          </span>
        ) : null}
        {item.value ? <span className="composer-bar-meta-value">{item.value}</span> : null}
      </span>
    );
  }

  return (
    <div className={`composer-bar composer-bar-${variant}`}>
      {topMetaItems.length > 0 ? (
        <div className="composer-bar-meta" aria-label="AwakenComposer context">
          {topMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
      <div className="composer-bar-surface">
        <div className="composer-bar-input-row">
          {variant === "dock" ? (
            <div className="composer-bar-context">
              <AwakenContextButton onClick={onAddContext} disabled={disabled} />
            </div>
          ) : null}
          <div className="composer-bar-input-wrap">
            <AwakenComposerInput
              value={value}
              onChange={onChange}
              onSubmit={handleSubmit}
              placeholder={placeholder}
              disabled={disabled}
              focusRequest={focusRequest}
              minRows={variant === "center" ? 2 : 1}
              maxRows={variant === "center" ? 8 : 5}
            />
          </div>
          {variant === "dock" ? (
            <div className="composer-bar-submit">
              <AwakenSendButton
                ready={isReady || submitMode === "stop"}
                onClick={handleSubmit}
                disabled={submitMode === "stop" ? false : disabled}
                mode={submitMode}
              />
            </div>
          ) : null}
        </div>
        {variant === "center" ? (
          <div className="composer-bar-toolbar">
            <div className="composer-bar-context">
              <AwakenContextButton onClick={onAddContext} disabled={disabled} />
            </div>
            {toolbarMetaItem ? (
              <div className="composer-bar-toolbar-meta">
                {toolbarMetaItem.icon ? (
                  <span className="composer-bar-meta-icon">{toolbarMetaItem.icon}</span>
                ) : null}
                <span>{toolbarMetaItem.label}</span>
                <ChevronDown size={16} strokeWidth={1.8} aria-hidden="true" />
              </div>
            ) : null}
            <div className="composer-bar-submit">
              <AwakenSendButton
                ready={isReady || submitMode === "stop"}
                onClick={handleSubmit}
                disabled={submitMode === "stop" ? false : disabled}
                mode={submitMode}
              />
            </div>
          </div>
        ) : null}
      </div>
      {bottomMetaItems.length > 0 ? (
        <div
          className="composer-bar-meta composer-bar-meta-bottom"
          aria-label="AwakenComposer context"
        >
          {bottomMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
    </div>
  );
}
