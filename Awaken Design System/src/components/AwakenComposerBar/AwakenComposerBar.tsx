import "./AwakenComposerBar.css";
import { useCallback } from "react";
import type { ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { AwakenComposerInput } from "../AwakenComposerInput";
import { AwakenContextButton } from "../AwakenContextButton";
import { AwakenSendButton } from "../AwakenSendButton";

export type ComposerBarMetaItem = {
  id: string;
  label: string;
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
      : metaItems;
  const handleSubmit = useCallback(() => {
    if (canSubmit) {
      onSubmit?.();
    }
  }, [canSubmit, onSubmit]);

  return (
    <div className={`composer-bar composer-bar-${variant}`}>
      {topMetaItems.length > 0 ? (
        <div className="composer-bar-meta" aria-label="AwakenComposer context">
          {topMetaItems.map((item) => (
            <span key={item.id} className="composer-bar-meta-item">
              {item.icon ? <span className="composer-bar-meta-icon">{item.icon}</span> : null}
              <span>{item.label}</span>
            </span>
          ))}
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
    </div>
  );
}
