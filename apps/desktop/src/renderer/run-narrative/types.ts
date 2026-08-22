import type { ComposerToken, RunInputAttachment } from "../../shared/runtime";
import type { ToolDisplayState } from "../../shared/toolStatus";

export type { ToolDisplayState };

export type RunNarrativeStatus =
  | "active"
  | "done"
  | "error"
  | "waiting"
  | "denied"
  | "cancelled"
  | "interrupted"
  | "blocked"
  | "unsupported";

export type RunAction = {
  id: string;
  /** Raw runtime identity used to join reducer-owned tool records. */
  toolCallId?: string;
  /** Stable presentation semantic derived from the canonical tool identity. */
  visualKind?: "search";
  title: string;
  detail?: string;
  status: ToolDisplayState;
  /** Canonical ToolOutcome status when known. */
  toolStatus?: string;
};

export type PromptNarrativeToken = ComposerToken & {
  leadingText?: string;
};

export type ChangedFile = {
  id: string;
  path?: string;
  title: string;
  description?: string;
  status: "active" | "done" | "error";
};

export type DeliverableFileType =
  | "word"
  | "markdown"
  | "excel"
  | "pdf"
  | "powerpoint"
  | "image"
  | "video";

export type DeliverableFile = {
  artifactId: string;
  runId: string;
  title: string;
  displayPath: string;
  fileType: DeliverableFileType;
  extension: string;
  legacy: boolean;
};

export type MediaGenerationStatus =
  | "preparing"
  | "queued"
  | "generating"
  | "downloading"
  | "ready"
  | "partial"
  | "unknown"
  | "error"
  | "cancelled";

export type MediaGenerationItem = {
  index: number;
  status: "ready" | "error" | "unknown";
  path?: string;
};

export type CompactBoundaryStatus = "started" | "finished" | "failed";

export type CompactBoundaryItem = {
  kind: "compactBoundary";
  id: string;
  status: CompactBoundaryStatus;
  title: string;
  detail?: string;
  mode?: string;
  boundaryId?: string;
  preEstimate?: number;
  postEstimate?: number;
  fallbackUsed?: boolean;
};

export type InspectorSelection =
  | { kind: "runtime"; id?: string }
  | { kind: "tool"; id: string }
  | { kind: "artifact"; id: string }
  | { kind: "fileChange"; id: string }
  | { kind: "deliverables"; id: string };

export type ACPDisplayStatus =
  | "active"
  | "done"
  | "error"
  | "interrupted"
  | "closed"
  | "started";

export type SubagentNarrativeTask = {
  taskId: string;
  description: string;
  agentType: "explore" | "plan" | "review" | "general";
  workspaceMode?: "shared_readonly" | "shared" | "git_worktree" | string;
  status: "queued" | "running" | "completed" | "failed" | "cancelling" | "cancelled" | "interrupted";
  activity?: string;
  toolUseCount: number;
  estimatedTokens?: number;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  createdAt?: string;
  startedAt?: string;
  finishedAt?: string;
  resultPreview?: string;
  errorCode?: string;
  errorMessage?: string;
};

export type RunPlanStepStatus = "pending" | "in_progress" | "completed";

export type RunPlanNarrativeStep = {
  step: string;
  activeForm: string;
  status: RunPlanStepStatus;
};

/**
 * Latest durable plan snapshot of one run as recorded by the narrative
 * projector from AWAKEN_PLAN_UPDATED frames. It is internal projection
 * bookkeeping: live frames never become turn-body items; the snapshot is
 * projected as a `runPlan` item only when the run reaches a terminal event
 * (contract: 运行计划与任务进度设计契约 §5.4/§8.2).
 */
export type RunPlanNarrativeSnapshot = {
  planId: string;
  version: number;
  explanation?: string;
  items: RunPlanNarrativeStep[];
  completedCount: number;
  totalCount: number;
};

