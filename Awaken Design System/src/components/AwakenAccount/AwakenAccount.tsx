import "./AwakenAccount.css";
import { useId, useState, type CSSProperties } from "react";
import { AwakenPopover } from "../AwakenPopover";
import { AwakenProfileButton } from "../AwakenProfileButton";
import {
  AwakenProfileBalancePanel,
  type AwakenProfileBalance,
} from "../AwakenProfileBalancePanel";
import type { AwakenThemeChoiceValue } from "../AwakenThemeChoiceGroup";

export type AwakenAccountProps = {
  name: string;
  avatarFallback: string;
  avatarSrc?: string;
  subtitle?: string;
  balances?: AwakenProfileBalance[];
  loading?: boolean;
  error?: string | null;
  open?: boolean;
  defaultOpen?: boolean;
  selectedProfileId?: string | null;
  defaultSelectedProfileId?: string | null;
  themePreference: AwakenThemeChoiceValue;
  themeSaving?: boolean;
  placement?: "top-start" | "top-end" | "bottom-start" | "bottom-end";
  panelId?: string;
  className?: string;
  style?: CSSProperties;
  onOpenChange?: (open: boolean) => void;
  onSelectedProfileIdChange?: (profileId: string) => void;
  onThemePreferenceChange?: (theme: AwakenThemeChoiceValue) => void;
  onOpenSettings?: () => void;
};

export function AwakenAccount({
  name,
  avatarFallback,
  avatarSrc,
  subtitle,
  balances = [],
  loading = false,
  error = null,
  open,
  defaultOpen = false,
  selectedProfileId,
  defaultSelectedProfileId,
  themePreference,
  themeSaving = false,
  placement = "top-start",
  panelId,
  className,
  style,
  onOpenChange,
  onSelectedProfileIdChange,
  onThemePreferenceChange,
  onOpenSettings,
}: AwakenAccountProps) {
  const generatedId = useId().replaceAll(":", "");
  const resolvedPanelId = panelId ?? `awaken-account-panel-${generatedId}`;
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const isControlled = open !== undefined;
  const isOpen = isControlled ? open : internalOpen;

  function setOpen(nextOpen: boolean): void {
    if (!isControlled) setInternalOpen(nextOpen);
    onOpenChange?.(nextOpen);
  }

  function openSettings(): void {
    setOpen(false);
    onOpenSettings?.();
  }

  return (
    <div
      className={["aw-account", className ?? ""].filter(Boolean).join(" ")}
      style={style}
    >
      <AwakenPopover
        open={isOpen}
        onOpenChange={setOpen}
        placement={placement}
        offset={8}
        trigger={
          <AwakenProfileButton
            name={name}
            subtitle={subtitle}
            avatarFallback={avatarFallback}
            avatarSrc={avatarSrc}
            expanded={isOpen}
            controls={resolvedPanelId}
          />
        }
        content={
          <AwakenProfileBalancePanel
            id={resolvedPanelId}
            balances={balances}
            loading={loading}
            error={error}
            selectedProfileId={selectedProfileId}
            defaultSelectedProfileId={defaultSelectedProfileId}
            themePreference={themePreference}
            themeSaving={themeSaving}
            role="region"
            onSelectedProfileIdChange={onSelectedProfileIdChange}
            onThemePreferenceChange={onThemePreferenceChange}
            onOpenSettings={openSettings}
          />
        }
      />
    </div>
  );
}
