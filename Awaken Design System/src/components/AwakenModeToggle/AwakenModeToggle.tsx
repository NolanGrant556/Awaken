import "./AwakenModeToggle.css";
import { useCallback } from "react";
import type { ComposerMode } from "../../types";
import { AwakenPill } from "../AwakenPill";

export type AwakenModeToggleProps = {
  mode: ComposerMode;
  onChange?: (mode: ComposerMode) => void;
  disabled?: boolean;
};

export function AwakenModeToggle({ mode, onChange, disabled }: AwakenModeToggleProps) {
  const handleToggle = useCallback(() => {
    const nextMode = mode === "Plan" ? "Act" : "Plan";
    onChange?.(nextMode);
  }, [mode, onChange]);

  return (
    <AwakenPill
      variant="bordered"
      size="sm"
      onClick={handleToggle}
      disabled={disabled}
      aria-label={`当前模式: ${mode}，点击切换`}
    >
      {mode}
    </AwakenPill>
  );
}
