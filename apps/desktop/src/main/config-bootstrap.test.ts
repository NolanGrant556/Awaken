import { mkdtemp, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ensureConfigDir } from "./config-bootstrap.js";

const electronMock = vi.hoisted(() => ({
  getPath: vi.fn(),
}));

vi.mock("electron", () => ({
  app: { getPath: electronMock.getPath },
}));

let userDataDir: string;

beforeEach(async () => {
  userDataDir = await mkdtemp(join(tmpdir(), "awaken-user-data-"));
  electronMock.getPath.mockReturnValue(userDataDir);
});

afterEach(async () => {
  await rm(userDataDir, { recursive: true, force: true });
  vi.clearAllMocks();
});

describe("ensureConfigDir", () => {
  it("creates only the config directory on first launch", async () => {
    const configDir = await ensureConfigDir();

    expect(configDir).toBe(join(userDataDir, "config"));
    expect(await readdir(configDir)).toEqual([]);
  });

  it("preserves existing files in the config directory", async () => {
    const configDir = join(userDataDir, "config");
    await mkdir(configDir, { recursive: true });
    await writeFile(join(configDir, "desktop-preferences.json"), "sentinel config", "utf8");
    await writeFile(join(configDir, "notes.txt"), "sentinel notes", "utf8");

    await ensureConfigDir();

    expect(await readFile(join(configDir, "desktop-preferences.json"), "utf8")).toBe("sentinel config");
    expect(await readFile(join(configDir, "notes.txt"), "utf8")).toBe("sentinel notes");
  });
});
