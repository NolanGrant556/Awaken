import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import type {
  AwakenThemePreference,
  DesktopPreferences,
  DesktopPreferencesSaveRequest,
} from "../shared/runtime.js";

const DESKTOP_PREFERENCES_FILE = "desktop-preferences.json";

const DEFAULT_DESKTOP_PREFERENCES: DesktopPreferences = {
  theme: "light",
};

function normalizeThemePreference(value: unknown): AwakenThemePreference {
  return value === "system" || value === "dark" ? value : "light";
}

function normalizePreferences(value: unknown): DesktopPreferences {
  if (!value || typeof value !== "object") {
    return { ...DEFAULT_DESKTOP_PREFERENCES };
  }

  const candidate = value as Record<string, unknown>;
  return {
    theme: normalizeThemePreference(candidate.theme),
  };
}

export class DesktopPreferencesStore {
  private preferences: DesktopPreferences = { ...DEFAULT_DESKTOP_PREFERENCES };
  private readonly filePath: string;

  constructor(configDir: string) {
    this.filePath = join(configDir, DESKTOP_PREFERENCES_FILE);
  }

  async initialize(): Promise<void> {
    this.preferences = await this.readPreferences();
  }

  snapshot(): DesktopPreferences {
    return { ...this.preferences };
  }

  async savePreferences(
    request: DesktopPreferencesSaveRequest = {},
  ): Promise<DesktopPreferences> {
    this.preferences = normalizePreferences({
      ...this.preferences,
      ...(request ?? {}),
    });
    await this.writePreferences(this.preferences);
    return this.snapshot();
  }

  private async readPreferences(): Promise<DesktopPreferences> {
    try {
      const text = await readFile(this.filePath, "utf8");
      return normalizePreferences(JSON.parse(text));
    } catch {
      return { ...DEFAULT_DESKTOP_PREFERENCES };
    }
  }

  private async writePreferences(preferences: DesktopPreferences): Promise<void> {
    await mkdir(dirname(this.filePath), { recursive: true });
    await writeFile(
      this.filePath,
      `${JSON.stringify({ version: 1, ...preferences }, null, 2)}\n`,
      "utf8",
    );
  }
}
