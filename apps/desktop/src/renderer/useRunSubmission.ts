import type {
  ReasoningEffortLevel,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  StartRunRequest,
} from "../shared/runtime";
import { runtimeRequestIssue } from "../shared/runtime-request";
import {
  type ExpandedFileComposerIdleMode,
  type ExpandedFileComposerTrackedRun,
} from "./ExpandedFileComposer";
import { toRunInputAttachment } from "./composerAttachments";
import type { RunSubmissionScope } from "./projectRunScope";
import {
  composerThreadLabel,
  effectivePromptForComposerDraft,
  promptNarrativeTokensForComposer,
} from "./promptPresentation";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction, RuntimeViewState } from "./runtime/reducer";
import type { ThreadSession } from "./runtime/thread-session";
import {
  type ThreadComposerDraft
} from "./useThreadComposerDraft";
type Options = {
  runtimeClient: RuntimeClient;
  threadSession: ThreadSession;
  dispatch: (action: RuntimeAction) => void;
  runtimeState: Pick<RuntimeViewState, "activeThreadId" | "runId">;
  desktopReady: boolean;
  desktopNeedsRestart: boolean;
  isBusy: boolean;
  getSubmissionScope: () => RunSubmissionScope;
  onRunStarted: (projectId?: string) => void;
  draft: {
    draft: ThreadComposerDraft;
    clearForSubmission: (ownerKey?: string) => (draft: ThreadComposerDraft) => void;
  };
  composerDraftKey: string;
  selectedModelProfile?: RuntimeModelProfileSummary;
  selectedReasoningEffort?: ReasoningEffortLevel;
  fileWorkspaceSurfaceActive: boolean;
  setExpandedFileComposerTrackedRun: (
    run: ExpandedFileComposerTrackedRun | undefined,
  ) => void;
  setExpandedFileComposerIdleMode: (
    mode: ExpandedFileComposerIdleMode,
  ) => void;
  prepareForRunStart: () => void;
  refreshThreads: () => Promise<void>;
  startThreadTitlePoll: (threadId: string) => void;
  setNotice: (message: string | null) => void;
};
export function useRunSubmission(options: Options) {
  const {
    runtimeClient,
    getSubmissionScope,
    onRunStarted,
    threadSession,
    dispatch,
    runtimeState,
    desktopReady,
    desktopNeedsRestart,
    isBusy,
    composerDraftKey,
    selectedModelProfile,
    selectedReasoningEffort,
    fileWorkspaceSurfaceActive,
    setExpandedFileComposerTrackedRun,
    setExpandedFileComposerIdleMode,
    prepareForRunStart,
    refreshThreads,
    startThreadTitlePoll,
    setNotice,
  } = options;
  const {
    draft,
    clearForSubmission,
  } = options.draft;
  const {
    value: composerValue,
    attachments: composerAttachments,
    tokens: composerTokens,
    comments: composerComments,
    tokenPlacements: composerTokenPlacements,
    tokenLeadingTexts: composerTokenLeadingTexts,
    selection: composerInputSelection,
  } = draft;
  const trimmedEffectiveComposerValue = effectivePromptForComposerDraft(draft);

  async function handleSubmitWithPrompt(options?: {
    prompt?: string;
    forceNewThread?: boolean;
    draft?: ThreadComposerDraft;
    restoreCanonicalOnFailure?: boolean;
    replaceTargetRunId?: string;
    onReplacementCommitted?: () => void;
  }): Promise<void> {
    if (!desktopReady) {
      setNotice(
        desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting",
      );
      return;
    }
    const sourceDraft = options?.draft;
    const prompt = (
      options?.prompt ??
      (sourceDraft
        ? effectivePromptForComposerDraft(sourceDraft)
        : trimmedEffectiveComposerValue)
    ).trim();
    const forceNewThread = options?.forceNewThread === true;
    const draftComposerValue = sourceDraft?.value ?? composerValue;
    const draftAttachments = (
      sourceDraft?.attachments ?? composerAttachments
    ).map((attachment) => ({ ...attachment }));
    const draftComposerTokens = (sourceDraft?.tokens ?? composerTokens).map(
      (token) => ({
        ...token,
        payload: token.payload ? { ...token.payload } : undefined,
      }),
    );
    const draftFileComments = (sourceDraft?.comments ?? composerComments).map(
      (comment) => ({ ...comment }),
    );
    const draftComposerTokenPlacements = {
      ...(sourceDraft?.tokenPlacements ?? composerTokenPlacements),
    };
    const draftComposerTokenLeadingTexts = {
      ...(sourceDraft?.tokenLeadingTexts ?? composerTokenLeadingTexts),
    };
    const draftComposerState: ThreadComposerDraft = {
      value: draftComposerValue,
      attachments: draftAttachments,
      tokens: draftComposerTokens,
      comments: draftFileComments,
      tokenPlacements: draftComposerTokenPlacements,
      tokenLeadingTexts: draftComposerTokenLeadingTexts,
      selection:
        (sourceDraft?.selection ?? composerInputSelection)
          ? {
              anchor: (sourceDraft?.selection ?? composerInputSelection)!
                .anchor,
              focus: (sourceDraft?.selection ?? composerInputSelection)!.focus,
            }
          : undefined,
    };
    const draftPromptTokens = promptNarrativeTokensForComposer(
      draftComposerTokens,
      draftComposerTokenLeadingTexts,
    );
    const runInputAttachments = draftAttachments.map(toRunInputAttachment);
    if (
      !prompt &&
      runInputAttachments.length === 0 &&
      draftComposerTokens.length === 0 &&
      draftFileComments.length === 0
    ) {
      setNotice("请输入内容、添加附件或文件评论");
      return;
    }
    const hasDeepResearchToken = draftComposerTokens.some(
      (token) =>
        token.type === "workflow" &&
        String(token.payload?.workflowId ?? "") === "deep-research",
    );
    if (hasDeepResearchToken && !prompt) {
      setNotice("请输入研究问题");
      return;
    }
    if (isBusy && !forceNewThread) {
      return;
    }
    const scope = getSubmissionScope();
    if (!scope.valid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const submissionGeneration = threadSession.currentGeneration;
    const { workspaceRoot } = scope.workspace;
    // An empty root means the selected project has no folder bound, so there is
    // no directory to validate; Core resolves its session workspace instead.
    if (workspaceRoot) {
      const validation = await runtimeClient.validateWorkspace(workspaceRoot);
      if (!threadSession.isCurrent(submissionGeneration)) return;
      if (!validation.valid) {
        setNotice(validation.error);
        return;
      }
    }
    if (!threadSession.isCurrent(submissionGeneration)) return;
    const trackExpandedFileRun = fileWorkspaceSurfaceActive;
    setExpandedFileComposerTrackedRun(undefined);
    if (trackExpandedFileRun) {
      setExpandedFileComposerIdleMode("compact");
    }
    const loadSeq = threadSession.begin(runtimeState.activeThreadId);
    setNotice(null);
    prepareForRunStart();
    if (options?.replaceTargetRunId) {
      dispatch({
        type: "run/promptReplaced",
        targetRunId: options.replaceTargetRunId,
        prompt,
        promptAttachments: forceNewThread ? [] : runInputAttachments,
        promptTokens: forceNewThread ? [] : draftPromptTokens,
        promptFileComments: forceNewThread ? [] : draftFileComments,
        startedAtMs: Date.now(),
      });
    } else {
      dispatch({
        type: "run/requested",
        prompt,
        promptAttachments: forceNewThread ? [] : runInputAttachments,
        promptTokens: forceNewThread ? [] : draftPromptTokens,
        promptFileComments: forceNewThread ? [] : draftFileComments,
        startedAtMs: Date.now(),
      });
    }
    options?.onReplacementCommitted?.();
    const restoreSubmissionDraft = clearForSubmission(composerDraftKey);

    try {
      const runProjectId = scope.projectId;
      const isNewThread = forceNewThread || !runtimeState.activeThreadId;
      const nextThreadLabel = isNewThread
        ? composerThreadLabel(prompt, draftAttachments, draftComposerTokens)
        : "";
      const attachedFiles = runInputAttachments
        .filter((attachment) => attachment.kind === "file" && attachment.path)
        .map((attachment) => ({
          path: attachment.path ?? "",
          displayName: attachment.displayName,
        }));
      const startRunRequest: StartRunRequest = {
        input: prompt,
        attachments: forceNewThread ? [] : runInputAttachments,
        composerTokens: forceNewThread ? [] : draftComposerTokens,
        fileComments: forceNewThread ? [] : draftFileComments,
        threadId: forceNewThread
          ? undefined
          : runtimeState.activeThreadId || undefined,
        modelProfileId: selectedModelProfile?.id,
        reasoningEffort: selectedReasoningEffort,
        workspace: {
          ...scope.workspace,
          attachedFiles: attachedFiles.length > 0 ? attachedFiles : undefined,
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      if (!threadSession.isCurrent(loadSeq)) {
        void refreshThreads();
        if (isNewThread) startThreadTitlePoll(run.threadId);
        return;
      }
      if (trackExpandedFileRun) {
        setExpandedFileComposerTrackedRun({
          threadId: run.threadId,
          runId: run.runId,
        });
      }
      onRunStarted(runProjectId);
      dispatch({ type: "run/created", run });
      dispatch({
        type: "run/threadAdded",
        threadId: run.threadId,
        label: nextThreadLabel,
        runId: run.runId,
        projectId: runProjectId,
        workspaceRoot,
      });
      if (isNewThread) {
        startThreadTitlePoll(run.threadId);
      }
      void refreshThreads();
      await threadSession.subscribe(run, loadSeq);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create run";
      const issue = error as Error & { code?: string; commentId?: string };
      const restoredComposerState =
        issue.code === "file_comment_stale"
          ? {
              ...draftComposerState,
              comments: draftComposerState.comments.map((comment) =>
                !issue.commentId || comment.id === issue.commentId
                  ? { ...comment, validationState: "stale" as const }
                  : comment,
              ),
            }
          : draftComposerState;
      if (trackExpandedFileRun) {
        setExpandedFileComposerIdleMode("full");
      }
      restoreSubmissionDraft(restoredComposerState);
      if (!threadSession.isCurrent(loadSeq)) return;
      if (options?.restoreCanonicalOnFailure && runtimeState.activeThreadId) {
        try {
          await threadSession.replay(runtimeState.activeThreadId, loadSeq);
        } catch {
          if (threadSession.isCurrent(loadSeq))
            dispatch({ type: "run/createFailed", message });
        }
        return;
      }
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleCancel(): Promise<void> {
    if (!runtimeState.runId || !isBusy) {
      return;
    }

    const generation = threadSession.currentGeneration;
    dispatch({ type: "run/cancelRequested" });

    try {
      const run = await runtimeClient.cancelRun(runtimeState.runId);
      if (!threadSession.isCurrent(generation)) return;
      dispatch({ type: "run/cancelSync", run });
      const terminalStatuses: RuntimeRunStatus[] = [
        "finished",
        "error",
        "cancelled",
      ];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      if (!threadSession.isCurrent(generation)) return;
      const message =
        error instanceof Error ? error.message : "Failed to cancel run";
      if (runtimeRequestIssue(error).code === "RUN_NOT_FOUND") {
        // The run no longer exists on the server (e.g. runtime restarted or
        // the run already terminated). Reset the local runtime state so the
        // user can start fresh instead of showing a confusing error.
        localStorage.removeItem("awaken:runId");
        dispatch({ type: "run/reset" });
        return;
      }
      dispatch({
        type: "run/streamFailed",
        message,
      });
    }
  }
  return { handleSubmitWithPrompt, handleCancel };
}
