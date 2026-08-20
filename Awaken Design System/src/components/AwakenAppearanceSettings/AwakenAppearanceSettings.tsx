import "./AwakenAppearanceSettings.css";
import {
  AwakenThemeChoiceGroup,
  type AwakenThemeChoiceGroupProps,
} from "../AwakenThemeChoiceGroup";
import { AwakenSelect } from "../AwakenSelect";
import type { ReactNode } from "react";

export const AWAKEN_FONT_SCALE_OPTIONS = [
  { value: "100", label: "100%" },
  { value: "125", label: "125%" },
  { value: "150", label: "150%" },
  { value: "200", label: "200%" },
] as const;

export type AwakenFontScaleValue = (typeof AWAKEN_FONT_SCALE_OPTIONS)[number]["value"];

export type AwakenAppearanceSettingsProps = AwakenThemeChoiceGroupProps & {
  message?: ReactNode;
  fontScale?: AwakenFontScaleValue;
  fontScaleDisabled?: boolean;
  onFontScaleChange?: (value: AwakenFontScaleValue) => void;
};

export function AwakenAppearanceSettings({
  message,
  fontScale = "100",
  fontScaleDisabled,
  onFontScaleChange,
  ...props
}: AwakenAppearanceSettingsProps) {
  return (
    <section className="appearance-settings" aria-label="外观">
      <div className="appearance-settings-row">
        <div className="appearance-settings-copy">
          <h2>主题</h2>
          <p>选择 Awaken 界面的显示方式。</p>
        </div>
        <AwakenThemeChoiceGroup {...props} />
      </div>
      <div className="appearance-settings-row">
        <div className="appearance-settings-copy">
          <h2>显示大小</h2>
          <p>只放大字号和行高，不改变栏宽或图标几何。</p>
        </div>
        <AwakenSelect
          value={fontScale}
          options={AWAKEN_FONT_SCALE_OPTIONS}
          ariaLabel="显示大小"
          disabled={fontScaleDisabled}
          onValueChange={(value) => {
            if (
              value === "100" ||
              value === "125" ||
              value === "150" ||
              value === "200"
            ) {
              onFontScaleChange?.(value);
            }
          }}
        />
      </div>
      {message ? <div className="model-settings-message">{message}</div> : null}
    </section>
  );
}
