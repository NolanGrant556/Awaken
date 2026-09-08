import type { JSX } from "react";
import type {
  NarrativeTurnSections,
  NarrativeTurnBodySegment,
} from "./workbenchPresentation";
import {
  AwakenExecutionFeed as ExecutionFeed,
  AwakenFileCommentSummary as FileCommentSummary,
  AwakenUserPromptBlock as UserPromptBlock,
} from "@awaken/design-system";
import type { CSSProperties } from "react";
import { RunElapsedTime } from "./RunElapsedTime";
import { promptAttachmentViews } from "./composerAttachments";
import {
  promptTextAfterTokenLeadingText,
  promptTokenViews,
} from "./promptPresentation";
import type { RunNarrativeItem } from "./run-narrative";
import type { RuntimeViewState } from "./runtime/reducer";
import { usePromptRewind } from "./usePromptRewind";
import {
  formatUserPromptTimestamp,
  narrativeTurnElapsedMs,
  projectNarrativeTurnBodySegments,
} from "./workbenchPresentation";
type Props = {
  promptActions: Pick<
    ReturnType<typeof usePromptRewind>,
    | "promptBubbleModeByTurnId"
    | "setPromptBubbleModeByTurnId"
    | "editingPrompt"
    | "setEditingPrompt"
    | "pendingPromptRewind"
    | "messageActionPendingRunId"
    | "requestPromptRewind"
    | "dismissPromptRewindDialog"
  >;
  narrativeSections: NarrativeTurnSections;
  renderNarrativeItem: (
    item: RunNarrativeItem,
    key: string,
    animateToolEntry?: boolean,
    disclosureKey?: string,
  ) => JSX.Element | null;
  runtimeState: Pick<
    RuntimeViewState,
    "activeThreadId" | "threadId" | "runStartedAtMs" | "runCompletedAtMs"
  >;
  latestPromptId: string | undefined;
  activeWorkbenchTurnId: string | undefined;
  isBusy: boolean;
  latestTurnMinHeightPx: number;
  composerPluginBrandByName: Map<string, string>;
  activeThreadSupportsMessageRewind: boolean;
  renderTurnBodySegment: (
    turnId: string,
    segment: NarrativeTurnBodySegment,
    timing: { startedAtMs?: number; completedAtMs?: number; running: boolean },
    disclosureScope: string,
  ) => (JSX.Element | null)[];
};
export function DesktopExecutionFeed({
  promptActions,
  narrativeSections,
  renderNarrativeItem,
  runtimeState,
  latestPromptId,
  activeWorkbenchTurnId,
  isBusy,
  latestTurnMinHeightPx,
  composerPluginBrandByName,
  activeThreadSupportsMessageRewind,
  renderTurnBodySegment,
}: Props) {
  const {
    promptBubbleModeByTurnId,
    setPromptBubbleModeByTurnId,
    editingPrompt,
    setEditingPrompt,
    pendingPromptRewind,
    messageActionPendingRunId,
    requestPromptRewind,
    dismissPromptRewindDialog,
  } = promptActions;
  return (
    <ExecutionFeed>
      {narrativeSections.leadItems.map((item) =>
        renderNarrativeItem(
          item,
          `lead:${item.id}`,
          true,
          `thread:${runtimeState.activeThreadId ?? runtimeState.threadId ?? "draft"}:lead:${item.id}`,
        ),
      )}
      {narrativeSections.turns.map((turn, index) => {
        const turnDisclosureScope = `thread:${runtimeState.activeThreadId ?? runtimeState.threadId ?? "draft"}:turn:${turn.id}`;
        const isLatestPrompt = turn.id === latestPromptId;
        const isActiveTurn = turn.id === activeWorkbenchTurnId;
        const settledElapsedMs = narrativeTurnElapsedMs(turn.items);
        const turnBodySegments = projectNarrativeTurnBodySegments({
          turnId: turn.id,
          items: turn.items,
          // The stable disclosure store owns the live open state.
          // Segmentation only needs the terminal boundary here.
          executionProcessOpen: false,
        });
        const hasExecutionProcess = turnBodySegments.some(
          (segment) => segment.kind === "executionProcess",
        );
        const turnTiming = {
          startedAtMs: isLatestPrompt ? runtimeState.runStartedAtMs : undefined,
          completedAtMs: isLatestPrompt
            ? runtimeState.runCompletedAtMs
            : undefined,
          running: isLatestPrompt && isBusy,
        };
        const turnStyle =
          isLatestPrompt && latestTurnMinHeightPx > 0
            ? ({
                "--aw-latest-turn-min-height": `${latestTurnMinHeightPx}px`,
              } as CSSProperties)
            : undefined;
        const visiblePromptText = promptTextAfterTokenLeadingText(
          turn.prompt.text,
          turn.prompt.tokens,
        );
        const promptRunId = turn.prompt.runId;
        const promptEditing = editingPrompt?.turnId === turn.id;

        return (
          <section
            key={turn.id}
            data-execution-turn-id={turn.id}
            className={`execution-turn-section${
              isLatestPrompt ? " execution-turn-section-latest" : ""
            }${isActiveTurn ? " execution-turn-section-active" : ""}`}
            style={turnStyle}
            aria-label={`Turn ${index + 1}`}
          >
            <div className="execution-turn-column">
              <div className="execution-turn-header">
                <UserPromptBlock
                  text={visiblePromptText}
                  timestampLabel={
                    turn.prompt.createdAtMs != null
                      ? formatUserPromptTimestamp({
                          atMs: turn.prompt.createdAtMs,
                        })
                      : undefined
                  }
                  timestampDateTime={
                    turn.prompt.createdAtMs != null
                      ? new Date(turn.prompt.createdAtMs).toISOString()
                      : undefined
                  }
                  tokens={promptTokenViews(
                    turn.prompt.tokens,
                    composerPluginBrandByName,
                  )}
                  attachments={promptAttachmentViews(turn.prompt.attachments)}
                  structuredContent={
                    <>
                      <FileCommentSummary
                        comments={(turn.prompt.fileComments ?? []).filter(
                          (item) => item.kind !== "selection",
                        )}
                        className="execution-turn-file-comments"
                      />
                      <FileCommentSummary
                        comments={(turn.prompt.fileComments ?? []).filter(
                          (item) => item.kind === "selection",
                        )}
                        className="execution-turn-file-comments"
                      />
                    </>
                  }
                  expanded={promptBubbleModeByTurnId[turn.id] === "expanded"}
                  onExpandedChange={(expanded) => {
                    setPromptBubbleModeByTurnId((current) => ({
                      ...current,
                      [turn.id]: expanded ? "expanded" : "collapsed",
                    }));
                  }}
                  onUndo={
                    activeThreadSupportsMessageRewind
                      ? () => {
                          if (!promptRunId) {
                            return;
                          }
                          requestPromptRewind(promptRunId, "undo");
                        }
                      : undefined
                  }
                  onEdit={
                    activeThreadSupportsMessageRewind
                      ? () => {
                          if (!promptRunId) {
                            return;
                          }
                          setEditingPrompt({
                            turnId: turn.id,
                            runId: promptRunId,
                            text: visiblePromptText,
                          });
                        }
                      : undefined
                  }
                  actionsDisabled={
                    !promptRunId ||
                    isBusy ||
                    Boolean(messageActionPendingRunId) ||
                    Boolean(editingPrompt) ||
                    Boolean(pendingPromptRewind)
                  }
                  actionPending={messageActionPendingRunId === promptRunId}
                  editing={promptEditing}
                  editText={
                    promptEditing ? editingPrompt.text : visiblePromptText
                  }
                  editSubmitDisabled={
                    (promptEditing
                      ? editingPrompt.text
                      : visiblePromptText
                    ).trim().length === 0 &&
                    (turn.prompt.tokens?.length ?? 0) === 0 &&
                    (turn.prompt.attachments?.length ?? 0) === 0 &&
                    (turn.prompt.fileComments?.length ?? 0) === 0
                  }
                  onEditTextChange={(text) => {
                    setEditingPrompt((current) =>
                      current?.turnId === turn.id
                        ? { ...current, text }
                        : current,
                    );
                  }}
                  onCancelEdit={() => {
                    setEditingPrompt(undefined);
                    dismissPromptRewindDialog();
                  }}
                  onSubmitEdit={() => {
                    if (!promptRunId) {
                      return;
                    }
                    requestPromptRewind(promptRunId, "edit");
                  }}
                />
              </div>
              <div className="execution-turn-body">
                {hasExecutionProcess ? null : (
                  <RunElapsedTime
                    elapsedMs={settledElapsedMs}
                    {...turnTiming}
                  />
                )}
                {turnBodySegments.flatMap((segment) =>
                  renderTurnBodySegment(
                    turn.id,
                    segment,
                    turnTiming,
                    `${turnDisclosureScope}:body:${segment.id}`,
                  ),
                )}
              </div>
            </div>
          </section>
        );
      })}
    </ExecutionFeed>
  );
}
