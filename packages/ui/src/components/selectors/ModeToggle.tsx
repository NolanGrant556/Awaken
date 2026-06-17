import { useCallback } from "react";
import type { ComposerMode } from "../../types";
import { Pill } from "../primitives/Pill";

type ModeToggleProps = {
  mode: ComposerMode;
  onChange?: (mode: ComposerMode) => void;
  disabled?: boolean;
};

export function ModeToggle({ mode, onChange, disabled }: ModeToggleProps) {
  const handleToggle = useCallback(() => {
    const nextMode = mode === "Plan" ? "Act" : "Plan";
    onChange?.(nextMode);
  }, [mode, onChange]);

  return (
    <Pill
      variant="bordered"
      size="sm"
      onClick={handleToggle}
      disabled={disabled}
      aria-label={`当前模式: ${mode}，点击切换`}
    >
      {mode}
    </Pill>
  );
}
