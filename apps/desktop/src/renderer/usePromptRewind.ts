import { useState } from "react";
import type { RewindThreadResult } from "../shared/runtime";
import {
  composerDraftFromRuntimeRun,
  promptRewindConfirmDialog,
} from "./promptPresentation";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction, RuntimeViewState } from "./runtime/reducer";
import type { ThreadSession } from "./runtime/thread-session";
import { useRunStageScrollController } from "./useRunStageScrollController";
import { useRunSubmission } from "./useRunSubmission";
import {
  composerDraftOwnerKey,
  useThreadComposerDraft,
} from "./useThreadComposerDraft";
import {
  promptRewindDialogOverlay,
  scheduleAfterNextPaint,
  waitForNextPaint,
} from "./workbenchPresentation";
type Options = {
  setPromptReplacementPending: (pending: boolean) => void;
  runtimeClient: RuntimeClient;
  threadSession: ThreadSession;
  dispatch: (action: RuntimeAction) => void;
  runtimeState: Pick<RuntimeViewState, "activeThreadId">;
  isBusy: boolean;
  narrativeSections: ReturnType<
    typeof useRunStageScrollController
  >["narrativeSections"];
  draft: ReturnType<typeof useThreadComposerDraft>;
  handleSubmitWithPrompt: ReturnType<
    typeof useRunSubmission
  >["handleSubmitWithPrompt"];
  requestComposerFocus: (offset?: number) => void;
  refreshThreads: () => Promise<void>;
  setNotice: (message: string | null) => void;
};
export function usePromptRewind(options: Options) {
  const {
    setPromptReplacementPending,
    runtimeClient,
    threadSession,
    dispatch,
    runtimeState,
    isBusy,
    narrativeSections,
    handleSubmitWithPrompt,
    requestComposerFocus,
    refreshThreads,
    setNotice,
  } = options;
  const { draft, replaceDraft: replaceComposerDraft } = options.draft;
  const {
    value: composerValue,
    attachments: composerAttachments,
    tokens: composerTokens,
    comments: composerComments,
  } = draft;

  const [promptBubbleModeByTurnId, setPromptBubbleModeByTurnId] = useState<
    Record<string, "collapsed" | "expanded">
  >({});

  const [editingPrompt, setEditingPrompt] = useState<{
    turnId: string;
    runId: string;
    text: string;
  }>();

  const [pendingPromptRewind, setPendingPromptRewind] = useState<{
    runId: string;
    mode: "undo" | "edit";
    hasDraft: boolean;
  }>();

  const [messageActionPendingRunId, setMessageActionPendingRunId] = useState<
    string | undefined
  >();

  const [rewindDialogHidden, setRewindDialogHidden] = useState(false);

  function composerHasUnsentDraft(): boolean {
    return (
      composerValue.trim().length > 0 ||
      composerAttachments.length > 0 ||
      composerTokens.length > 0 ||
      composerComments.length > 0
    );
  }

  function requestPromptRewind(runId: string, mode: "undo" | "edit"): void {
    setRewindDialogHidden(false);
    setPendingPromptRewind({
      runId,
      mode,
      hasDraft: composerHasUnsentDraft(),
    });
  }

  function dismissPromptRewindDialog(): void {
    setPendingPromptRewind(undefined);
    setRewindDialogHidden(false);
  }

  async function rewindPromptRun(
    runId: string,
    options?: { applyReplay?: boolean },
  ): Promise<RewindThreadResult | undefined> {
    const threadId = runtimeState.activeThreadId;
    const expectedHeadRunId = [...narrativeSections.turns]
      .reverse()
      .find((turn) => turn.prompt.runId)?.prompt.runId;
    if (!threadId || !expectedHeadRunId || isBusy) {
      return undefined;
    }

    const requestGeneration = threadSession.currentGeneration;
    setMessageActionPendingRunId(runId);
    setNotice(null);
    let result: RewindThreadResult;
    try {
      result = await runtimeClient.rewindThread({
        threadId,
        targetRunId: runId,
        expectedHeadRunId,
      });
    } catch (error) {
      if (!threadSession.isCurrent(requestGeneration)) return undefined;
      setNotice(
        error instanceof Error ? error.message : "撤销消息失败，请稍后重试",
      );
      setMessageActionPendingRunId(undefined);
      return undefined;
    }

    if (!threadSession.isCurrent(requestGeneration)) return undefined;
    const loadSeq = threadSession.begin(threadId);
    if (options?.applyReplay === false) {
      setMessageActionPendingRunId(undefined);
      return result;
    }
    dispatch({
      type: "run/threadReplayStarted",
      threadId,
      runId: result.thread.runId || undefined,
    });
    try {
      await threadSession.replay(threadId, loadSeq);
      void refreshThreads();
    } catch (error) {
      if (!threadSession.isCurrent(loadSeq)) return undefined;
      setNotice(
        error instanceof Error ? error.message : "撤销消息失败，请稍后重试",
      );
    } finally {
      if (threadSession.isCurrent(loadSeq))
        setMessageActionPendingRunId(undefined);
    }
    return threadSession.isCurrent(loadSeq) ? result : undefined;
  }

  async function handleUndoPrompt(runId: string): Promise<void> {
    const result = await rewindPromptRun(runId);
    if (!result) {
      return;
    }
    setEditingPrompt(undefined);
    const restoredDraft = composerDraftFromRuntimeRun(result.targetRun);
    replaceComposerDraft(
      restoredDraft,
      composerDraftOwnerKey(result.thread.threadId),
    );
    requestComposerFocus(restoredDraft.value.length);
  }

  async function handleSubmitPromptEdit(): Promise<void> {
    if (!editingPrompt) {
      return;
    }
    const targetRunId = editingPrompt.runId;
    const editedText = editingPrompt.text;
    setPromptReplacementPending(true);
    try {
      const result = await rewindPromptRun(targetRunId, {
        applyReplay: false,
      });
      if (!result) {
        return;
      }
      const editedDraft = composerDraftFromRuntimeRun(
        result.targetRun,
        editedText,
      );
      await handleSubmitWithPrompt({
        draft: editedDraft,
        restoreCanonicalOnFailure: true,
        replaceTargetRunId: targetRunId,
        onReplacementCommitted: () => {
          setEditingPrompt(undefined);
          scheduleAfterNextPaint(dismissPromptRewindDialog);
        },
      });
    } finally {
      setPromptReplacementPending(false);
    }
  }

  async function handleConfirmPromptRewind(): Promise<void> {
    const pending = pendingPromptRewind;
    if (!pending) {
      return;
    }
    if (pending.mode === "undo") {
      dismissPromptRewindDialog();
      await handleUndoPrompt(pending.runId);
      return;
    }
    setRewindDialogHidden(true);
    try {
      await waitForNextPaint();
      await handleSubmitPromptEdit();
    } finally {
      scheduleAfterNextPaint(dismissPromptRewindDialog);
    }
  }

  const promptRewindDialog = pendingPromptRewind
    ? promptRewindConfirmDialog(pendingPromptRewind)
    : undefined;

  const promptRewindOverlay = promptRewindDialogOverlay({
    pending: Boolean(pendingPromptRewind),
    hidden: rewindDialogHidden,
  });
  return {
    promptBubbleModeByTurnId,
    setPromptBubbleModeByTurnId,
    editingPrompt,
    setEditingPrompt,
    pendingPromptRewind,
    setPendingPromptRewind,
    messageActionPendingRunId,
    setMessageActionPendingRunId,
    rewindDialogHidden,
    setRewindDialogHidden,
    composerHasUnsentDraft,
    requestPromptRewind,
    dismissPromptRewindDialog,
    rewindPromptRun,
    handleUndoPrompt,
    handleSubmitPromptEdit,
    handleConfirmPromptRewind,
    promptRewindDialog,
    promptRewindOverlay,
  };
}
