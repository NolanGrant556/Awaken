import type { TurnMarkerRailDisplay } from "./workbenchPresentation";
import type { AwakenMediaMetadata } from "@awaken/design-system";
import type { ArtifactOpenRequest } from "../shared/runtime";
import type { RunPlanView } from "./runtime/reducer";
import {
  AwakenAssistantMessage as AssistantMessage,
  AwakenIcon,
  AwakenContextUsagePanel as ContextUsagePanel,
  AwakenMediaPreviewOverlay as MediaPreviewOverlay,
  AwakenRunPlanPanel as RunPlanPanel,
  AwakenSubagentExecutionCard as SubagentExecutionCard,
} from "@awaken/design-system";
import { shouldShowRunPlanSurface } from "./WorkbenchSurfacePolicy";
import { contextUsagePanelData } from "./contextUsagePresentation";
import type { RunNarrativeItem } from "./run-narrative";
import { type SubagentTaskView } from "./runtime/reducer";
import { subagentAgentTypeLabel } from "./subagentPresentation";
import { useArtifactMediaSources } from "./useArtifactMediaSources";
import { useFileWorkspaceSession } from "./useFileWorkspaceSession";
import { useNarrativeRenderer } from "./useNarrativeRenderer";
import { useRunPlanEntry } from "./useRunPlanEntry";
import { useRunStageScrollController } from "./useRunStageScrollController";
import { useWorkbenchPanels } from "./useWorkbenchPanels";
import {
  contextUsagePanelBottomOffset,
  runPlanPanelBottomOffset,
  subagentDetailPanelBottomOffset,
} from "./workbenchPresentation";

