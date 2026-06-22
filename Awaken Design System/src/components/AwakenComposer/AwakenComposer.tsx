import "./AwakenComposer.css";
import { useCallback } from "react";
import type { ModelOption, ComposerMode } from "../../types";
import { AwakenComposerInput } from "../AwakenComposerInput";
import { AwakenSendButton } from "../AwakenSendButton";
import { AwakenContextButton } from "../AwakenContextButton";
import { AwakenModeToggle } from "../AwakenModeToggle";
import { AwakenModelSelector } from "../AwakenModelSelector";

export type AwakenComposerProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onAddContext?: () => void;
  mode: ComposerMode;
  onModeChange?: (mode: ComposerMode) => void;
  model: ModelOption;
  models: ModelOption[];
  onModelChange?: (model: ModelOption) => void;
  placeholder?: string;
  disabled?: boolean;
  isReady?: boolean;
  submitMode?: "send" | "stop";
};

export function AwakenComposer({
  value,
  onChange,
  onSubmit,
  onAddContext,
  mode,
  onModeChange,
  model,
  models,
  onModelChange,
  placeholder,
  disabled,
  isReady = false,
  submitMode = "send",
}: AwakenComposerProps) {
  const canSubmit = submitMode === "stop" || !disabled;
  const handleSubmit = useCallback(() => {
    if (canSubmit) {
      onSubmit?.();
    }
  }, [canSubmit, onSubmit]);

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "8px",
        padding: "12px 16px",
        borderRadius: "var(--aw-radius-workbench)",
        background: "var(--aw-surface-composer)",
        border: "1px solid var(--aw-border-light)",
        minHeight: 116,
      }}
    >
      <AwakenComposerInput
        value={value}
        onChange={onChange}
        onSubmit={handleSubmit}
        placeholder={placeholder}
        disabled={disabled}
      />
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "8px",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "6px",
          }}
        >
          <AwakenContextButton onClick={onAddContext} disabled={disabled} />
          <AwakenModeToggle mode={mode} onChange={onModeChange} disabled={disabled} />
          <AwakenModelSelector
            value={model}
            options={models}
            onChange={onModelChange}
            disabled={disabled}
          />
        </div>
        <AwakenSendButton
          ready={isReady}
          onClick={handleSubmit}
          disabled={submitMode === "stop" ? false : disabled}
          mode={submitMode}
        />
      </div>
    </div>
  );
}
