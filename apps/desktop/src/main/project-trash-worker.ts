import type { RuntimeProjectOperationWire } from "../shared/runtime.js";

export type ProjectTrashWorkerDependencies = {
  claim: (operationId: string) => Promise<RuntimeProjectOperationWire | null>;
  stage: (
    operationId: string,
    attemptId: string,
  ) => Promise<RuntimeProjectOperationWire>;
  pathExists: (path: string) => Promise<boolean>;
  beginInvocation: (operationId: string, attemptId: string) => Promise<void>;
  trash: (workspaceRoot: string) => Promise<void>;
  complete: (operationId: string, attemptId: string) => Promise<void>;
  fail: (
    operationId: string,
    attemptId: string,
    code: string,
    message: string,
  ) => Promise<void>;
  reportTrashFailure?: (args: {
    operationId: string;
    workspaceRoot: string;
    error: string;
  }) => void;
};

function errorCode(error: unknown): string | undefined {
  if (!error || typeof error !== "object" || !("code" in error)) {
    return undefined;
  }
  const code = (error as { code?: unknown }).code;
  return typeof code === "string" ? code : undefined;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function executeProjectTrashOperation(
  operation: RuntimeProjectOperationWire,
  dependencies: ProjectTrashWorkerDependencies,
): Promise<void> {
  // Every execution, including crash recovery from `trashing`, must acquire a
  // fresh durable lease and attempt token. No phase may bypass ownership.
  const claimed = await dependencies.claim(operation.operation_id);
  if (!claimed) {
    return;
  }

  const workspaceRoot = claimed.workspace_root?.trim();
  const attemptId = claimed.attempt_id?.trim();
  if (!attemptId) {
    throw new Error("Claimed project trash operation is missing its attempt token.");
  }
  if (!workspaceRoot || !claimed.path_fingerprint) {
    await dependencies.fail(
      claimed.operation_id,
      attemptId,
      "PROJECT_TRASH_ATTESTATION_FAILED",
      "The durable trash operation is missing its path attestation.",
    );
    return;
  }

  let staged: RuntimeProjectOperationWire;
  try {
    staged = await dependencies.stage(claimed.operation_id, attemptId);
  } catch (error) {
    if (errorCode(error) === "PROJECT_PATH_FINGERPRINT_CHANGED") {
      await dependencies.fail(
        claimed.operation_id,
        attemptId,
        "PROJECT_PATH_FINGERPRINT_CHANGED",
        "The project path identity changed before it could be moved to trash.",
      );
      return;
    }
    throw error;
  }

  const stagingPath = staged.trash_staging_path?.trim();
  if (!stagingPath || staged.attempt_id?.trim() !== attemptId) {
    throw new Error("Staged project trash operation is missing its owned staging path.");
  }
  const stagingExists = await dependencies.pathExists(stagingPath);
  if (!stagingExists) {
    if (staged.phase === "trashing" && staged.trash_invoked_at) {
      // The operation-owned path disappearing after the durable invocation
      // boundary is the crash-recovery proof that the system trash move won.
      await dependencies.complete(staged.operation_id, attemptId);
    } else {
      await dependencies.fail(
        staged.operation_id,
        attemptId,
        "PROJECT_STAGING_MISSING_BEFORE_TRASH",
        "The operation-owned project staging path disappeared before trash invocation.",
      );
    }
    return;
  }

  await dependencies.beginInvocation(staged.operation_id, attemptId);

  try {
    await dependencies.trash(stagingPath);
  } catch (error) {
    const message = errorMessage(error);
    dependencies.reportTrashFailure?.({
      operationId: claimed.operation_id,
      workspaceRoot,
      error: message,
    });
    await dependencies.fail(
      claimed.operation_id,
      attemptId,
      "PROJECT_TRASH_FAILED",
      message,
    );
    return;
  }

  await dependencies.complete(staged.operation_id, attemptId);
}
