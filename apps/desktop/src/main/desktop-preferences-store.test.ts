import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { DesktopPreferencesStore } from "./desktop-preferences-store.js";

let configDir: string;

beforeEach(async () => {
  configDir = await mkdtemp(join(tmpdir(), "awaken-desktop-preferences-"));
});

afterEach(async () => {
  await rm(configDir, { recursive: true, force: true });
});

describe("DesktopPreferencesStore", () => {
  it("defaults to the existing light theme when no preferences file exists", async () => {
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();

    expect(store.snapshot()).toEqual({ theme: "light", fontScale: 100 });
  });

  it("persists the selected dark theme", async () => {
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();
    await store.savePreferences({ theme: "dark" });

    const reloaded = new DesktopPreferencesStore(configDir);
    await reloaded.initialize();

    expect(reloaded.snapshot()).toEqual({ theme: "dark", fontScale: 100 });
    await expect(
      readFile(join(configDir, "desktop-preferences.json"), "utf8"),
    ).resolves.toContain('"theme": "dark"');
  });

  it("persists the system theme preference", async () => {
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();
    await store.savePreferences({ theme: "system" });

    const reloaded = new DesktopPreferencesStore(configDir);
    await reloaded.initialize();

    expect(reloaded.snapshot()).toEqual({ theme: "system", fontScale: 100 });
    await expect(
      readFile(join(configDir, "desktop-preferences.json"), "utf8"),
    ).resolves.toContain('"theme": "system"');
  });

  it("normalizes unknown stored themes back to light", async () => {
    await writeFile(
      join(configDir, "desktop-preferences.json"),
      JSON.stringify({ version: 1, theme: "sepia" }),
      "utf8",
    );
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();

    expect(store.snapshot()).toEqual({ theme: "light", fontScale: 100 });
  });

  it("persists a supported font scale and keeps the current theme", async () => {
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();
    await store.savePreferences({ fontScale: 150 });

    const reloaded = new DesktopPreferencesStore(configDir);
    await reloaded.initialize();

    expect(reloaded.snapshot()).toEqual({ theme: "light", fontScale: 150 });
    await expect(
      readFile(join(configDir, "desktop-preferences.json"), "utf8"),
    ).resolves.toContain('"fontScale": 150');
  });

  it("normalizes unknown font scales back to 100", async () => {
    await writeFile(
      join(configDir, "desktop-preferences.json"),
      JSON.stringify({ version: 1, theme: "dark", fontScale: 180 }),
    );
    const store = new DesktopPreferencesStore(configDir);

    await store.initialize();

    expect(store.snapshot()).toEqual({ theme: "dark", fontScale: 100 });
  });
});
