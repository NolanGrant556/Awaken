import type { ComposerToken, RunInputAttachment } from "../../shared/runtime";
import type { ToolDisplayState } from "../../shared/toolStatus";

export type { ToolDisplayState };

export type RunNarrativeStatus =
  | "active"
  | "done"
  | "error"
  | "waiting"
  | "approved"
  | "rejected"
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
  | "powerpoint";

export type DeliverableFile = {
  artifactId: string;
  runId: string;
  title: string;
  displayPath: string;
  fileType: DeliverableFileType;
  extension: string;
  legacy: boolean;
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
  | { kind: "approval"; id: string }
  | { kind: "artifact"; id: string }
  | { kind: "fileChange"; id: string }
  | { kind: "deliverables"; id: string };

export type RunNarrativeItem =
  | {
      kind: "prompt";
      id: string;
      text: string;
      attachments?: RunInputAttachment[];
      tokens?: PromptNarrativeToken[];
    }
  | { kind: "planning"; id: string; text: string; status: "active" | "done" }
  | { kind: "thinking"; id: string; text: string; status: "active" | "done" }
  | { kind: "exploring"; id: string; actions: RunAction[]; status: "active" | "done" }
  | { kind: "tool"; id: string; title: string; detail?: string; status: ToolDisplayState; toolStatus?: string }
  | { kind: "approval"; id: string; title: string; status: "waiting" | "approved" | "rejected" }
  | { kind: "fileChange"; id: string; files: ChangedFile[]; status: "active" | "done" }
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
  | { kind: "hint"; id: string; text: string };

export type RunNarrativeState = {
  committedItems: RunNarrativeItem[];
  activeItem?: RunNarrativeItem;
  inspectorSelection?: InspectorSelection;
  seenEventKeys: string[];
};
