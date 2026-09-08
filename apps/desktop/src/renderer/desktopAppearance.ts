import type { AwakenTheme, AwakenThemePreference } from "../shared/runtime";
import { readStoredString } from "./desktopStorage";

export const STORED_THEME_KEY = "awaken:appearance:theme";

export const LEGACY_STORED_FONT_SCALE_KEY = "awaken:appearance:fontScale";

export const SYSTEM_THEME_QUERY = "(prefers-color-scheme: dark)";

export function normalizeThemePreference(value: string): AwakenThemePreference {
  return value === "system" || value === "dark" ? value : "light";
}

export function readStoredThemePreference(): AwakenThemePreference {
  return normalizeThemePreference(readStoredString(STORED_THEME_KEY));
}

export function readSystemTheme(): AwakenTheme {
  if (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(SYSTEM_THEME_QUERY).matches
  ) {
    return "dark";
  }
  return "light";
}

export function effectiveThemeForPreference(
  preference: AwakenThemePreference,
  systemTheme: AwakenTheme,
): AwakenTheme {
  return preference === "system" ? systemTheme : preference;
}
