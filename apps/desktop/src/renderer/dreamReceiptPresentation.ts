import type { MemoryDreamTask } from "../shared/runtime";
import type { AguiEvent } from "../shared/agui";

export const DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY =
  "awaken:memory:dreamReceiptThreadAssignments";

export function dreamTaskReceiptEvent(
  task: MemoryDreamTask,
): AguiEvent | undefined {
  if (
    !["completed", "failed", "cancelled", "interrupted"].includes(task.status)
  ) {
    return undefined;
  }
  const suffix = task.status.toUpperCase();
  return {
    type: `AWAKEN_MEMORY_DREAM_${suffix}` as AguiEvent["type"],
    recordId: task.taskId,
    timestamp: Date.parse(task.finishedAt ?? task.createdAt),
    errorCode: task.errorCode,
    message: task.errorMessage,
    diagnostic: { reports: task.reports ?? {}, projectId: task.projectId },
  };
}
