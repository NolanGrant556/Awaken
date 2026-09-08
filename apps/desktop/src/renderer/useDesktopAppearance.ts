import { useEffect, useState } from "react";
import type { AwakenTheme, AwakenThemePreference } from "../shared/runtime";
import {
  STORED_THEME_KEY,
  LEGACY_STORED_FONT_SCALE_KEY,
  SYSTEM_THEME_QUERY,
  readStoredThemePreference,
  readSystemTheme,
  effectiveThemeForPreference,
} from "./desktopAppearance";
import { writeStoredValue } from "./desktopStorage";
import type { RuntimeClient } from "./runtime/client";

export function useDesktopAppearance(runtimeClient: RuntimeClient) {
  const [appearanceThemePreference, setAppearanceThemePreference] =
    useState<AwakenThemePreference>(readStoredThemePreference);

  const [systemAppearanceTheme, setSystemAppearanceTheme] =
    useState<AwakenTheme>(readSystemTheme);

  const appearanceTheme = effectiveThemeForPreference(
    appearanceThemePreference,
    systemAppearanceTheme,
  );

  const [appearanceSaving, setAppearanceSaving] = useState(false);

  const [appearanceMessage, setAppearanceMessage] = useState<string | null>(
    null,
  );

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }
    document.documentElement.dataset.awTheme = appearanceTheme;
    document.documentElement.dataset.awThemePreference =
      appearanceThemePreference;
    delete document.documentElement.dataset.awFontScale;
    document.documentElement.style.colorScheme =
      appearanceTheme === "dark" ? "dark" : "light";
    writeStoredValue(STORED_THEME_KEY, appearanceThemePreference);
    try {
      window.localStorage.removeItem(LEGACY_STORED_FONT_SCALE_KEY);
    } catch {
      // Leftover scale mirrors are unused after the preference was removed.
    }
  }, [appearanceTheme, appearanceThemePreference]);

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return;
    }

    const media = window.matchMedia(SYSTEM_THEME_QUERY);
    const updateSystemTheme = (matches: boolean) => {
      setSystemAppearanceTheme(matches ? "dark" : "light");
    };
    const handleChange = (event: MediaQueryListEvent) => {
      updateSystemTheme(event.matches);
    };

    updateSystemTheme(media.matches);
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadAppearancePreferences(): Promise<void> {
      try {
        const preferences = await runtimeClient.getDesktopPreferences();
        if (!cancelled) {
          setAppearanceThemePreference(preferences.theme);
        }
      } catch {
        // The localStorage mirror already provides the best available fallback.
      }
    }

    void loadAppearancePreferences();
    return () => {
      cancelled = true;
    };
  }, [runtimeClient]);

  async function handleAppearanceThemeChange(
    theme: AwakenThemePreference,
  ): Promise<void> {
    if (theme === appearanceThemePreference || appearanceSaving) {
      return;
    }

    const previousTheme = appearanceThemePreference;
    setAppearanceThemePreference(theme);
    setAppearanceSaving(true);
    setAppearanceMessage(null);
    try {
      const preferences = await runtimeClient.saveDesktopPreferences({ theme });
      setAppearanceThemePreference(preferences.theme);
    } catch (error) {
      setAppearanceThemePreference(previousTheme);
      setAppearanceMessage(
        error instanceof Error ? error.message : "Failed to save theme",
      );
    } finally {
      setAppearanceSaving(false);
    }
  }
  return {
    appearanceThemePreference,
    appearanceTheme,
    appearanceSaving,
    appearanceMessage,
    handleAppearanceThemeChange,
  };
}
