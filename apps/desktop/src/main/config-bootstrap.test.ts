import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
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
  it("creates config.toml and .env templates on first launch", async () => {
    const configDir = await ensureConfigDir();

    expect(configDir).toBe(join(userDataDir, "config"));
    expect(await readFile(join(configDir, "config.toml"), "utf8")).toContain("[llm]");
    expect(await readFile(join(configDir, "config.toml"), "utf8")).toContain("model_id");
    expect(await readFile(join(configDir, ".env"), "utf8")).toContain("LLM_API_KEY=");
  });

  it("preserves existing config.toml and .env files", async () => {
    const configDir = join(userDataDir, "config");
    await mkdir(configDir, { recursive: true });
    await writeFile(join(configDir, "config.toml"), "sentinel config", "utf8");
    await writeFile(join(configDir, ".env"), "sentinel env", "utf8");

    await ensureConfigDir();

    expect(await readFile(join(configDir, "config.toml"), "utf8")).toBe("sentinel config");
    expect(await readFile(join(configDir, ".env"), "utf8")).toBe("sentinel env");
  });
});
