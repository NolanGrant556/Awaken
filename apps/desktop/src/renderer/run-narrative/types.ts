export type RunNarrativeStatus =
  | "active"
  | "done"
  | "error"
  | "waiting"
  | "approved"
  | "rejected";

export type RunAction = {
  id: string;
  title: string;
  detail?: string;
  status: "active" | "done" | "error";
};

export type ChangedFile = {
  id: string;
  path?: string;
  title: string;
  description?: string;
  status: "active" | "done" | "error";
};

export type InspectorSelection =
  | { kind: "runtime"; id?: string }
  | { kind: "tool"; id: string }
  | { kind: "approval"; id: string }
  | { kind: "artifact"; id: string }
  | { kind: "fileChange"; id: string };

export type RunNarrativeItem =
  | { kind: "prompt"; id: string; text: string }
  | { kind: "planning"; id: string; text: string; status: "active" | "done" }
  | { kind: "thinking"; id: string; text: string; status: "active" | "done" }
  | { kind: "exploring"; id: string; actions: RunAction[]; status: "active" | "done" }
  | { kind: "tool"; id: string; title: string; detail?: string; status: "active" | "done" | "error" }
  | { kind: "approval"; id: string; title: string; status: "waiting" | "approved" | "rejected" }
  | { kind: "fileChange"; id: string; files: ChangedFile[]; status: "active" | "done" }
  | {
      kind: "answer";
      id: string;
      markdown: string;
      status: "active" | "done" | "error";
      isFinal?: boolean;
      elapsedMs?: number;
    };

export type RunNarrativeState = {
  committedItems: RunNarrativeItem[];
  activeItem?: RunNarrativeItem;
  inspectorSelection?: InspectorSelection;
  seenEventKeys: string[];
};
