import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  AwakenProfileMenuPanel,
  AwakenProfileUsageDialog,
  type AwakenProfileBalance,
  type AwakenThemeChoiceValue,
} from "@awaken/design-system";
import type { ModelProfileBalance } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import { providerBrandForId } from "./model-provider-brand";

type ProfileBalancePopoverProps = {
  open: boolean;
  anchorRef: React.RefObject<HTMLElement | null>;
  runtimeClient: RuntimeClient;
  themePreference: AwakenThemeChoiceValue;
  themeSaving: boolean;
  onThemePreferenceChange: (theme: AwakenThemeChoiceValue) => void;
  onOpenSettings: () => void;
  onClose: () => void;
};

export function credentialRecoveryForBalances(
  balances: ModelProfileBalance[],
): "unreadable" | null {
  return balances.some((balance) => balance.credentialIssue === "unreadable")
    ? "unreadable"
    : null;
}

export function ProfileBalancePopover({
  open,
  anchorRef,
  runtimeClient,
  themePreference,
  themeSaving,
  onThemePreferenceChange,
  onOpenSettings,
  onClose,
}: ProfileBalancePopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const [usageOpen, setUsageOpen] = useState(false);
  const [balances, setBalances] = useState<AwakenProfileBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedProfileId, setSelectedProfileId] = useState<string | null>(null);
  const [position, setPosition] = useState<{ left: number; bottom: number }>({
    left: 0,
    bottom: 0,
  });

  useLayoutEffect(() => {
    if (!open || !anchorRef.current) return;
    const rect = anchorRef.current.getBoundingClientRect();
    const gap = 8;
    setPosition({
      left: rect.left,
      bottom: window.innerHeight - rect.top + gap,
    });
  }, [open, anchorRef]);

  useEffect(() => {
    if (!usageOpen) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    setBalances([]);
    setSelectedProfileId(null);

    async function load(): Promise<void> {
      try {
        const result = await runtimeClient.getModelProfileBalances();
        if (cancelled) return;
        setBalances(result.balances.map((balance) => ({
          ...balance,
          providerBrand: providerBrandForId(balance.providerId),
        })));
        setSelectedProfileId(result.balances[0]?.profileId ?? null);
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "查询用量失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [runtimeClient, usageOpen]);

  useEffect(() => {
    if (!open) return;
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") onClose();
    }
    function handlePointerDown(event: MouseEvent): void {
      const target = event.target as Node;
      if (
        popoverRef.current &&
        !popoverRef.current.contains(target) &&
        !anchorRef.current?.contains(target)
      ) {
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("pointerdown", handlePointerDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [open, onClose, anchorRef]);

  const handleOpenSettings = useCallback(() => {
    setUsageOpen(false);
    onClose();
    onOpenSettings();
  }, [onClose, onOpenSettings]);

  const handleOpenUsage = useCallback(() => {
    onClose();
    setUsageOpen(true);
  }, [onClose]);

  return (
    <>
      {open ? (
        <div
          ref={popoverRef}
          className="profile-balance-popover"
          style={{ left: position.left, bottom: position.bottom }}
        >
          <AwakenProfileMenuPanel
            themePreference={themePreference}
            themeSaving={themeSaving}
            onOpenUsage={handleOpenUsage}
            onThemePreferenceChange={onThemePreferenceChange}
            onOpenSettings={handleOpenSettings}
          />
        </div>
      ) : null}

      <AwakenProfileUsageDialog
        open={usageOpen}
        balances={balances}
        loading={loading}
        error={error}
        selectedProfileId={selectedProfileId}
        onSelectedProfileIdChange={setSelectedProfileId}
        onOpenSettings={handleOpenSettings}
        onOpenChange={setUsageOpen}
      />
    </>
  );
}
