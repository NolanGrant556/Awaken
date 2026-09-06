export const COMPUTER_OVERLAY_CHANNELS = {
  getState: "awaken:computer-overlay-get-state",
  stateChanged: "awaken:computer-overlay-state-changed",
  stop: "awaken:computer-overlay-stop",
} as const;

export const COMPUTER_CURSOR_CHANNELS = {
  hintChanged: "awaken:computer-cursor-hint-changed",
} as const;

export type ComputerOverlayState = {
  runId: string;
  stopping: boolean;
};

export type ComputerCursorHint = {
  kind: string;
  pulse: boolean;
};

export type ComputerOverlayStopResult = {
  ok: boolean;
  message?: string;
};

export type ComputerOverlayBridge = {
  getState(): Promise<ComputerOverlayState | null>;
  onStateChanged(callback: (state: ComputerOverlayState | null) => void): () => void;
  stop(): Promise<ComputerOverlayStopResult>;
};

export type ComputerCursorBridge = {
  onHintChanged(callback: (hint: ComputerCursorHint | null) => void): () => void;
};