export type RunNarrativeItem =
  | {
      kind: "prompt";
      id: string;
      text: string;
      attachments?: RunInputAttachment[];
      tokens?: PromptNarrativeToken[];
    }
  | {
      kind: "planning";
      id: string;
      text: string;
      status: "active" | "done";
      phase?: "continuation" | "compaction";
    }
  | { kind: "thinking"; id: string; text: string; status: "active" | "done" }
  | { kind: "exploring"; id: string; actions: RunAction[]; status: "active" | "done" }
  | {
      kind: "tool";
      id: string;
      /** Raw runtime identity; `id` remains the run-scoped narrative identity. */
      toolCallId?: string;
      title: string;
      detail?: string;
      status: ToolDisplayState;
      toolStatus?: string;
    }
  | {
      kind: "skill";
      id: string;
      title: string;
      detail?: string;
      output?: string;
      status: "active" | "done" | "error";
    }
  | {
      kind: "hook";
      id: string;
      title: string;
      detail?: string;
      output?: string;
      status: "active" | "done" | "error" | "blocked" | "interrupted";
    }
  | {
      kind: "mediaGeneration";
      id: string;
      toolCallId: string;
      runId?: string;
      mediaKind: "image" | "video";
      title: string;
      detail?: string;
      status: MediaGenerationStatus;
      requestedCount?: number;
      requestedAspectRatio?: number;
      taskId?: string;
      mediaItems?: MediaGenerationItem[];
      artifactPaths: string[];
    }
  | { kind: "fileChange"; id: string; files: ChangedFile[]; status: "active" | "done" }
  | { kind: "memoryReceipt"; id: string; title: string; detail?: string; status: "done" | "error" }
  | {
      kind: "acpSession";
      id: string;
      runnerId: string;
      sessionId?: string;
      status: ACPDisplayStatus;
      message?: string;
    }
  | {
      kind: "acpTool";
      id: string;
      runnerId: string;
      sessionId?: string;
      acpToolCallId?: string;
      title: string;
      detail?: string;
      status: ACPDisplayStatus;
    }
  | {
      kind: "subagentGroup";
      id: string;
      tasks: SubagentNarrativeTask[];
    }
  | {
      kind: "deliverables";
      id: string;
      runId: string;
      terminalStatus: "finished" | "error" | "cancelled";
      files: DeliverableFile[];
    }
  | {
      /**
       * Terminal historical projection of a run plan snapshot (id
       * `run-plan:<runId>:<planId>`). Inserted before the run's final answer
       * so it renders inside the turn's 执行过程; keeps the real three-state
       * step statuses of the last snapshot, never auto-completing unfinished
       * steps (contract §5.4/§8.2).
       */
      kind: "runPlan";
      id: string;
      runId: string;
      planId: string;
      version: number;
      /** `执行计划 · N/M` */
      title: string;
      explanation?: string;
      items: RunPlanNarrativeStep[];
      completedCount: number;
      totalCount: number;
    }
  | CompactBoundaryItem
  | {
      kind: "answer";
      id: string;
      markdown: string;
      status: "active" | "done" | "error";
      isFinal?: boolean;
      elapsedMs?: number;
    }
  | { kind: "hint"; id: string; text: string }
  | {
      /**
       * Live-only provider reconnect row at the current execution cursor.
       * One stable identity per phase; recovered, run terminal, and resumed
       * model output strip these items. Never journaled.
       */
      kind: "providerReconnect";
      id: string;
      phase: "waiting_for_network" | "reconnecting";
      attempt: number;
      maxAttempts: number;
      detail?: string;
      observedAtMs?: number;
      streamCursor?: number;
    }
  | {
      kind: "scheduledNotice";
      id: string;
      taskId: string;
      executionId?: string;
      taskName?: string;
      noticeContent: string;
      timestamp?: number;
    };

export type RunNarrativeState = {
  committedItems: RunNarrativeItem[];
  activeItem?: RunNarrativeItem;
  inspectorSelection?: InspectorSelection;
  seenEventKeys: string[];
  replayedItemIds: string[];
  /** Latest AWAKEN_PLAN_UPDATED snapshot per run, version-upserted. */
  runPlanSnapshotsByRunId: Record<string, RunPlanNarrativeSnapshot>;
  /**
   * Generic tool-row ids (`tool:<runId>:<toolCallId>`) of update_plan calls
   * that produced a plan_updated receipt. The typed plan projection owns
   * their user-facing row, so START/RESULT frames for these calls must not
   * (re)create a generic Tool row; failed calls without a receipt keep theirs.
   */
  runPlanReceiptToolIds: string[];
};
