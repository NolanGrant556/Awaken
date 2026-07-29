import "./AwakenThemeChoiceGroup.css";
import { Monitor, Moon, Sun } from "lucide-react";

export type AwakenThemeChoiceValue = "system" | "light" | "dark";

export type AwakenThemeChoiceGroupProps = {
  value: AwakenThemeChoiceValue;
  size?: "default" | "compact";
  disabled?: boolean;
  onValueChange?: (value: AwakenThemeChoiceValue) => void;
};

const choices = [
  { value: "system", label: "跟随系统", Icon: Monitor },
  { value: "light", label: "浅色", Icon: Sun },
  { value: "dark", label: "黑色", Icon: Moon },
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
      {choices.map(({ value: choiceValue, label, Icon }) => {
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
            <Icon size={17} strokeWidth={1.9} aria-hidden="true" />
            <span>{label}</span>
          </button>
        );
      })}
    </div>
  );
}
