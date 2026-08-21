import "./AwakenThemeChoiceGroup.css";
import { AwakenIcon } from "../AwakenIcon";

export type AwakenThemeChoiceValue = "system" | "light" | "dark";

export type AwakenThemeChoiceGroupProps = {
  value: AwakenThemeChoiceValue;
  size?: "default" | "compact" | "menu";
  disabled?: boolean;
  onValueChange?: (value: AwakenThemeChoiceValue) => void;
};

const choices = [
  { value: "system", label: "跟随系统", concept: "theme-system" as const },
  { value: "light", label: "浅色", concept: "theme-light" as const },
  { value: "dark", label: "黑色", concept: "theme-dark" as const },
] as const;

export function AwakenThemeChoiceGroup({
  value,
  size = "default",
  disabled = false,
  onValueChange,
}: AwakenThemeChoiceGroupProps) {
  return (
    <div
      className={`theme-choice-group theme-choice-group--${size}`}
      role="radiogroup"
      aria-label="主题"
    >
      {choices.map(({ value: choiceValue, label, concept }) => {
        const selected = choiceValue === value;
        return (
          <button
            key={choiceValue}
            type="button"
            className={["theme-choice", selected ? "theme-choice-active" : ""].filter(Boolean).join(" ")}
            role="radio"
            aria-checked={selected}
            disabled={disabled}
            onClick={() => onValueChange?.(choiceValue)}
          >
            <AwakenIcon concept={concept} size={16} />
            <span>{label}</span>
          </button>
        );
      })}
    </div>
  );
}
