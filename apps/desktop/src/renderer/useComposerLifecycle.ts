import { useCallback, useMemo, useState } from "react";
import type { Dispatch, SetStateAction } from "react";

export type ComposerSubmitMode = "send" | "stop";

export type ComposerRuntimePanel = {
  id: string;
  title: string;
  message: string;
  tone?: "error";
};

type ComposerRuntimeStatus = {
  desktopNeedsRestart: boolean;
  desktopStarting: boolean;
  runtimeErrorMessage?: string | null;
};

type ComposerLifecycleOptions = ComposerRuntimeStatus & {
  desktopReady: boolean;
  isBusy: boolean;
  isWorkspaceValid: boolean;
  hasAttachments: boolean;
  inputAttachmentBlocked?: boolean;
};

type ComposerBarLifecycleState = {
  disabled: boolean;
  isReady: boolean;
  submitMode: ComposerSubmitMode;
  submitDisabled: boolean;
};

type ComposerLifecycle = {
  composerValue: string;
  setComposerValue: Dispatch<SetStateAction<string>>;
  clearComposerValue: () => void;
  restoreComposerValue: (value: string) => void;
  trimmedComposerValue: string;
  composerFocusRequest: number;
  requestComposerFocus: () => void;
  composerBarState: ComposerBarLifecycleState;
  composerRuntimePanels: ComposerRuntimePanel[];
};

export function composerRuntimePanelsForStatus({
  desktopNeedsRestart,
  desktopStarting,
  runtimeErrorMessage,
}: ComposerRuntimeStatus): ComposerRuntimePanel[] {
  const panels: ComposerRuntimePanel[] = [];

  if (desktopNeedsRestart) {
    panels.push({
      id: "restart",
      title: "Awaken needs restart",
      message: "Quit Awaken and open it again.",
      tone: "error",
    });
  }
  if (desktopStarting) {
    panels.push({
      id: "starting",
      title: "Starting Awaken",
      message: "Preparing the desktop app.",
    });
  }
  if (runtimeErrorMessage) {
    panels.push({
      id: "run-error",
      title: "Run error",
      message: runtimeErrorMessage,
      tone: "error",
    });
  }

  return panels;
}

export function useComposerLifecycle({
  desktopReady,
  desktopStarting,
  desktopNeedsRestart,
  isBusy,
  isWorkspaceValid,
  hasAttachments,
  inputAttachmentBlocked = false,
  runtimeErrorMessage,
}: ComposerLifecycleOptions): ComposerLifecycle {
  const [composerValue, setComposerValue] = useState("");
  const [composerFocusRequest, setComposerFocusRequest] = useState(0);
  const trimmedComposerValue = composerValue.trim();

  const requestComposerFocus = useCallback(() => {
    setComposerFocusRequest((request) => request + 1);
  }, []);

  const clearComposerValue = useCallback(() => {
    setComposerValue("");
  }, []);

  const restoreComposerValue = useCallback((value: string) => {
    setComposerValue(value);
  }, []);

  const composerBarState = useMemo<ComposerBarLifecycleState>(() => {
    const hasDraftInput = trimmedComposerValue.length > 0 || hasAttachments;
    const canSend =
      desktopReady &&
      hasDraftInput &&
      isWorkspaceValid &&
      !inputAttachmentBlocked;
    const submitMode = isBusy ? "stop" : "send";
    return {
      disabled: !desktopReady || isBusy || !isWorkspaceValid,
      isReady: isBusy || canSend,
      submitMode,
      submitDisabled: submitMode === "stop" ? false : !canSend,
    };
  }, [
    desktopReady,
    hasAttachments,
    inputAttachmentBlocked,
    isBusy,
    isWorkspaceValid,
    trimmedComposerValue,
  ]);

  const composerRuntimePanels = useMemo(
    () =>
      composerRuntimePanelsForStatus({
        desktopNeedsRestart,
        desktopStarting,
        runtimeErrorMessage,
      }),
    [desktopNeedsRestart, desktopStarting, runtimeErrorMessage],
  );

  return {
    composerValue,
    setComposerValue,
    clearComposerValue,
    restoreComposerValue,
    trimmedComposerValue,
    composerFocusRequest,
    requestComposerFocus,
    composerBarState,
    composerRuntimePanels,
  };
}
