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
  agentType: "explore" | "review" | "general";
  status: "queued" | "running" | "completed" | "failed" | "cancelling" | "cancelled" | "interrupted";
  activity?: string;
  resultPreview?: string;
  errorCode?: string;
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
      phase?: "continuation";
    }
  | { kind: "thinking"; id: string; text: string; status: "active" | "done" }
  | { kind: "exploring"; id: string; actions: RunAction[]; status: "active" | "done" }
  | { kind: "tool"; id: string; title: string; detail?: string; status: ToolDisplayState; toolStatus?: string }
  | {
      kind: "skill";
      id: string;
      title: string;
      detail?: string;
      output?: string;
      status: "active" | "done" | "error";
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
};