type Props = {
  workbenchPanels: Pick<
    ReturnType<typeof useWorkbenchPanels>,
    | "contextUsagePanelOpen"
    | "contextUsagePanelRef"
    | "mediaPreview"
    | "setMediaPreview"
    | "setOpenSubagentTaskId"
    | "closeContextUsagePanel"
  >;
  artifactMedia: Pick<
    ReturnType<typeof useArtifactMediaSources>,
    | "openingArtifactId"
    | "recordArtifactMediaMetadata"
    | "refreshArtifactMediaSource"
    | "openArtifactExternally"
    | "revealArtifact"
  >;
  narrativeRenderer: Pick<
    ReturnType<typeof useNarrativeRenderer>,
    "renderNarrativeActivityItems"
  >;
  fileWorkspace: Pick<
    ReturnType<typeof useFileWorkspaceSession>,
    "fileWorkspaceSurfaceActive"
  >;
  scrollController: Pick<
    ReturnType<typeof useRunStageScrollController>,
    | "activeWorkbenchTurnId"
    | "composerBarInsetPx"
    | "composerBarSafeAreaPx"
    | "composerContentInsetPx"
    | "handleReturnToLatestClick"
    | "handleTurnMarkerClick"
    | "returnToLatestBottomPx"
    | "returnToLatestVisible"
  >;
  renderNoticeToast: () => import("react").ReactNode;
  showTurnMarkerRail: boolean;
  turnMarkerDisplay: TurnMarkerRailDisplay;
  contextPanelData: ReturnType<typeof contextUsagePanelData>;
  contextUsagePanelId: "awaken-context-usage-panel";
  activeMediaPreviewItem:
    | {
        src: string | undefined;
        metadata: AwakenMediaMetadata;
        id: string;
        kind: "image" | "video";
        title: string;
        alt?: string | undefined;
        formatLabel?: string | undefined;
        sourceKey: string;
        request: ArtifactOpenRequest;
      }
    | undefined;
  runPlanEntry: ReturnType<typeof useRunPlanEntry>;
  activeRunPlan: RunPlanView | undefined;
  runPlanPanelId: "awaken-run-plan-panel";
  activeRunPlanItems: {
    step: string;
    activeForm: string;
    status: "completed" | "pending" | "in_progress";
  }[];
  runPlanPaused: boolean;
  openSubagent: SubagentTaskView | undefined;
  openSubagentMetrics: string[];
  handleCancelSubagent: (taskId: string) => Promise<void>;
  cancellingSubagentTaskId: string | undefined;
  openSubagentNarrativeItems: RunNarrativeItem[];
  openSubagentHasAnswer: boolean;
  activeMediaPreviewIndex: number;
  activeMediaPreviewSourceState: string | null | undefined;
  mediaRevealLabel: string;
};
export function WorkbenchOverlays({
  workbenchPanels,
  artifactMedia,
  narrativeRenderer,
  fileWorkspace,
  scrollController,
  renderNoticeToast,
  showTurnMarkerRail,
  turnMarkerDisplay,
  contextPanelData,
  contextUsagePanelId,
  activeMediaPreviewItem,
  runPlanEntry,
  activeRunPlan,
  runPlanPanelId,
  activeRunPlanItems,
  runPlanPaused,
  openSubagent,
  openSubagentMetrics,
  handleCancelSubagent,
  cancellingSubagentTaskId,
  openSubagentNarrativeItems,
  openSubagentHasAnswer,
  activeMediaPreviewIndex,
  activeMediaPreviewSourceState,
  mediaRevealLabel,
}: Props) {
  const {
    contextUsagePanelOpen,
    contextUsagePanelRef,
    mediaPreview,
    setMediaPreview,
    setOpenSubagentTaskId,
    closeContextUsagePanel,
  } = workbenchPanels;
  const {
    openingArtifactId,
    recordArtifactMediaMetadata,
    refreshArtifactMediaSource,
    openArtifactExternally,
    revealArtifact,
  } = artifactMedia;
  const { renderNarrativeActivityItems } = narrativeRenderer;
  const { fileWorkspaceSurfaceActive } = fileWorkspace;
  const {
    activeWorkbenchTurnId,
    composerBarInsetPx,
    composerBarSafeAreaPx,
    composerContentInsetPx,
    handleReturnToLatestClick,
    handleTurnMarkerClick,
    returnToLatestBottomPx,
    returnToLatestVisible,
  } = scrollController;
  return (
    <>
      {renderNoticeToast()}
      {showTurnMarkerRail ? (
        <nav
          className={`turn-marker-rail turn-marker-rail-${turnMarkerDisplay.mode}`}
          aria-label="Message shortcuts"
          style={{
            bottom: Math.max(28, composerBarSafeAreaPx),
          }}
        >
          {turnMarkerDisplay.items.map((marker) => (
            <button
              key={`${marker.id}:${marker.index}`}
              type="button"
              className={`turn-marker-button${
                marker.id === activeWorkbenchTurnId
                  ? " turn-marker-button-active"
                  : ""
              }`}
              aria-current={
                marker.id === activeWorkbenchTurnId ? "true" : undefined
              }
              aria-label={`跳转到第 ${marker.index + 1} 条消息`}
              title={`第 ${marker.index + 1} 条消息`}
              onClick={() => handleTurnMarkerClick(marker.id)}
            >
              <span className="turn-marker-bar" aria-hidden="true" />
            </button>
          ))}
        </nav>
      ) : null}
      {!fileWorkspaceSurfaceActive && returnToLatestVisible ? (
        <div
          className="return-to-latest-overlay"
          style={{ bottom: returnToLatestBottomPx }}
        >
          <button
            type="button"
            className="return-to-latest-button"
            aria-label="回到最新输出"
            title="回到最新输出"
            onClick={handleReturnToLatestClick}
          >
            <AwakenIcon concept="dropdown" size={16} />
          </button>
        </div>
      ) : null}
      {!fileWorkspaceSurfaceActive &&
      contextUsagePanelOpen &&
      contextPanelData ? (
        <div
          className="context-usage-overlay"
          style={{
            bottom: contextUsagePanelBottomOffset({
              composerBarInset: composerBarInsetPx,
            }),
          }}
        >
          <ContextUsagePanel
            id={contextUsagePanelId}
            ref={contextUsagePanelRef}
            {...contextPanelData}
            onClose={() => closeContextUsagePanel(true)}
          />
        </div>
      ) : null}
      {shouldShowRunPlanSurface({
        fileWorkspaceSurfaceActive,
        mediaPreviewActive: Boolean(activeMediaPreviewItem),
      }) &&
      runPlanEntry.panelOpen &&
      activeRunPlan ? (
        <div
          className="run-plan-overlay"
          style={{
            bottom: runPlanPanelBottomOffset({
              composerContentInset: composerContentInsetPx,
            }),
          }}
        >
          <RunPlanPanel
            id={runPlanPanelId}
            ref={runPlanEntry.panelRef}
            items={activeRunPlanItems}
            explanation={activeRunPlan.explanation}
            paused={runPlanPaused}
            onClose={() => runPlanEntry.closePanel(true)}
          />
        </div>
      ) : null}
      {!fileWorkspaceSurfaceActive &&
      !activeMediaPreviewItem &&
      openSubagent ? (
        <div
          className="subagent-detail-overlay"
          style={{
            bottom: subagentDetailPanelBottomOffset({
              composerContentInset: composerContentInsetPx,
            }),
          }}
        >
          <SubagentExecutionCard
            title={openSubagent.description}
            agentLabel={subagentAgentTypeLabel(openSubagent.agentType)}
            status={openSubagent.status}
            activity={openSubagent.activity}
            metrics={openSubagentMetrics}
            onClose={() => setOpenSubagentTaskId(undefined)}
            onCancel={
              ["queued", "running", "cancelling"].includes(openSubagent.status)
                ? () => void handleCancelSubagent(openSubagent.taskId)
                : undefined
            }
            cancelDisabled={
              openSubagent.status === "cancelling" ||
              cancellingSubagentTaskId === openSubagent.taskId
            }
          >
            {openSubagentNarrativeItems.length > 0 ? (
              renderNarrativeActivityItems(
                `subagent:${openSubagent.taskId}`,
                openSubagentNarrativeItems,
                false,
              )
            ) : (
              <p className="subagent-execution-empty">等待子任务执行记录…</p>
            )}
            {!openSubagentHasAnswer && openSubagent.resultPreview ? (
              <AssistantMessage
                markdown={openSubagent.resultPreview}
                status={openSubagent.status === "failed" ? "error" : "done"}
                showFooter={false}
              />
            ) : null}
            {openSubagent.errorMessage ? (
              <div className="subagent-execution-error" role="alert">
                <strong>{openSubagent.errorCode || "子任务失败"}</strong>
                <p>{openSubagent.errorMessage}</p>
              </div>
            ) : null}
          </SubagentExecutionCard>
        </div>
      ) : null}
      {activeMediaPreviewItem && mediaPreview ? (
        <MediaPreviewOverlay
          item={activeMediaPreviewItem}
          position={activeMediaPreviewIndex}
          count={mediaPreview.items.length}
          sourceUnavailable={activeMediaPreviewSourceState === null}
          openingExternal={
            openingArtifactId === activeMediaPreviewItem.request.artifactId
          }
          localOpenLabel="使用本地应用打开"
          revealLabel={mediaRevealLabel}
          onClose={() => setMediaPreview(null)}
          onPrevious={
            mediaPreview.items.length > 1
              ? () => {
                  const previousIndex =
                    (activeMediaPreviewIndex - 1 + mediaPreview.items.length) %
                    mediaPreview.items.length;
                  setMediaPreview((current) =>
                    current
                      ? {
                          ...current,
                          activeId:
                            current.items[previousIndex]?.id ??
                            current.activeId,
                        }
                      : current,
                  );
                }
              : undefined
          }
          onNext={
            mediaPreview.items.length > 1
              ? () => {
                  const nextIndex =
                    (activeMediaPreviewIndex + 1) % mediaPreview.items.length;
                  setMediaPreview((current) =>
                    current
                      ? {
                          ...current,
                          activeId:
                            current.items[nextIndex]?.id ?? current.activeId,
                        }
                      : current,
                  );
                }
              : undefined
          }
          onOpenExternal={() => {
            void openArtifactExternally(activeMediaPreviewItem.request);
          }}
          onReveal={() => {
            void revealArtifact(activeMediaPreviewItem.request);
          }}
          onMediaError={() =>
            refreshArtifactMediaSource(
              activeMediaPreviewItem.sourceKey,
              activeMediaPreviewItem.request,
            )
          }
          onMetadata={(metadata) =>
            recordArtifactMediaMetadata(
              activeMediaPreviewItem.sourceKey,
              metadata,
            )
          }
        />
      ) : null}
    </>
  );
}
