import {
  AwakenAssistantMessage as AssistantMessage,
  AwakenIcon,
  AwakenDeliverableList as DeliverableList,
  AwakenMediaGenerationCard as MediaGenerationCard,
  AwakenReconnectStatus as ReconnectStatus,
  AwakenSubagentGroup as SubagentGroup,
  AwakenToolTraceLine as ToolTraceLine,
  type AwakenMediaMetadata,
  type PlatformKind,
  type ToolTraceAction,
  type ToolTraceDetailCard,
} from "@awaken/design-system";
import { Fragment, useCallback, useMemo, useRef } from "react";
import type { AwakenTheme } from "../shared/runtime";
import type { ToolDisplayState } from "../shared/toolStatus";
import {
  artifactMediaSourceKey,
  MediaArtifactPreviewItem,
  mediaArtifactRequestForPath,
  mediaFormatLabelForPath,
} from "./artifactMediaPresentation";
import type {
  ACPDisplayStatus,
  DeliverableFile,
  RunNarrativeItem,
} from "./run-narrative";
import { RunElapsedTime } from "./RunElapsedTime";
import type { RuntimeViewState } from "./runtime/reducer";
import {
  StoredExecutionProcessDisclosure,
  StoredToolTraceGroup,
  StoredToolTraceLine,
  StoredThoughtTrace,
} from "./StoredExecutionDisclosures";
import { subagentElapsedMs } from "./subagentPresentation";
import { toolResultPatch } from "./tracePatch";
import {
  activityBatchSummary,
  detailCardFromSections,
  toolRecordIdForNarrativeItem,
  rawToolId,
  argumentRecord,
  firstStringArgument,
  traceOperation,
  normalizeTracePath,
  ToolTraceActionCacheEntry,
  toolTracePresentation,
} from "./toolTracePresentation";
import type { useArtifactMediaSources } from "./useArtifactMediaSources";
import type { WorkbenchDisclosureStore } from "./workbenchDisclosureStore";
import {
  projectNarrativeActivitySegments,
  shouldShowAssistantAnswerFooter,
  type NarrativeActivityBatch,
  type NarrativeTurnBodySegment,
} from "./workbenchPresentation";
type Options = {
  traceRoot?: string;
  onOpenTraceFile?: (
    path: string,
    startLine?: number,
    endLine?: number,
  ) => void;
  runtimeState: Pick<
    RuntimeViewState,
    | "activeThreadId"
    | "threadId"
    | "artifacts"
    | "subagentsById"
    | "toolResults"
    | "patchPreviews"
  >;
  appearanceTheme: AwakenTheme;
  platform: PlatformKind;
  replayedNarrativeItemIds: Set<string>;
  media: ReturnType<typeof useArtifactMediaSources>;
  subagentNowMs: number;
  openSubagentTaskId?: string;
  handleOpenSubagent: (taskId?: string) => void;
  onStopAllAgents?: () => void | Promise<void>;
  openMediaPreview: (
    items: MediaArtifactPreviewItem[],
    activeId: string,
  ) => void;
  handleOpenDeliverablePreview: (
    file: Pick<DeliverableFile, "runId" | "title" | "fileType" | "artifactId">,
    threadId: string,
  ) => void;
  setNotice: (message: string | null) => void;
  workbenchDisclosureStore: WorkbenchDisclosureStore;
};
export function useNarrativeRenderer(options: Options) {
  const {
    runtimeState,
    appearanceTheme,
    platform,
    replayedNarrativeItemIds,
    subagentNowMs,
    openSubagentTaskId,
    handleOpenSubagent,
    onStopAllAgents,
    openMediaPreview,
    handleOpenDeliverablePreview,
    setNotice,
    workbenchDisclosureStore,
  } = options;
  const {
    artifactMediaSources,
    artifactMediaMetadata,
    openingArtifactId,
    recordArtifactMediaMetadata,
    refreshArtifactMediaSource,
    openArtifactExternally,
    revealArtifact,
  } = options.media;

  const toolTraceActionCacheRef = useRef<
    Map<string, ToolTraceActionCacheEntry>
  >(new Map());
  const traceCacheScope = `${runtimeState.activeThreadId ?? ""}:${runtimeState.threadId ?? ""}`;
  const traceCacheScopeRef = useRef(traceCacheScope);
  if (traceCacheScopeRef.current !== traceCacheScope) {
    traceCacheScopeRef.current = traceCacheScope;
    toolTraceActionCacheRef.current.clear();
  }

  const toolResultsById = useMemo(
    () =>
      new Map(
        [
          ...runtimeState.toolResults,
          ...Object.values(runtimeState.subagentsById).flatMap(
            (task) => task.toolResults ?? [],
          ),
        ].map((record) => [
          record.runId ? `${record.runId}:${record.id}` : record.id,
          record,
        ]),
      ),
    [runtimeState.toolResults, runtimeState.subagentsById],
  );

  const patchPreviewsById = useMemo(
    () =>
      new Map(
        [
          ...runtimeState.patchPreviews,
          ...Object.values(runtimeState.subagentsById).flatMap(
            (task) => task.patchPreviews ?? [],
          ),
        ].map((preview) => [
          preview.runId ? `${preview.runId}:${preview.id}` : preview.id,
          preview,
        ]),
      ),
    [runtimeState.patchPreviews, runtimeState.subagentsById],
  );

  const copyToolDetail = useCallback((copyText: string): void => {
    void navigator.clipboard.writeText(copyText).catch(() => {
      setNotice("复制失败，请手动选择详情内容。");
    });
  }, []);

  function cachedToolTraceAction(
    key: string,
    inputs: readonly unknown[],
    create: () => ToolTraceAction,
  ): ToolTraceAction {
    const cached = toolTraceActionCacheRef.current.get(key);
    if (
      cached &&
      cached.inputs.length === inputs.length &&
      cached.inputs.every((input, index) => Object.is(input, inputs[index]))
    ) {
      return cached.action;
    }
    const action = create();
    toolTraceActionCacheRef.current.set(key, { inputs, action });
    if (toolTraceActionCacheRef.current.size > 4096) {
      const oldest = toolTraceActionCacheRef.current.keys().next().value;
      if (oldest !== undefined) toolTraceActionCacheRef.current.delete(oldest);
    }
    return action;
  }

  function toolTraceActionFor(
    {
      id,
      toolCallId,
      visualKind,
      title,
      detail,
      status,
    }: {
      id: string;
      toolCallId?: string;
      visualKind?: "search";
      title: string;
      detail?: string;
      status: ToolDisplayState;
    },
    animateEntry?: boolean,
  ): ToolTraceAction {
    const toolId = toolRecordIdForNarrativeItem({ id, toolCallId });
    const tool =
      toolResultsById.get(rawToolId(id)) ?? toolResultsById.get(toolId);
    const patch =
      patchPreviewsById.get(rawToolId(id)) ?? patchPreviewsById.get(toolId);
    return cachedToolTraceAction(
      `tool:${id}`,
      [
        tool,
        patch,
        detail,
        title,
        visualKind,
        status,
        animateEntry,
        copyToolDetail,
        options.onOpenTraceFile,
      ],
      () => {
        const presentation = toolTracePresentation(tool, detail, {
          title,
          patch,
          allowLegacyFallbackOutput: status !== "active",
          status,
        });
        const args = argumentRecord(tool?.args);
        const filePath = firstStringArgument(args, [
          "file_path",
          "path",
          "filePath",
        ]);
        const operation = traceOperation(tool?.title, presentation.kind);
        const hasFile = Boolean(
          filePath && (operation === "read" || operation === "edit"),
        );
        return {
          id,
          title: presentation.title || title,
          detail: presentation.detail,
          output: presentation.output,
          outputLabel: presentation.outputLabel,
          outputTruncated: presentation.outputTruncated,
          detailCard: presentation.detailCard,
          kind: presentation.kind,
          icon:
            visualKind === "search" ? (
              <AwakenIcon concept="search" size={14} />
            ) : undefined,
          status,
          animateEntry,
          onCopyDetail: copyToolDetail,
          fileLabel: hasFile ? filePath!.split(/[\\/]/).at(-1) : undefined,
          onOpenFile:
            hasFile && options.onOpenTraceFile
              ? () =>
                  options.onOpenTraceFile!(
                    filePath!,
                    typeof args?.offset === "number" ? args.offset : undefined,
                    typeof args?.offset === "number" &&
                      typeof args?.limit === "number" &&
                      args.limit > 0
                      ? args.offset + args.limit - 1
                      : undefined,
                  )
              : undefined,
        };
      },
    );
  }

  function skillTraceAction(
    item: Extract<RunNarrativeItem, { kind: "skill" }>,
    animateEntry?: boolean,
  ): ToolTraceAction {
    return cachedToolTraceAction(
      `skill:${item.id}`,
      [item, animateEntry, copyToolDetail],
      () => {
        const sections: ToolTraceDetailCard["sections"] = [];
        if (item.detail?.trim()) {
          sections.push({
            label: "名称、来源与版本",
            content: item.detail.trim(),
            format: "text",
          });
        }
        if (item.output?.trim()) {
          sections.unshift({
            label: "结果",
            content: item.output.trim(),
            format: "text",
          });
        }
        return {
          id: item.id,
          title: item.title.replace(/\s*\{\s*$/, "").trim(),
          status: item.status,
          kind: "skill",
          detailCard: detailCardFromSections("Skill 详情", sections),
          output: item.output,
          outputLabel: "Skill 输出",
          animateEntry,
          onCopyDetail: copyToolDetail,
        };
      },
    );
  }

  function hookTraceAction(
    item: Extract<RunNarrativeItem, { kind: "hook" }>,
    animateEntry?: boolean,
  ): ToolTraceAction {
    return cachedToolTraceAction(
      `hook:${item.id}`,
      [item, animateEntry, copyToolDetail],
      () => {
        const sections: ToolTraceDetailCard["sections"] = [];
        if (item.detail?.trim())
          sections.push({
            label: "事件与来源",
            content: item.detail.trim(),
            format: "text",
          });
        if (item.output?.trim())
          sections.unshift({
            label: "安全回执",
            content: item.output.trim(),
            format: "text",
          });
        return {
          id: item.id,
          title: item.title,
          status: item.status,
          kind: "terminal",
          detailCard: detailCardFromSections("Hook 详情", sections),
          output: item.output,
          outputLabel: "Hook 回执",
          animateEntry,
          onCopyDetail: copyToolDetail,
        };
      },
    );
  }

  function actionsForActivityBatch(
    batch: NarrativeActivityBatch,
    animateToolEntry: boolean,
  ): ToolTraceAction[] {
    return batch.items.flatMap((item) => {
      if (item.kind === "thinking" || item.kind === "planning") return [];
      if (item.kind === "exploring") {
        return item.actions.map((action) =>
          toolTraceActionFor(
            action,
            animateToolEntry && !replayedNarrativeItemIds.has(action.id),
          ),
        );
      }
      const animateEntry =
        animateToolEntry && !replayedNarrativeItemIds.has(item.id);
      if (item.kind === "skill") {
        return [skillTraceAction(item, animateEntry)];
      }
      if (item.kind === "hook") {
        return [hookTraceAction(item, animateEntry)];
      }
      return [toolTraceActionFor(item, animateEntry)];
    });
  }

  function renderStoredToolTraceLine(
    action: ToolTraceAction,
    key: string,
    disclosureKey: string,
  ) {
    const { id, ...lineProps } = action;
    delete lineProps.open;
    delete lineProps.onOpenChange;
    return (
      <StoredToolTraceLine
        key={key}
        {...lineProps}
        disclosureKey={disclosureKey}
        store={workbenchDisclosureStore}
      />
    );
  }

  function renderNarrativeActivityItems(
    turnId: string,
    items: RunNarrativeItem[],
    animateToolEntry = true,
    disclosureScope = turnId,
  ) {
    const scope = `${runtimeState.activeThreadId ?? runtimeState.threadId ?? "local"}:${disclosureScope.replace(/:(?:items|execution-items)$/, "")}`;
    const entryKey = (id: string) => `${scope}:activity-entry:${id}`;
    return projectNarrativeActivitySegments({ turnId, items }).map(
      (segment) => {
        if (segment.kind === "item")
          return renderNarrativeItem(
            segment.item,
            segment.id,
            animateToolEntry,
            entryKey(segment.item.id),
          );
        const actions = actionsForActivityBatch(segment, false);
        const actionsById = new Map(
          actions.map((action) => [action.id, action]),
        );
        const summary = activityBatchSummary(
          actions.map((action) => {
            const id = rawToolId(action.id);
            const record =
              toolResultsById.get(id) ??
              toolResultsById.get(toolRecordIdForNarrativeItem(action));
            const patch =
              patchPreviewsById.get(id) ??
              patchPreviewsById.get(toolRecordIdForNarrativeItem(action)) ??
              toolResultPatch(record);
            const args = argumentRecord(record?.args);
            const path = firstStringArgument(args, [
              "file_path",
              "path",
              "filePath",
            ]);
            const paths = patch?.affectedPaths.length
              ? patch.affectedPaths
              : path
                ? [path]
                : [];
            return {
              id: action.id,
              kind: action.kind ?? "tool",
              title: action.title,
              status: action.status,
              toolName: record?.title,
              patch,
              paths: paths.map((path) =>
                normalizeTracePath(
                  !/^(?:[/\\]|[A-Za-z]:)/.test(path) && options.traceRoot
                    ? `${options.traceRoot}/${path}`
                    : path,
                ),
              ),
            };
          }),
        );
        const entries = segment.items.flatMap<{
          id: string;
          action?: ToolTraceAction;
          thought?: Extract<
            RunNarrativeItem,
            { kind: "thinking" | "planning" }
          >;
        }>((item) => {
          if (item.kind === "exploring")
            return item.actions.map((action) => ({
              id: action.id,
              action: actionsById.get(action.id)!,
            }));
          if (item.kind === "thinking" || item.kind === "planning")
            return [{ id: item.id, thought: item }];
          return [{ id: item.id, action: actionsById.get(item.id)! }];
        }) as Array<{
          id: string;
          action?: ToolTraceAction;
          thought?: Extract<
            RunNarrativeItem,
            { kind: "thinking" | "planning" }
          >;
        }>;
        const active = entries.filter(
          (entry) =>
            (entry.action?.status ?? entry.thought?.status) === "active",
        );
        const last = active.at(-1);
        const currentLabel = last?.thought
          ? last.thought.kind === "planning" && last.thought.phase
            ? last.thought.text
            : "思考中"
          : last?.action
            ? `正在${last.action.title.replace(/^正在/, "")}`
            : undefined;
        const failures = actions.filter(
          (action) => action.status !== "active" && action.status !== "done",
        );
        return (
          <StoredToolTraceGroup
            key={segment.id}
            disclosureKey={`${scope}:${segment.id}`}
            store={workbenchDisclosureStore}
            actions={actions}
            replayed={!animateToolEntry}
            label={summary.label}
            detail={summary.detail}
            entryCount={entries.length}
            activityStatus={active.length ? "active" : "done"}
            currentActivity={
              currentLabel
                ? `${currentLabel}${active.length > 1 ? ` · 另有 ${active.length - 1} 项进行中` : ""}`
                : undefined
            }
            attention={
              failures.length
                ? failures
                    .map(
                      (action) =>
                        `${action.title}：${action.detail || "未成功完成"}`,
                    )
                    .join("；")
                : undefined
            }
            animateItems={false}
          >
            {entries.map((entry) =>
              entry.thought ? (
                <StoredThoughtTrace
                  key={entry.id}
                  disclosureKey={entryKey(entry.id)}
                  store={workbenchDisclosureStore}
                  text={
                    entry.thought.kind === "planning" ? "" : entry.thought.text
                  }
                  status={entry.thought.status}
                  label={
                    entry.thought.kind === "planning" && entry.thought.phase
                      ? entry.thought.text
                      : undefined
                  }
                  animateEntry={false}
                />
              ) : (
                renderStoredToolTraceLine(
                  entry.action!,
                  entry.id,
                  `${entryKey(entry.id)}:tool`,
                )
              ),
            )}
          </StoredToolTraceGroup>
        );
      },
    );
  }

  function renderNarrativeItem(
    item: RunNarrativeItem,
    key: string,
    animateToolEntry = true,
    disclosureKey = key,
  ) {
    if (item.kind === "planning" || item.kind === "thinking") {
      const thoughtTrace = (
        <StoredThoughtTrace
          disclosureKey={disclosureKey}
          store={workbenchDisclosureStore}
          text={item.kind === "planning" ? "" : item.text}
          status={item.kind === "planning" ? "active" : item.status}
          animateEntry={
            animateToolEntry && !replayedNarrativeItemIds.has(item.id)
          }
          label={item.kind === "planning" && item.phase ? item.text : undefined}
          theme={appearanceTheme}
        />
      );
      return item.kind === "planning" && item.phase ? (
        <div key={key} className="narrative-model-continuation">
          {thoughtTrace}
        </div>
      ) : (
        <Fragment key={key}>{thoughtTrace}</Fragment>
      );
    }
    if (item.kind === "answer") {
      const showAnswerFooter = shouldShowAssistantAnswerFooter({
        status: item.status,
        isFinal: item.isFinal,
      });
      return (
        <div
          key={key}
          className="assistant-message-wrapper"
          data-final={item.isFinal ? "true" : "false"}
        >
          <AssistantMessage
            markdown={item.markdown}
            status={item.status}
            showFooter={showAnswerFooter}
          />
        </div>
      );
    }
    if (item.kind === "hint") {
      return (
        <div key={key} className="narrative-hint">
          {item.text}
        </div>
      );
    }
    if (item.kind === "providerReconnect") {
      return (
        <ReconnectStatus
          key={key}
          title={providerReconnectTitle(item)}
          detail={item.detail}
        />
      );
    }
    if (item.kind === "compactBoundary") {
      return null;
    }
    if (item.kind === "exploring") {
      const actions = item.actions.map((action) =>
        toolTraceActionFor(
          action,
          animateToolEntry && !replayedNarrativeItemIds.has(action.id),
        ),
      );
      if (actions.length === 1) {
        return renderStoredToolTraceLine(
          actions[0],
          key,
          `${disclosureKey}:tool`,
        );
      }
      return (
        <StoredToolTraceGroup
          key={key}
          disclosureKey={`${disclosureKey}:batch`}
          store={workbenchDisclosureStore}
          actions={actions}
          replayed={item.actions.every((action) =>
            replayedNarrativeItemIds.has(action.id),
          )}
          animateItems={animateToolEntry}
        />
      );
    }
    if (item.kind === "tool") {
      return renderStoredToolTraceLine(
        toolTraceActionFor(
          item,
          animateToolEntry && !replayedNarrativeItemIds.has(item.id),
        ),
        key,
        `${disclosureKey}:tool`,
      );
    }
    if (item.kind === "skill") {
      return renderStoredToolTraceLine(
        skillTraceAction(
          item,
          animateToolEntry && !replayedNarrativeItemIds.has(item.id),
        ),
        key,
        `${disclosureKey}:skill`,
      );
    }
    if (item.kind === "hook") {
      return renderStoredToolTraceLine(
        hookTraceAction(
          item,
          animateToolEntry && !replayedNarrativeItemIds.has(item.id),
        ),
        key,
        `${disclosureKey}:hook`,
      );
    }
    if (item.kind === "mediaGeneration") {
      const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
      const artifactsByPath = new Map(
        runtimeState.artifacts
          .filter((candidate) => Boolean(candidate.path))
          .map((candidate) => [
            candidate.path!.replace(/\\/g, "/").toLowerCase(),
            candidate,
          ]),
      );
      const resultItems =
        item.mediaItems && item.mediaItems.length > 0
          ? item.mediaItems
          : item.artifactPaths.map((path, index) => ({
              index: index + 1,
              status: "ready" as const,
              path,
            }));
      const mediaEntries = resultItems.map((resultItem) => {
        const artifactPath = resultItem.path || "";
        const artifact = artifactPath
          ? artifactsByPath.get(artifactPath.replace(/\\/g, "/").toLowerCase())
          : undefined;
        const request =
          threadId && item.runId && artifactPath
            ? mediaArtifactRequestForPath(
                threadId,
                item.runId,
                artifactPath,
                artifact,
              )
            : undefined;
        const sourceKey = request ? artifactMediaSourceKey(request) : undefined;
        const sourceState = sourceKey
          ? artifactMediaSources[sourceKey]
          : undefined;
        const sourceFailed = sourceState === null;
        return {
          id:
            artifact?.id || request?.artifactId || `media-${resultItem.index}`,
          index: resultItem.index,
          status:
            resultItem.status === "ready" && sourceFailed
              ? ("error" as const)
              : resultItem.status,
          sourceUnavailable: resultItem.status === "ready" && sourceFailed,
          src: typeof sourceState === "string" ? sourceState : undefined,
          title: artifact?.title || `${item.title} ${resultItem.index}`,
          formatLabel: mediaFormatLabelForPath(artifactPath || artifact?.path),
          sourceKey,
          request,
        };
      });
      const previewItems: MediaArtifactPreviewItem[] = mediaEntries.flatMap(
        (entry) =>
          entry.src && entry.request && entry.sourceKey
            ? [
                {
                  id: entry.id,
                  kind: item.mediaKind,
                  title: entry.title,
                  alt: entry.title,
                  formatLabel: entry.formatLabel,
                  sourceKey: entry.sourceKey,
                  request: entry.request,
                },
              ]
            : [],
      );
      const mediaSources = mediaEntries.map((entry) => {
        const sourceKey = entry.request
          ? artifactMediaSourceKey(entry.request)
          : undefined;
        return {
          id: entry.id,
          index: entry.index,
          status: entry.status,
          detail: entry.sourceUnavailable
            ? "预览不可用"
            : entry.status === "ready"
              ? "点击预览"
              : entry.status === "unknown"
                ? "状态待确认"
                : "生成失败",
          src: entry.src,
          alt: entry.title,
          formatLabel: entry.formatLabel,
          metadata: sourceKey ? artifactMediaMetadata[sourceKey] : undefined,
          onPreview:
            entry.src && entry.request
              ? () => openMediaPreview(previewItems, entry.id)
              : undefined,
          onMediaError:
            sourceKey && entry.request
              ? () => refreshArtifactMediaSource(sourceKey)
              : undefined,
          onMetadata: sourceKey
            ? (metadata: AwakenMediaMetadata) =>
                recordArtifactMediaMetadata(sourceKey, metadata)
            : undefined,
        };
      });
      return (
        <MediaGenerationCard
          key={key}
          kind={item.mediaKind}
          status={item.status}
          title={item.title}
          detail={item.detail}
          requestedCount={item.requestedCount}
          requestedAspectRatio={item.requestedAspectRatio}
          sources={mediaSources}
        />
      );
    }
    if (item.kind === "subagentGroup") {
      return (
        <SubagentGroup
          key={key}
          tasks={item.tasks.map((task) => ({
            ...(runtimeState.subagentsById[task.taskId] ?? task),
            elapsedMs: subagentElapsedMs(
              runtimeState.subagentsById[task.taskId] ?? task,
              subagentNowMs,
            ),
            selected: task.taskId === openSubagentTaskId,
          }))}
          onSelect={handleOpenSubagent}
          onStopAll={onStopAllAgents}
        />
      );
    }
    if (item.kind === "scheduledNotice") {
      return (
        <div
          key={key}
          className="scheduled-notice-card"
          role="note"
          aria-label="定时提醒"
        >
          <span className="scheduled-notice-card__icon" aria-hidden="true">
            <AwakenIcon concept="scheduled-task" size={14} />
          </span>
          <span className="scheduled-notice-card__body">
            <span className="scheduled-notice-card__header">
              <span className="scheduled-notice-card__label">定时提醒</span>
              {item.taskName ? (
                <span className="scheduled-notice-card__title">
                  {item.taskName}
                </span>
              ) : null}
            </span>
            <span className="scheduled-notice-card__text">
              {item.noticeContent}
            </span>
          </span>
        </div>
      );
    }
    if (item.kind === "memoryReceipt") {
      return (
        <ToolTraceLine
          key={key}
          title={item.title}
          detail={item.detail}
          status={item.status}
          animateEntry={
            animateToolEntry && !replayedNarrativeItemIds.has(item.id)
          }
        />
      );
    }
    if (item.kind === "acpSession") {
      const title = item.message
        ? `${acpRunnerLabel(item.runnerId)} · ${item.message}`
        : acpRunnerLabel(item.runnerId);
      return (
        <ToolTraceLine
          key={key}
          title={title}
          detail={item.sessionId}
          status={acpDisplayStatusToToolStatus(item.status)}
          animateEntry={
            animateToolEntry && !replayedNarrativeItemIds.has(item.id)
          }
        />
      );
    }
    if (item.kind === "acpTool") {
      return (
        <ToolTraceLine
          key={key}
          title={item.title}
          detail={item.detail}
          status={acpDisplayStatusToToolStatus(item.status)}
          animateEntry={
            animateToolEntry && !replayedNarrativeItemIds.has(item.id)
          }
        />
      );
    }
    if (item.kind === "deliverables") {
      const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
      return (
        <DeliverableList
          key={key}
          terminalStatus={item.terminalStatus}
          items={item.files.map((file) => {
            const isMedia =
              file.fileType === "image" || file.fileType === "video";
            const mediaRequest =
              threadId && isMedia
                ? {
                    threadId,
                    runId: file.runId,
                    artifactId: file.artifactId,
                  }
                : undefined;
            const mediaSourceKey = mediaRequest
              ? artifactMediaSourceKey(mediaRequest)
              : undefined;
            const mediaSourceState =
              mediaSourceKey !== undefined
                ? artifactMediaSources[mediaSourceKey]
                : undefined;
            return {
              id: file.artifactId,
              title: file.title,
              extension: file.extension,
              fileType: file.fileType,
              previewSrc:
                isMedia && typeof mediaSourceState === "string"
                  ? mediaSourceState
                  : undefined,
              previewUnavailable: isMedia && mediaSourceState === null,
              previewMetadata:
                mediaSourceKey !== undefined
                  ? artifactMediaMetadata[mediaSourceKey]
                  : undefined,
              onPreview:
                isMedia && threadId
                  ? () => handleOpenDeliverablePreview(file, threadId)
                  : undefined,
              onMediaError:
                mediaRequest && mediaSourceKey
                  ? () => refreshArtifactMediaSource(mediaSourceKey)
                  : undefined,
              onMediaMetadata: mediaSourceKey
                ? (metadata: AwakenMediaMetadata) =>
                    recordArtifactMediaMetadata(mediaSourceKey, metadata)
                : undefined,
              opening: openingArtifactId === file.artifactId,
              disabled: !threadId,
              onOpen: threadId
                ? isMedia
                  ? () =>
                      openArtifactExternally({
                        threadId,
                        runId: file.runId,
                        artifactId: file.artifactId,
                      })
                  : () => handleOpenDeliverablePreview(file, threadId)
                : undefined,
              onOpenExternal:
                threadId && !isMedia
                  ? () =>
                      openArtifactExternally({
                        threadId,
                        runId: file.runId,
                        artifactId: file.artifactId,
                      })
                  : undefined,
              onReveal: threadId
                ? () =>
                    revealArtifact({
                      threadId,
                      runId: file.runId,
                      artifactId: file.artifactId,
                    })
                : undefined,
              localOpenLabel: "使用本地应用打开",
              revealLabel:
                platform === "macos"
                  ? "在 Finder 中显示"
                  : platform === "windows"
                    ? "在文件资源管理器中显示"
                    : "在文件管理器中显示",
            };
          })}
        />
      );
    }
    if (item.kind === "fileChange") {
      return null;
    }
    return null;
  }

  function providerReconnectTitle(
    item: Extract<RunNarrativeItem, { kind: "providerReconnect" }>,
  ): string {
    if (item.phase === "waiting_for_network") {
      return "正在等待网络";
    }
    if (item.maxAttempts > 0) {
      return `正在重新连接 ${item.attempt}/${item.maxAttempts}`;
    }
    return "正在重新连接";
  }

  function renderTurnBodySegment(
    turnId: string,
    segment: NarrativeTurnBodySegment,
    timing: {
      startedAtMs?: number;
      completedAtMs?: number;
      running: boolean;
    },
    disclosureScope: string,
  ) {
    // The outer process wrapper changes at completion; entry choices belong to
    // the turn, independently of that wrapper's body segment identity.
    const segmentSuffix = `:body:${segment.id}`;
    const activityScope = disclosureScope.endsWith(segmentSuffix)
      ? disclosureScope.slice(0, -segmentSuffix.length)
      : disclosureScope;
    if (segment.kind === "items") {
      return renderNarrativeActivityItems(
        turnId,
        segment.items,
        true,
        `${activityScope}:items`,
      );
    }

    const hasElapsedTime =
      segment.elapsedMs !== undefined ||
      (timing.startedAtMs !== undefined &&
        (timing.running || timing.completedAtMs !== undefined));

    return [
      <StoredExecutionProcessDisclosure
        key={`${turnId}:${segment.id}`}
        disclosureKey={`${disclosureScope}:execution`}
        store={workbenchDisclosureStore}
        title={
          <span className="execution-process-disclosure-title">
            {hasElapsedTime ? (
              <RunElapsedTime
                elapsedMs={segment.elapsedMs}
                startedAtMs={timing.startedAtMs}
                completedAtMs={timing.completedAtMs}
                running={timing.running}
              />
            ) : (
              segment.title
            )}
          </span>
        }
      >
        <div className="execution-process-disclosure-body">
          {renderNarrativeActivityItems(
            turnId,
            segment.items,
            false,
            `${activityScope}:execution-items`,
          )}
        </div>
      </StoredExecutionProcessDisclosure>,
    ];
  }

  function acpRunnerLabel(runnerId: string): string {
    return runnerId
      .split("-")
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ");
  }

  function acpDisplayStatusToToolStatus(
    status: ACPDisplayStatus,
  ): ToolDisplayState {
    switch (status) {
      case "started":
      case "active":
        return "active";
      case "done":
      case "closed":
        return "done";
      case "error":
        return "error";
      case "interrupted":
        return "interrupted";
      default:
        return "active";
    }
  }
  return {
    renderNarrativeItem,
    renderTurnBodySegment,
    renderNarrativeActivityItems,
  };
}
