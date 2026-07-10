import { describe, expect, it } from "vitest";
import { buildNotifications, notificationKeyForSession } from "./pet-notifications";
import type { PetSessionActivity } from "../../shared/pets";

describe("buildNotifications", () => {
  it("excludes idle sessions", () => {
    const snapshot = {
      sessions: [session({ status: "idle" })],
      emittedAtMs: Date.now(),
    };
    expect(buildNotifications(snapshot, new Set())).toHaveLength(0);
  });

  it("includes running sessions with stop action", () => {
    const snapshot = {
      sessions: [session({ status: "running", runId: "run-1" })],
      emittedAtMs: Date.now(),
    };
    const notifications = buildNotifications(snapshot, new Set());
    expect(notifications).toHaveLength(1);
    expect(notifications[0]?.level).toBe("loading");
    expect(notifications[0]?.actions[0]?.request).toEqual({
      type: "stop-run",
      runId: "run-1",
    });
  });

  it("prioritizes waiting over failed over review over running", () => {
    const snapshot = {
      sessions: [
        session({ key: "running", status: "running" }),
        session({ key: "review", status: "review" }),
        session({ key: "failed", status: "failed" }),
        session({ key: "waiting", status: "waiting" }),
      ],
      emittedAtMs: Date.now(),
    };
    const notifications = buildNotifications(snapshot, new Set());
    expect(notifications.map((n) => n.status)).toEqual([
      "waiting",
      "failed",
      "review",
      "running",
    ]);
  });

  it("filters expired notifications", () => {
    const now = Date.now();
    const snapshot = {
      sessions: [
        session({
          key: "old-running",
          status: "running",
          updatedAtMs: now - 10 * 60 * 1000,
        }),
      ],
      emittedAtMs: now,
    };
    expect(buildNotifications(snapshot, new Set(), now)).toHaveLength(0);
  });

  it("expires completed review notifications quickly", () => {
    const now = Date.now();
    const snapshot = {
      sessions: [
        session({
          key: "old-review",
          status: "review",
          updatedAtMs: now - 20 * 1000,
        }),
      ],
      emittedAtMs: now,
    };
    expect(buildNotifications(snapshot, new Set(), now)).toHaveLength(0);
  });

  it("filters dismissed keys", () => {
    const s = session({ status: "failed" });
    const snapshot = { sessions: [s], emittedAtMs: Date.now() };
    const dismissed = new Set([notificationKeyForSession(s)]);
    expect(buildNotifications(snapshot, dismissed)).toHaveLength(0);
  });
});

function session(overrides: Partial<PetSessionActivity> = {}): PetSessionActivity {
  return {
    key: "thread-1",
    status: "running",
    title: "Test",
    updatedAtMs: Date.now(),
    ...overrides,
  };
}
