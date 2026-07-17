import { mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { AwakenEnvStore } from "./awaken-env-store.js";

let awakenHome: string;

beforeEach(async () => {
  awakenHome = await mkdtemp(join(tmpdir(), "awaken-env-store-"));
});

afterEach(async () => {
  await rm(awakenHome, { recursive: true, force: true });
});

describe("AwakenEnvStore", () => {
  it("atomically stores one escaped key per profile with mode 0600", async () => {
    const store = new AwakenEnvStore(awakenHome);
    await Promise.all([
      store.setKey("profile-one", "key with spaces and # hash"),
      store.setKey("profile-two", "line-one\nline-two=ok"),
    ]);

    await expect(store.getKey("profile-one")).resolves.toBe("key with spaces and # hash");
    await expect(store.getKey("profile-two")).resolves.toBe("line-one\nline-two=ok");
    const envPath = join(awakenHome, ".env");
    if (process.platform !== "win32") {
      expect((await stat(envPath)).mode & 0o777).toBe(0o600);
    }
    const raw = await readFile(envPath, "utf8");
    expect(raw).toContain("AWAKEN_API_KEY_profile-one=");
    expect(raw).toContain("AWAKEN_API_KEY_profile-two=");
    expect(raw).not.toContain(".tmp");
  });

  it("preserves unrelated variables and deletes only the selected profile key", async () => {
    await writeFile(join(awakenHome, ".env"), 'UNRELATED="keep"\n', "utf8");
    const store = new AwakenEnvStore(awakenHome);
    await store.setKey("profile-one", "secret");
    await store.deleteKey("profile-one");

    expect(await readFile(join(awakenHome, ".env"), "utf8")).toBe('UNRELATED="keep"\n');
    await expect(store.getKey("profile-one")).resolves.toBe("");
  });

  it("rejects profile IDs that could inject another env assignment", async () => {
    const store = new AwakenEnvStore(awakenHome);
    await expect(store.setKey("bad\nINJECTED", "secret")).rejects.toMatchObject({
      code: "invalid_profile_id",
    });
    await expect(readFile(join(awakenHome, ".env"), "utf8")).rejects.toMatchObject({
      code: "ENOENT",
    });
  });
});
