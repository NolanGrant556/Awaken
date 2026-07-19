import type { MemoryDreamTask } from "../shared/runtime";

export type DreamReceiptThreadAssignments = Record<string, string>;

const MAX_DREAM_RECEIPT_THREAD_ASSIGNMENTS = 100;

export function parseDreamReceiptThreadAssignments(
  raw: string,
): DreamReceiptThreadAssignments {
  if (!raw.trim()) return {};
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const entries = Object.entries(parsed).filter(
      ([taskId, threadId]) =>
        taskId.trim().length > 0 &&
        typeof threadId === "string" &&
        threadId.trim().length > 0,
    );
    return Object.fromEntries(
      entries.slice(-MAX_DREAM_RECEIPT_THREAD_ASSIGNMENTS),
    );
  } catch {
    return {};
  }
}

export function assignDreamReceiptToThread(
  current: DreamReceiptThreadAssignments,
  taskId: string,
  threadId: string,
): DreamReceiptThreadAssignments {
  const entries = Object.entries(current).filter(([id]) => id !== taskId);
  entries.push([taskId, threadId]);
  return Object.fromEntries(
    entries.slice(-MAX_DREAM_RECEIPT_THREAD_ASSIGNMENTS),
  );
}

export function dreamTasksForThreadReplay(
  tasks: MemoryDreamTask[],
  threadId: string,
  assignments: DreamReceiptThreadAssignments,
): MemoryDreamTask[] {
  return tasks.filter((task) => assignments[task.taskId] === threadId);
}
