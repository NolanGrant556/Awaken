import type {
  AwakenTheme,
  AwakenThemePreference,
} from "../shared/runtime.js";

export type MainWindowBackgroundTarget = {
  isDestroyed: () => boolean;
  setBackgroundColor: (color: string) => void;
};

function effectiveThemeForPreference(
  theme: AwakenThemePreference | undefined,
  shouldUseDarkColors: boolean,
): AwakenTheme {
  if (theme === "system") {
    return shouldUseDarkColors ? "dark" : "light";
  }
  return theme === "dark" ? "dark" : "light";
}

export function windowBackgroundColor(
  theme: AwakenThemePreference | undefined,
  shouldUseDarkColors: boolean,
): string {
  return effectiveThemeForPreference(theme, shouldUseDarkColors) === "dark"
    ? "#0f0f0e"
    : "#f8f8f7";
}

export function updateMainWindowBackground(
  mainWindow: MainWindowBackgroundTarget | null,
  theme: AwakenThemePreference,
  shouldUseDarkColors: boolean,
): void {
  if (!mainWindow || mainWindow.isDestroyed()) {
    return;
  }
  mainWindow.setBackgroundColor(windowBackgroundColor(theme, shouldUseDarkColors));
}
