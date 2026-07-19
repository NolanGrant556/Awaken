import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { MemoryDreamSchedulerStore } from "./memory-dream-scheduler-store.js";

describe("MemoryDreamSchedulerStore", () => {
  it("requires activity and 24 hours after a successful Dream", async () => {
    const root = await mkdtemp(join(tmpdir(), "awaken-dream-"));
    const store = new MemoryDreamSchedulerStore(root);
    await store.initialize();
    for (let i = 0; i < 5; i++) store.recordCompletedRun(`run-${i}`);
    store.recordCompletedRun("run-4");
    expect(store.eligible()).toBe(true);
    store.recordSuccessfulDream(new Date("2026-07-18T00:00:00Z"));
    for (let i = 0; i < 20; i++) store.recordMemoryMutation(`mutation-${i}`);
    expect(store.eligible(Date.parse("2026-07-18T23:59:59Z"))).toBe(false);
    expect(store.eligible(Date.parse("2026-07-19T00:00:00Z"))).toBe(true);
  });
});
