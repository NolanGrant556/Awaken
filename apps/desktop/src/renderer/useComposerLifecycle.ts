import { useCallback, useMemo, useState } from "react";

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
};

type ComposerLifecycleOptions = ComposerRuntimeStatus & {
  composerValue: string;
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
  composerFocusRequest: number;
  composerFocusOffset?: number;
  requestComposerFocus: (offset?: number) => void;
  composerBarState: ComposerBarLifecycleState;
  composerRuntimePanels: ComposerRuntimePanel[];
};

export function composerRuntimePanelsForStatus({
  desktopNeedsRestart,
  desktopStarting,
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
  return panels;
}

export function useComposerLifecycle({
  composerValue,
  desktopReady,
  desktopStarting,
  desktopNeedsRestart,
  isBusy,
  isWorkspaceValid,
  hasAttachments,
  inputAttachmentBlocked = false,
}: ComposerLifecycleOptions): ComposerLifecycle {
  const [composerFocusTarget, setComposerFocusTarget] = useState<{
    request: number;
    offset?: number;
  }>({ request: 0 });
  const trimmedComposerValue = composerValue.trim();

  const requestComposerFocus = useCallback((offset?: number) => {
    setComposerFocusTarget((current) => ({
      request: current.request + 1,
      offset,
    }));
  }, []);

  const composerBarState = useMemo<ComposerBarLifecycleState>(() => {
    const hasDraftInput = trimmedComposerValue.length > 0 || hasAttachments;
    const canSend = desktopReady && hasDraftInput && isWorkspaceValid && !inputAttachmentBlocked;
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
      }),
    [desktopNeedsRestart, desktopStarting],
  );

  return {
    composerFocusRequest: composerFocusTarget.request,
    composerFocusOffset: composerFocusTarget.offset,
    requestComposerFocus,
    composerBarState,
    composerRuntimePanels,
  };
}
