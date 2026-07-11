import { describe, expect, it, vi } from "vitest";
import type { RuntimeProjectOperationWire } from "../shared/runtime.js";
import {
  executeProjectTrashOperation,
  type ProjectTrashWorkerDependencies,
} from "./project-trash-worker.js";

function operation(
  overrides: Partial<RuntimeProjectOperationWire> = {},
): RuntimeProjectOperationWire {
  return {
    operation_id: "operation-1",
    idempotency_key: "request-1",
    project_id: "project-1",
    action: "trash_and_purge",
    status: "running",
    phase: "trash_claimed",
    workspace_root: "/tmp/project",
    path_fingerprint: "/tmp/project:1:2",
    attempt_id: "attempt-1",
    matched_threads: 1,
    pending_cleanup: 0,
    created_at: new Date(0).toISOString(),
    updated_at: new Date(0).toISOString(),
    ...overrides,
  };
}

function dependencies(
  claimed: RuntimeProjectOperationWire | null,
  stagingExists = true,
): ProjectTrashWorkerDependencies & {
  claim: ReturnType<typeof vi.fn>;
  stage: ReturnType<typeof vi.fn>;
  pathExists: ReturnType<typeof vi.fn>;
  beginInvocation: ReturnType<typeof vi.fn>;
  trash: ReturnType<typeof vi.fn>;
  complete: ReturnType<typeof vi.fn>;
  fail: ReturnType<typeof vi.fn>;
} {
  return {
    claim: vi.fn(async () => claimed),
    stage: vi.fn(async () => ({
      ...claimed!,
      phase: claimed?.phase === "trashing" ? "trashing" : "trash_staged",
      trash_staging_path: "/tmp/.awaken-trash-operation-1/project",
    })),
    pathExists: vi.fn(async () => stagingExists),
    beginInvocation: vi.fn(async () => undefined),
    trash: vi.fn(async () => undefined),
    complete: vi.fn(async () => undefined),
    fail: vi.fn(async () => undefined),
  };
}

describe("project trash worker", () => {
  it("always claims a lease, including recovery from the trashing phase", async () => {
    const deps = dependencies(null);

    await executeProjectTrashOperation(
      operation({ phase: "trashing", trash_invoked_at: new Date().toISOString() }),
      deps,
    );

    expect(deps.claim).toHaveBeenCalledOnce();
    expect(deps.claim).toHaveBeenCalledWith("operation-1");
    expect(deps.trash).not.toHaveBeenCalled();
    expect(deps.complete).not.toHaveBeenCalled();
  });

  it("preserves project history when staging detects a path replacement", async () => {
    const claimed = operation({ phase: "trash_claimed", trash_invoked_at: undefined });
    const deps = dependencies(claimed);
    deps.stage.mockRejectedValue({ code: "PROJECT_PATH_FINGERPRINT_CHANGED" });

    await executeProjectTrashOperation(operation(), deps);

    expect(deps.fail).toHaveBeenCalledWith(
      "operation-1",
      "attempt-1",
      "PROJECT_PATH_FINGERPRINT_CHANGED",
      expect.any(String),
    );
    expect(deps.beginInvocation).not.toHaveBeenCalled();
    expect(deps.complete).not.toHaveBeenCalled();
    expect(deps.trash).not.toHaveBeenCalled();
  });

  it("completes missing-source recovery only after the durable invocation boundary", async () => {
    const claimed = operation({
      phase: "trashing",
      trash_invoked_at: new Date().toISOString(),
    });
    const deps = dependencies(claimed, false);

    await executeProjectTrashOperation(operation(), deps);

    expect(deps.complete).toHaveBeenCalledWith("operation-1", "attempt-1");
    expect(deps.fail).not.toHaveBeenCalled();
    expect(deps.trash).not.toHaveBeenCalled();
  });

  it("passes the current attempt through attest, invocation, trash, and complete", async () => {
    const calls: string[] = [];
    const claimed = operation({ attempt_id: "attempt-current" });
    const deps = dependencies(claimed);
    deps.claim.mockImplementation(async () => {
      calls.push("claim");
      return claimed;
    });
    deps.stage.mockImplementation(async (_operationId, attemptId) => {
      calls.push(`stage:${attemptId}`);
      return {
        ...claimed,
        phase: "trash_staged",
        trash_staging_path: "/tmp/.awaken-trash-operation-1/project",
      };
    });
    deps.beginInvocation.mockImplementation(async (_operationId, attemptId) => {
      calls.push(`begin:${attemptId}`);
    });
    deps.trash.mockImplementation(async () => {
      calls.push("trash");
    });
    deps.complete.mockImplementation(async (_operationId, attemptId) => {
      calls.push(`complete:${attemptId}`);
    });

    await executeProjectTrashOperation(operation(), deps);

    expect(calls).toEqual([
      "claim",
      "stage:attempt-current",
      "begin:attempt-current",
      "trash",
      "complete:attempt-current",
    ]);
    expect(deps.fail).not.toHaveBeenCalled();
    expect(deps.trash).toHaveBeenCalledWith(
      "/tmp/.awaken-trash-operation-1/project",
    );
  });

  it("never calls the system trash API when the staged path vanished before invocation", async () => {
    const claimed = operation();
    const deps = dependencies(claimed, false);

    await executeProjectTrashOperation(operation(), deps);

    expect(deps.fail).toHaveBeenCalledWith(
      "operation-1",
      "attempt-1",
      "PROJECT_STAGING_MISSING_BEFORE_TRASH",
      expect.any(String),
    );
    expect(deps.beginInvocation).not.toHaveBeenCalled();
    expect(deps.trash).not.toHaveBeenCalled();
  });
});
