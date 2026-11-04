import { describe, expect, it, vi } from "vitest";
import type { DesktopUpdateSnapshot } from "../shared/desktop-update.js";
import {
  isLoopbackGenericFeed,
  isLoopbackHttpUrl,
  isUpdateSmokeRequested,
  runPackagedUpdateSmoke,
  UPDATE_SMOKE_FLAG,
  type UpdateSmokeController,
} from "./desktop-update-smoke.js";

function snapshot(partial: Partial<DesktopUpdateSnapshot>): DesktopUpdateSnapshot {
  return {
    operationId: "op",
    phase: "idle",
    currentVersion: "0.0.1",
    ...partial,
  };
}

function controller(
  overrides: Partial<UpdateSmokeController> = {},
): UpdateSmokeController {
  return {
    check: vi.fn(async () => snapshot({ phase: "available", availableVersion: "0.1.0" })),
    download: vi.fn(async () => snapshot({ phase: "downloaded", availableVersion: "0.1.0" })),
    restartAndInstall: vi.fn(async () => snapshot({ phase: "installing", availableVersion: "0.1.0" })),
    ...overrides,
  };
}

const LOOPBACK_FEED = "provider: generic\nurl: http://127.0.0.1:9/\n";

describe("desktop update smoke feed gate", () => {
  it("accepts only a 127.0.0.1 generic feed", () => {
    expect(isUpdateSmokeRequested([UPDATE_SMOKE_FLAG])).toBe(true);
    expect(isUpdateSmokeRequested(["--dev"])).toBe(false);
    expect(isLoopbackHttpUrl("http://127.0.0.1:4312/feed")).toBe(true);
    expect(isLoopbackHttpUrl("http://127.0.0.1/")).toBe(true);
    expect(isLoopbackHttpUrl("https://127.0.0.1/")).toBe(false);
    expect(isLoopbackHttpUrl("http://localhost/")).toBe(false);
    expect(isLoopbackHttpUrl("http://127.0.0.1.example/")).toBe(false);
    expect(isLoopbackHttpUrl("http://user:pass@127.0.0.1/")).toBe(false);
    expect(isLoopbackGenericFeed(LOOPBACK_FEED)).toBe(true);
    expect(isLoopbackGenericFeed('provider: generic\nurl: "http://127.0.0.1:9/"\n')).toBe(true);
    expect(
      isLoopbackGenericFeed("provider: github\nowner: NolanGrant556\nrepo: Awaken\nurl: http://127.0.0.1:9/\n"),
    ).toBe(false);
  });
});

describe("runPackagedUpdateSmoke", () => {
  it("does not check when the packaged feed is the production GitHub provider", async () => {
    const calls = controller();
    const lines: string[] = [];
    const result = await runPackagedUpdateSmoke({
      feedText: "provider: github\nowner: NolanGrant556\nrepo: Awaken\n",
      install: true,
      controller: calls,
      log: (line) => lines.push(line),
    });
    expect(result).toBe("ignored");
    expect(lines).toEqual(["desktop_update_smoke_ignored"]);
    expect(calls.check).not.toHaveBeenCalled();
    expect(calls.download).not.toHaveBeenCalled();
    expect(calls.restartAndInstall).not.toHaveBeenCalled();
  });

  it("records defer before restarting, and waits out active runs", async () => {
    let installs = 0;
    const calls = controller({
      restartAndInstall: vi.fn(async () => {
        installs += 1;
        if (installs === 1) {
          return snapshot({ phase: "downloaded", errorCode: "active_runs", availableVersion: "0.1.0" });
        }
        return snapshot({ phase: "installing", availableVersion: "0.1.0" });
      }),
    });
    const lines: string[] = [];
    let now = 0;
    const result = await runPackagedUpdateSmoke({
      feedText: LOOPBACK_FEED,
      install: true,
      controller: calls,
      log: (line) => lines.push(line),
      now: () => now,
      sleep: async (ms) => {
        now += ms;
      },
      deadlineMs: 5_000,
    });
    expect(result).toBe("ok");
    expect(lines).toEqual([
      "desktop_update_smoke_available 0.1.0",
      "desktop_update_smoke_deferred 0.1.0",
      "desktop_update_smoke_installing 0.1.0",
    ]);
    expect(installs).toBe(2);
    expect(lines.indexOf("desktop_update_smoke_deferred 0.1.0")).toBeLessThan(
      lines.indexOf("desktop_update_smoke_installing 0.1.0"),
    );
  });

  it("stops when active runs never clear", async () => {
    const lines: string[] = [];
    let now = 0;
    const result = await runPackagedUpdateSmoke({
      feedText: LOOPBACK_FEED,
      install: true,
      controller: controller({
        restartAndInstall: async () =>
          snapshot({ phase: "downloaded", errorCode: "active_runs", availableVersion: "0.1.0" }),
      }),
      log: (line) => lines.push(line),
      now: () => now,
      sleep: async (ms) => {
        now += ms;
      },
      deadlineMs: 1_000,
    });
    expect(result).toBe("failed");
    expect(lines.at(-1)).toBe("desktop_update_smoke_failed install active_runs");
  });

  it("can defer without installing", async () => {
    const calls = controller();
    const result = await runPackagedUpdateSmoke({
      feedText: LOOPBACK_FEED,
      install: false,
      controller: calls,
      log: () => undefined,
    });
    expect(result).toBe("ok");
    expect(calls.restartAndInstall).not.toHaveBeenCalled();
  });
});
