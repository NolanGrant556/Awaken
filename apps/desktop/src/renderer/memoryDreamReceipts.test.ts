import { describe, expect, it } from "vitest";
import type { MemoryDreamTask } from "../shared/runtime";
import {
  assignDreamReceiptToThread,
  dreamTasksForThreadReplay,
  parseDreamReceiptThreadAssignments,
} from "./memoryDreamReceipts";

function dreamTask(taskId: string): MemoryDreamTask {
  return {
    taskId,
    status: "completed",
    scopes: ["user"],
    createdAt: new Date(0).toISOString(),
  };
}

describe("memory Dream receipt thread assignments", () => {
  it("does not replay legacy or unrelated global Dream tasks into a thread", () => {
    const tasks = [dreamTask("dream_legacy"), dreamTask("dream_other")];
    const assignments = assignDreamReceiptToThread(
      {},
      "dream_other",
      "thread_other",
    );

    expect(
      dreamTasksForThreadReplay(tasks, "thread_current", assignments),
    ).toEqual([]);
  });

  it("replays a receipt only to the thread that originally received it", () => {
    const tasks = [dreamTask("dream_assigned"), dreamTask("dream_unassigned")];
    const assignments = assignDreamReceiptToThread(
      {},
      "dream_assigned",
      "thread_current",
    );

    expect(
      dreamTasksForThreadReplay(tasks, "thread_current", assignments).map(
        (task) => task.taskId,
      ),
    ).toEqual(["dream_assigned"]);
  });

  it("fails closed when the persisted presentation mapping is invalid", () => {
    expect(parseDreamReceiptThreadAssignments("not-json")).toEqual({});
    expect(
      parseDreamReceiptThreadAssignments(
        JSON.stringify({ dream_valid: "thread_1", dream_invalid: 42 }),
      ),
    ).toEqual({ dream_valid: "thread_1" });
  });
});
