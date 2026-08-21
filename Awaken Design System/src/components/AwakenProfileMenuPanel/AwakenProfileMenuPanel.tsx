import "./AwakenProfileMenuPanel.css";
import { AwakenIcon } from "../AwakenIcon";
import {
  useId,
  useState,
  type AriaRole,
  type CSSProperties,
  type ReactNode,
} from "react";
import {
  AwakenThemeChoiceGroup,
  type AwakenThemeChoiceValue,
} from "../AwakenThemeChoiceGroup";

export type AwakenProfileMenuPanelProps = {
  id?: string;
  themePreference: AwakenThemeChoiceValue;
  themeSaving?: boolean;
  settingsLabel?: ReactNode;
  ariaLabel?: string;
  role?: AriaRole;
  className?: string;
  style?: CSSProperties;
  onOpenUsage?: () => void;
  onThemePreferenceChange?: (theme: AwakenThemeChoiceValue) => void;
  onOpenSettings?: () => void;
};

export function AwakenProfileMenuPanel({
  id,
  themePreference,
  themeSaving = false,
  settingsLabel = "设置",
  ariaLabel = "个人菜单",
  role = "dialog",
  className,
  style,
  onOpenUsage,
  onThemePreferenceChange,
  onOpenSettings,
}: AwakenProfileMenuPanelProps) {
  const generatedId = useId().replaceAll(":", "");
  const appearancePanelId = `${id ?? `aw-profile-menu-${generatedId}`}-appearance`;
  const [appearanceOpen, setAppearanceOpen] = useState(false);

  function selectTheme(theme: AwakenThemeChoiceValue): void {
    onThemePreferenceChange?.(theme);
    setAppearanceOpen(false);
  }

  return (
    <div
      id={id}
      className={["aw-profile-menu-panel", className ?? ""].filter(Boolean).join(" ")}
      style={style}
      role={role}
      aria-label={ariaLabel}
    >
      <div className="aw-profile-menu-panel__footer">
        <div className="aw-profile-menu-panel__appearance">
          <button
            type="button"
            className="aw-profile-menu-panel__action"
            aria-haspopup="dialog"
            aria-expanded={appearanceOpen}
            aria-controls={appearancePanelId}
            onClick={() => setAppearanceOpen((current) => !current)}
          >
            <AwakenIcon concept="settings-appearance" size={16} />
            <span>外观</span>
            <AwakenIcon concept="disclose" size={14} className="aw-profile-menu-panel__action-chevron" />
          </button>
          {appearanceOpen ? (
            <div
              id={appearancePanelId}
              className="aw-profile-menu-panel__appearance-popover"
              role="dialog"
              aria-label="外观"
            >
              <AwakenThemeChoiceGroup
                value={themePreference}
                size="menu"
                disabled={themeSaving}
                onValueChange={selectTheme}
              />
            </div>
          ) : null}
        </div>
        <button
          type="button"
          className="aw-profile-menu-panel__action"
          onClick={onOpenUsage}
        >
          <AwakenIcon concept="usage" size={16} />
          <span>查询用量</span>
        </button>
        <button
          type="button"
          className="aw-profile-menu-panel__action"
          onClick={onOpenSettings}
        >
          <AwakenIcon concept="settings" size={16} />
          {settingsLabel}
        </button>
      </div>
    </div>
  );
}
