import {
  Fragment,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";

import {
  AwakenAppShell as AppShell,
  AwakenAssistantMessage as AssistantMessage,
  AwakenButton as Button,
  AwakenComposerBar as ComposerBar,
  AwakenContextUsagePanel as ContextUsagePanel,
  AwakenDisclosure as Disclosure,
  AwakenDialog as Dialog,
  AwakenDeliverableList as DeliverableList,
  AwakenExecutionFeed as ExecutionFeed,
  AwakenIcon,
  AwakenMediaGenerationCard as MediaGenerationCard,
  AwakenMediaInspector as MediaInspector,
  AwakenMediaPreviewOverlay as MediaPreviewOverlay,
  AwakenInspectorRail as InspectorRail,
  AwakenSubagentGroup as SubagentGroup,
  AwakenSubagentExecutionCard as SubagentExecutionCard,
  awakenSubagentStatusLabels,
  AwakenProjectFoldersEditor as ProjectFoldersEditor,
  AwakenProviderLogo,
  AwakenReconnectStatus as ReconnectStatus,
  AwakenSelectableCard as SelectableCard,
  AwakenSidebar as Sidebar,
  AwakenSidebarSectionAction as SidebarSectionAction,
  AwakenTaskWelcome as TaskWelcome,
  AwakenThoughtTrace as ThoughtTrace,
  AwakenTooltip as Tooltip,
  AwakenToolTraceGroup as ToolTraceGroup,
  AwakenToolTraceLine as ToolTraceLine,
  AwakenUserPromptBlock as UserPromptBlock,
  AwakenWorkbenchChrome as WorkbenchChrome,
  AwakenWorkbenchShell as WorkbenchShell,
  AwakenWorkbenchSidebarFrame as WorkbenchSidebarFrame,
  AwakenRunPlanHistorical as RunPlanHistorical,
  AwakenRunPlanPanel as RunPlanPanel,
  AwakenRunPlanTrigger as RunPlanTrigger,
  AwakenRunStage as RunStage,
  AwakenComposerDock as ComposerDock,
  type ComposerBarInputSelection,
  type ComposerBarMetaItem,
  type ComposerBarStructuredInputValue,
  type AwakenContextUsagePanelProps,
  type AwakenMediaPreviewItem,
  type AwakenMediaMetadata,
  type NavItem,
  type PlatformKind,
  type ProjectFolderRow,
  type ProjectHoverCard,
  type ProjectItem,
  type ProjectMenuItem,
  type ToolTraceAction,
  type ToolTraceDetailCard,
} from "@awaken/design-system";
import { providerBrandForId } from "./model-provider-brand";
import {
  createRuntimeClient,
  type SubscribeHandlers,
} from "./runtime/client";
import type {
  AwakenTheme,
  AwakenThemePreference,
  ComposerToken,
  MemoryDreamTask,
  ProjectEntryActionRequest,
  ProjectPathComposerTokenPayload,
  RuntimeInfo,
  ModelAccessType,
  ModelProvider,
  ReasoningEffortLevel,
  RuntimeProject,
  RunInputAttachment,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  RuntimeThread,
  StartRunRequest,
  ArtifactOpenRequest,
  UpsertProjectResult,
} from "../shared/runtime";
import type { AguiEvent } from "../shared/agui";
import type {
  DesktopUpdateErrorCode,
  DesktopUpdateSnapshot,
} from "../shared/desktop-update";
import {
  buildSidebarUpdateAction,
  desktopUpdateInstallBlockedMessage,
} from "./desktopUpdatePresentation";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  threadItemFromRuntime,
  type ThreadItem,
  type RuntimeAction,
  type UserQuestionItem,
  type ComputerReviewItem,
  type ComputerHandoffItem,
  type SubagentTaskView,
} from "./runtime/reducer";
import type {
  ACPDisplayStatus,
  PromptNarrativeToken,
  RunNarrativeItem,
  SubagentNarrativeTask,
} from "./run-narrative";
import { describeGlobPattern } from "./run-narrative/tool-display";
import type { ToolDisplayState } from "../shared/toolStatus";
import { projectPetActivity } from "./pets/pet-activity-projector";
import {
  projectNarrativeActivitySegments,
  projectNarrativeTurnBodySegments,
  contextUsagePanelBottomOffset,
  diffLineStats,
  runPlanPanelBottomOffset,
  runPlanStepStatusForDisplay,
  subagentDetailPanelBottomOffset,
  narrativeTurnElapsedMs,
  projectVisibleNarrativeItems,
  shouldShowAssistantAnswerFooter,
  shouldUseCenterComposer,
  turnMarkerRailHeight,
  turnMarkerRailDisplay,
  type NarrativeTurnBodySegment,
  type NarrativeActivityBatch,
} from "./workbenchPresentation";
import { RunElapsedTime } from "./RunElapsedTime";
import { selectAllInFocusedEditor } from "./desktop-selection";
import { useComposerLifecycle } from "./useComposerLifecycle";
import {
  composerDraftOwnerKey,
  useThreadComposerDraft,
  type ThreadComposerDraft,
} from "./useThreadComposerDraft";
import { useRunStageScrollController } from "./useRunStageScrollController";
import { useRunPlanEntry } from "./useRunPlanEntry";
import { MACOS_WINDOW_CHROME_GEOMETRY } from "../shared/window-chrome";
import {
  actionsForThreadTranscript,
  streamCursorForTranscriptRun,
} from "./runtime/thread-transcript";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  TRACKED_TRASH_OPERATIONS_STORAGE_KEY,
  canReconcileTrackedTrashOperation,
  isSameWorkspacePath,
  normalizeWorkspacePath,
  parseTrackedTrashOperations,
  projectIdForThread,
  projectOperationReconciliationState,
  upsertTrackedTrashOperation,
  workspaceLabelForPath,
  workspaceProjectForFolderPath,
  workspaceProjectRunRoots,
  type TrackedTrashOperation,
  type WorkspaceProject,
  type WorkspaceProjectFolder,
} from "./projectWorkspace";
import { DesktopSettingsDialog } from "./settings/DesktopSettingsDialog";
import { ProfileBalancePopover } from "./ProfileBalancePopover";
import { ExtensionsView, type ExtensionSection } from "./ExtensionsView";
import { ScheduledTasksView } from "./ScheduledTasksView";
import { ExternalActionGate } from "./ExternalActionGate";
import { ComputerReviewGate } from "./ComputerReviewGate";
import { ComputerHandoffIndicator } from "./ComputerHandoffIndicator";
import {
  ExpandedFileComposer,
  expandedFileComposerCompletionKey,
  hasExpandedFileComposerDraft,
  shouldCollapseExpandedFileComposer,
  type ExpandedFileComposerTrackedRun,
  type ExpandedFileComposerIdleMode,
} from "./ExpandedFileComposer";
import { ProjectFilesWorkspace } from "./ProjectFilesWorkspace";
import ProjectFileIcon from "./ProjectFileIcon";
import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import type { ArtifactPreviewSelection } from "./file-preview/ArtifactPreviewHost";
import {
  activeProjectFileTab,
  createProjectFileTabsState,
  projectFileTabsReducer,
  type ProjectFileWorkspaceTab,
} from "./projectFileTabs";
import { projectFileIconKind } from "./projectFiles";
import {
  assignDreamReceiptToThread,
  dreamTasksForThreadReplay,
  parseDreamReceiptThreadAssignments,
  type DreamReceiptThreadAssignments,
} from "./memoryDreamReceipts";
import {
  MAX_COMPOSER_IMAGE_BYTES,
  SUPPORTED_COMPOSER_IMAGE_MIME_TYPES,
  composerAttachmentViews,
  composerDraftAttachmentFromPickedFile,
  type ComposerDraftAttachment,
  promptAttachmentViews,
  readComposerDraftImageAttachment,
  toRunInputAttachment,
} from "./composerAttachments";
import {
  actionsFromSlashProviders,
  composerActionTriggerFromDraft,
  computerUseMentionProvider,
  composerTokenFromSlashAction,
  filterSlashActions,
  firstSelectableSlashAction,
  skillSlashProvider,
  type ComposerTokenPlacement,
  type SlashAction,
} from "./composerSlash";
import {
  DEFAULT_INSPECTOR_WIDTH,
  FILE_WORKSPACE_INSPECTOR_CAP,
  MAX_INSPECTOR_WIDTH,
  MIN_INSPECTOR_WIDTH,
  computeEffectiveInspectorWidth,
  inspectorWidthCap,
  readWindowWidth,
  sanitizeInspectorPreference,
} from "./workbenchLayout";

type UserQuestionPromptProps = {
  question: UserQuestionItem;
  disabled?: boolean;
  onCancel: () => void;
  onSubmit: (answer: string) => void;
};

type MediaArtifactPreviewItem = Omit<AwakenMediaPreviewItem, "src" | "metadata"> & {
  sourceKey: string;
  request: ArtifactOpenRequest;
};

type MediaArtifactPreviewState = {
  items: MediaArtifactPreviewItem[];
  activeId: string;
};

type MediaArtifactRecordRef = {
  id: string;
  path?: string;
};

export function mediaArtifactRequestForPath(
  threadId: string,
  runId: string,
  artifactPath: string,
  artifact?: MediaArtifactRecordRef,
): ArtifactOpenRequest {
  if (artifact) {
    return { threadId, runId, artifactId: artifact.id };
  }
  return {
    threadId,
    runId,
    artifactId: `legacy-media:${artifactPath}`,
    artifactPath,
  };
}

function artifactMediaSourceKey(request: ArtifactOpenRequest): string {
  return `${request.threadId}:${request.runId}:${request.artifactId}`;
}

export function mediaFormatLabelForPath(path?: string): string | undefined {
  const filename = path?.trim().split(/[\\/]/).pop();
  const extension = filename?.includes(".") ? filename.split(".").pop() : undefined;
  return extension?.trim().toUpperCase() || undefined;
}

const MAX_ARTIFACT_MEDIA_SOURCE_RETRIES = 2;

export type UserQuestionSelection = {
  selected: string[];
  otherText: string;
};

const USER_QUESTION_OTHER_OPTION = "__other__";

type ProjectDeleteAction = "detach" | "delete_history" | "trash";

type PendingProjectDelete = {
  projectId: string;
  backendProjectId: string;
  label: string;
  action: ProjectDeleteAction;
  threadCount: number;
  /** 仅 trash 用：废纸篓是文件夹粒度，这里是本次要移走的那个目录。 */
  folderPath?: string;
};

type PendingThreadDelete = {
  threadIds: string[];
  source: "single" | "batch";
};

export type WorkbenchInspectorPresentation = {
  available: boolean;
  effectiveCollapsed: boolean;
  rootClassName:
    | "desktop-root-inspector-expanded"
    | "desktop-root-inspector-collapsed"
    | "desktop-root-inspector-hidden";
};

export type ExpandedWorkspaceSurface = "conversation" | "file";

export function mediaRevealLabelForPlatform(platform: PlatformKind): string {
  if (platform === "macos") return "在 Finder 中显示";
  if (platform === "windows") return "在文件资源管理器中显示";
  return "在文件管理器中显示";
}

export function shouldUseCenterComposerWithFileWorkspace({
  standardCenterComposer,
  fileWorkspaceOpen,
  fileWorkspaceExpanded,
  expandedWorkspaceSurface,
}: {
  standardCenterComposer: boolean;
  fileWorkspaceOpen: boolean;
  fileWorkspaceExpanded: boolean;
  expandedWorkspaceSurface: ExpandedWorkspaceSurface;
}): boolean {
  return (
    !(fileWorkspaceOpen && fileWorkspaceExpanded && expandedWorkspaceSurface === "file") &&
    standardCenterComposer
  );
}

export function shouldEnableInspectorResizeHandle({
  inspectorAvailable,
  fileWorkspaceExpanded,
}: {
  inspectorAvailable: boolean;
  fileWorkspaceExpanded: boolean;
}): boolean {
  return inspectorAvailable && !fileWorkspaceExpanded;
}

export function composerMetaItemsForFileWorkspaceSurface(
  metaItems: ComposerBarMetaItem[],
  fileWorkspaceSurfaceActive: boolean,
): ComposerBarMetaItem[] {
  return fileWorkspaceSurfaceActive ? metaItems.filter((item) => item.id === "model") : metaItems;
}

export type WorkbenchNavSection = ExtensionSection | "scheduled-tasks";

/** 一级导航中占据主内容区、且不属于 workbench 的页面（inspector 在这些页面隐藏）。 */
export function workbenchNavSectionForNavId(
  activeNavId: string | undefined,
): WorkbenchNavSection | undefined {
  if (activeNavId === "connectors" || activeNavId === "skills") {
    return activeNavId;
  }
  if (activeNavId === "scheduled-tasks") {
    return "scheduled-tasks";
  }
  return undefined;
}

export function workbenchInspectorPresentation(
  activeNavSection: WorkbenchNavSection | undefined,
  inspectorCollapsed: boolean,
): WorkbenchInspectorPresentation {
  if (activeNavSection) {
    return {
      available: false,
      effectiveCollapsed: true,
      rootClassName: "desktop-root-inspector-hidden",
    };
  }

  return {
    available: true,
    effectiveCollapsed: inspectorCollapsed,
    rootClassName: inspectorCollapsed
      ? "desktop-root-inspector-collapsed"
      : "desktop-root-inspector-expanded",
  };
}

export function nextSubagentInspectorTaskId(
  currentTaskId: string | undefined,
  fallbackTaskId: string | undefined,
): string | undefined {
  return currentTaskId ? undefined : fallbackTaskId;
}

const emptyUserQuestionSelection = (): UserQuestionSelection => ({
  selected: [],
  otherText: "",
});

export function selectedUserQuestionLabels(selection: UserQuestionSelection): string[] {
  const labels = selection.selected.filter((label) => label !== USER_QUESTION_OTHER_OPTION);
  if (selection.selected.includes(USER_QUESTION_OTHER_OPTION) && selection.otherText.trim()) {
    labels.push(selection.otherText.trim());
  }
  return labels;
}

export function isUserQuestionSelectionAnswered(selection: UserQuestionSelection): boolean {
  return selectedUserQuestionLabels(selection).length > 0;
}

export function nextUserQuestionPromptIndex(currentIndex: number, questionCount: number): number {
  if (questionCount <= 0) {
    return 0;
  }
  return Math.min(currentIndex + 1, questionCount - 1);
}

function clampUserQuestionPromptIndex(index: number, questionCount: number): number {
  if (questionCount <= 0) {
    return 0;
  }
  return Math.min(Math.max(index, 0), questionCount - 1);
}

export function promptNarrativeTokensForComposer(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
): PromptNarrativeToken[] {
  return tokens.map((token) => {
    const leadingText = token.leadingText ?? leadingTexts[token.id];
    return {
      ...token,
      payload: token.payload ? { ...token.payload } : undefined,
      leadingText: leadingText || undefined,
    };
  });
}

export function reconcileComposerStructuredInput(
  tokens: ComposerToken[],
  placements: Record<string, ComposerTokenPlacement>,
  nextValue: ComposerBarStructuredInputValue,
): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
} {
  const leadingTextByTokenId = new Map(
    nextValue.tokens.map((token) => [token.id, token.leadingText]),
  );
  const currentTokenIds = new Set(tokens.map((token) => token.id));
  const remainingTokenIds = new Set(
    nextValue.tokens.map((token) => token.id).filter((tokenId) => currentTokenIds.has(tokenId)),
  );
  const nextTokens = tokens
    .filter((token) => remainingTokenIds.has(token.id))
    .map((token) => {
      const leadingText = leadingTextByTokenId.get(token.id) ?? "";
      const tokenWithoutLeadingText = { ...token };
      delete tokenWithoutLeadingText.leadingText;
      return leadingText ? { ...tokenWithoutLeadingText, leadingText } : tokenWithoutLeadingText;
    });

  return {
    tokens: nextTokens,
    placements: Object.fromEntries(
      Object.entries(placements).filter(([tokenId]) => remainingTokenIds.has(tokenId)),
    ),
    leadingTexts: Object.fromEntries(
      nextValue.tokens
        .filter((token) => remainingTokenIds.has(token.id) && token.leadingText)
        .map((token) => [token.id, token.leadingText]),
    ),
    value: nextValue.value,
  };
}

function composerTokenLeadingText(
  token: ComposerToken,
  leadingTexts: Record<string, string>,
): string {
  return token.leadingText ?? leadingTexts[token.id] ?? "";
}

export function composerStructuredDraftLength(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
  value: string,
): number {
  return tokens.reduce(
    (length, token) => length + composerTokenLeadingText(token, leadingTexts).length + 1,
    value.length,
  );
}

function composerTokenEndOffset(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
  tokenId: string,
): number | undefined {
  let offset = 0;
  for (const token of tokens) {
    offset += composerTokenLeadingText(token, leadingTexts).length + 1;
    if (token.id === tokenId) return offset;
  }
  return undefined;
}

export function insertComposerTokenAtOffset(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  offset: number;
  token: ComposerToken;
}): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  focusOffset: number;
} {
  const totalLength = composerStructuredDraftLength(args.tokens, args.leadingTexts, args.value);
  const offset = Math.max(0, Math.min(args.offset, totalLength));
  const nextTokens: ComposerToken[] = [];
  let consumed = 0;
  let inserted = false;

  for (const currentToken of args.tokens) {
    const leadingText = composerTokenLeadingText(currentToken, args.leadingTexts);
    if (!inserted && offset <= consumed + leadingText.length) {
      const splitOffset = offset - consumed;
      const before = leadingText.slice(0, splitOffset);
      const after = leadingText.slice(splitOffset);
      nextTokens.push(
        before ? { ...args.token, leadingText: before } : { ...args.token },
        after
          ? { ...currentToken, leadingText: after }
          : (() => {
              const token = { ...currentToken };
              delete token.leadingText;
              return token;
            })(),
      );
      inserted = true;
    } else {
      nextTokens.push(
        leadingText
          ? { ...currentToken, leadingText }
          : (() => {
              const token = { ...currentToken };
              delete token.leadingText;
              return token;
            })(),
      );
    }
    consumed += leadingText.length + 1;
  }

  let value = args.value;
  if (!inserted) {
    const tailOffset = Math.max(0, Math.min(offset - consumed, value.length));
    const before = value.slice(0, tailOffset);
    value = value.slice(tailOffset);
    nextTokens.push(before ? { ...args.token, leadingText: before } : { ...args.token });
  }

  return {
    tokens: nextTokens,
    placements: Object.fromEntries(
      nextTokens.map((token) => [
        token.id,
        token.id === args.token.id ? "after" : (args.placements[token.id] ?? "after"),
      ]),
    ),
    leadingTexts: Object.fromEntries(
      nextTokens
        .filter((token) => Boolean(token.leadingText))
        .map((token) => [token.id, token.leadingText ?? ""]),
    ),
    value,
    focusOffset: offset + 1,
  };
}

export function removeComposerTextRange(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  start: number;
  end: number;
}): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
} {
  const start = Math.max(0, Math.min(args.start, args.end));
  const end = Math.max(start, args.end);
  let logicalOffset = 0;
  const tokens = args.tokens.map((token) => {
    const leadingText = composerTokenLeadingText(token, args.leadingTexts);
    const leadingEnd = logicalOffset + leadingText.length;
    if (start >= logicalOffset && end <= leadingEnd) {
      const nextLeadingText = `${leadingText.slice(0, start - logicalOffset)}${leadingText.slice(
        end - logicalOffset,
      )}`;
      logicalOffset = leadingEnd + 1;
      const nextToken = { ...token };
      if (nextLeadingText) {
        nextToken.leadingText = nextLeadingText;
      } else {
        delete nextToken.leadingText;
      }
      return nextToken;
    }
    logicalOffset = leadingEnd + 1;
    return leadingText ? { ...token, leadingText } : { ...token };
  });

  let value = args.value;
  const tailEnd = logicalOffset + value.length;
  if (start >= logicalOffset && end <= tailEnd) {
    value = `${value.slice(0, start - logicalOffset)}${value.slice(end - logicalOffset)}`;
  }

  return {
    tokens,
    placements: { ...args.placements },
    leadingTexts: Object.fromEntries(
      tokens
        .filter((token) => Boolean(token.leadingText))
        .map((token) => [token.id, token.leadingText ?? ""]),
    ),
    value,
  };
}

export function replaceComposerActionTriggerWithToken(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  trigger: { start: number; end: number };
  token: ComposerToken;
}) {
  const withoutTrigger = removeComposerTextRange({
    tokens: args.tokens,
    placements: args.placements,
    leadingTexts: args.leadingTexts,
    value: args.value,
    start: args.trigger.start,
    end: args.trigger.end,
  });
  if (withoutTrigger.tokens.some((token) => token.id === args.token.id)) {
    return { ...withoutTrigger, focusOffset: args.trigger.start };
  }
  return insertComposerTokenAtOffset({
    ...withoutTrigger,
    offset: args.trigger.start,
    token: args.token,
  });
}

export function promptTokenViews(tokens?: PromptNarrativeToken[]) {
  return tokens?.map((token) => ({
    id: token.id,
    type: token.type,
    label: token.label,
    leadingText: token.leadingText || undefined,
    title:
      typeof token.payload?.skillFilePath === "string" ? token.payload.skillFilePath : token.label,
  }));
}

export function promptTextAfterTokenLeadingText(
  text: string,
  tokens?: PromptNarrativeToken[],
): string {
  let remainingText = text;
  for (const token of tokens ?? []) {
    const leadingText = token.leadingText;
    if (!leadingText) {
      continue;
    }

    // The submitted prompt trims only its outer boundary. Prefer the exact
    // durable segment so whitespace between the token and the suffix survives,
    // then tolerate only the outer trim variants for token-only/indented input.
    const candidates = [
      leadingText,
      leadingText.trimStart(),
      leadingText.trimEnd(),
      leadingText.trim(),
    ].filter(
      (candidate, index, values) => candidate.length > 0 && values.indexOf(candidate) === index,
    );
    const matchedLeadingText = candidates.find((candidate) => remainingText.startsWith(candidate));
    if (!matchedLeadingText) {
      return text;
    }
    remainingText = remainingText.slice(matchedLeadingText.length);
  }
  return remainingText;
}

export function UserQuestionPrompt({
  question,
  disabled,
  onCancel,
  onSubmit,
}: UserQuestionPromptProps) {
  const [answers, setAnswers] = useState<Record<number, UserQuestionSelection>>({});
  const [activeQuestionIndex, setActiveQuestionIndex] = useState(0);
  const otherInputRefs = useRef<Record<number, HTMLInputElement | null>>({});
  const questions = question.questions;

  useEffect(() => {
    setAnswers({});
    setActiveQuestionIndex(0);
  }, [question.id]);

  useEffect(() => {
    setActiveQuestionIndex((current) => clampUserQuestionPromptIndex(current, questions.length));
  }, [questions.length]);

  if (questions.length === 0) {
    return null;
  }

  const questionCount = questions.length;
  const currentPageIndex = clampUserQuestionPromptIndex(activeQuestionIndex, questionCount);
  const activeQuestion = questions[currentPageIndex];
  const hasPreviousQuestion = currentPageIndex > 0;
  const hasNextQuestion = currentPageIndex < questionCount - 1;
  const hasMultipleQuestions = questionCount > 1;

  const currentAnswer = (index: number): UserQuestionSelection =>
    answers[index] ?? emptyUserQuestionSelection();

  const isSelected = (index: number, label: string): boolean =>
    currentAnswer(index).selected.includes(label);

  const goToQuestion = (index: number) => {
    setActiveQuestionIndex(clampUserQuestionPromptIndex(index, questionCount));
  };

  const goToPreviousQuestion = () => {
    goToQuestion(currentPageIndex - 1);
  };

  const goToNextQuestion = () => {
    goToQuestion(currentPageIndex + 1);
  };

  const toggleOption = (index: number, multiSelect: boolean, label: string) => {
    setAnswers((current) => {
      const entry = current[index] ?? emptyUserQuestionSelection();
      let selected: string[];
      if (multiSelect) {
        selected = entry.selected.includes(label)
          ? entry.selected.filter((item) => item !== label)
          : [...entry.selected, label];
      } else {
        selected = [label];
      }
      return {
        ...current,
        [index]: { ...entry, selected },
      };
    });
    if (label === USER_QUESTION_OTHER_OPTION) {
      window.setTimeout(() => otherInputRefs.current[index]?.focus(), 0);
    }
    if (!multiSelect && label !== USER_QUESTION_OTHER_OPTION) {
      window.setTimeout(() => {
        setActiveQuestionIndex((current) =>
          current === index ? nextUserQuestionPromptIndex(index, questionCount) : current,
        );
      }, 120);
    }
  };

  const setOtherText = (index: number, value: string) => {
    setAnswers((current) => {
      const entry = current[index] ?? emptyUserQuestionSelection();
      return {
        ...current,
        [index]: { ...entry, otherText: value },
      };
    });
  };

  const selectedLabelsForQuestion = (index: number): string[] => {
    return selectedUserQuestionLabels(currentAnswer(index));
  };

  const canSubmit = questions.every((_item, index) => selectedLabelsForQuestion(index).length > 0);
  const currentQuestionAnswered = isUserQuestionSelectionAnswered(currentAnswer(currentPageIndex));
  const answeredCount = questions.filter((_item, index) =>
    isUserQuestionSelectionAnswered(currentAnswer(index)),
  ).length;

  const handleSubmit = () => {
    if (disabled || !canSubmit) {
      return;
    }
    onSubmit(
      questions
        .map((item, index) => {
          const labels = selectedLabelsForQuestion(index);
          return `${item.question}: ${labels.join(", ")}`;
        })
        .join("\n"),
    );
  };

  const handlePrimaryAction = () => {
    if (disabled) {
      return;
    }
    if (hasNextQuestion) {
      if (currentQuestionAnswered) {
        goToNextQuestion();
      }
      return;
    }
    handleSubmit();
  };

  const isMulti = activeQuestion.multiSelect ?? false;
  const options = Array.isArray(activeQuestion.options) ? activeQuestion.options : [];
  const hasOther = isSelected(currentPageIndex, USER_QUESTION_OTHER_OPTION);
  const primaryActionDisabled =
    disabled || (hasNextQuestion ? !currentQuestionAnswered : !canSubmit);
  const primaryActionLabel = hasNextQuestion ? "下一题" : "提交回答";

  return (
    <div
      className="user-question-card"
      aria-label={`需要回答的问题 ${currentPageIndex + 1} of ${questionCount}`}
    >
      <div className="user-question-card__topbar">
        <div className="user-question-card__heading">
          {activeQuestion.header ? (
            <div className="user-question-card__header">{activeQuestion.header}</div>
          ) : null}
          <div className="user-question-card__title">{activeQuestion.question}</div>
        </div>
        <div className="user-question-card__topbar-actions">
          {hasMultipleQuestions ? (
            <div className="user-question-card__pager" aria-label="问题切换">
              <button
                type="button"
                className="user-question-card__pager-button"
                aria-label="上一题"
                title="上一题"
                onClick={goToPreviousQuestion}
                disabled={disabled || !hasPreviousQuestion}
              >
                <AwakenIcon concept="collapse-up" size={16} />
              </button>
              <span className="user-question-card__counter" aria-live="polite">
                {currentPageIndex + 1} of {questionCount}
              </span>
              <button
                type="button"
                className="user-question-card__pager-button"
                aria-label="下一题"
                title="下一题"
                onClick={goToNextQuestion}
                disabled={disabled || !hasNextQuestion}
              >
                <AwakenIcon concept="dropdown" size={16} />
              </button>
            </div>
          ) : null}
          <button
            type="button"
            className="user-question-card__close"
            aria-label="取消提问"
            title="取消提问"
            onClick={onCancel}
            disabled={disabled}
          >
            <AwakenIcon concept="close" size={20} />
          </button>
        </div>
      </div>
      <div className="user-question-card__body">
        <div
          className="user-question-card__question"
          key={`${currentPageIndex}:${activeQuestion.question}`}
        >
          <div className="user-question-card__options">
            {options.map((option) => (
              <button
                key={option.label}
                type="button"
                className={`user-question-card__option${
                  isSelected(currentPageIndex, option.label)
                    ? " user-question-card__option--selected"
                    : ""
                }`}
                onClick={() => toggleOption(currentPageIndex, isMulti, option.label)}
                disabled={disabled}
              >
                <span className="user-question-card__option-label">{option.label}</span>
                {option.description ? (
                  <span className="user-question-card__option-description">
                    {option.description}
                  </span>
                ) : null}
              </button>
            ))}
            <div
              className={`user-question-card__option user-question-card__option--other${
                hasOther ? " user-question-card__option--selected" : ""
              }${disabled ? " user-question-card__option--disabled" : ""}`}
              onClick={() => {
                if (!disabled) {
                  toggleOption(currentPageIndex, isMulti, USER_QUESTION_OTHER_OPTION);
                }
              }}
              onKeyDown={(event) => {
                if (disabled || (event.key !== "Enter" && event.key !== " ")) {
                  return;
                }
                event.preventDefault();
                toggleOption(currentPageIndex, isMulti, USER_QUESTION_OTHER_OPTION);
              }}
              role="button"
              aria-pressed={hasOther}
              tabIndex={disabled ? -1 : 0}
            >
              <span className="user-question-card__option-label">其他（输入你的答案）</span>
              {hasOther ? (
                <input
                  ref={(node) => {
                    otherInputRefs.current[currentPageIndex] = node;
                  }}
                  type="text"
                  className="user-question-card__other-input"
                  placeholder="请输入你的回答..."
                  value={currentAnswer(currentPageIndex).otherText}
                  onChange={(event) => setOtherText(currentPageIndex, event.target.value)}
                  disabled={disabled}
                  onClick={(event) => event.stopPropagation()}
                  onKeyDown={(event) => {
                    event.stopPropagation();
                    if (event.key !== "Enter") {
                      return;
                    }
                    event.preventDefault();
                    handlePrimaryAction();
                  }}
                />
              ) : null}
            </div>
          </div>
        </div>
      </div>
      <div className="user-question-card__actions">
        {hasMultipleQuestions ? (
          <span className="user-question-card__answer-status">
            已回答 {answeredCount} / {questionCount}
          </span>
        ) : null}
        <Button variant="default" size="sm" disabled={disabled} onClick={onCancel}>
          取消
        </Button>
        <Button
          variant="primary"
          size="sm"
          loading={disabled}
          disabled={primaryActionDisabled}
          onClick={() => {
            handlePrimaryAction();
          }}
        >
          {primaryActionLabel}
        </Button>
      </div>
    </div>
  );
}

export function shouldShowSidebarThreadSpinner(thread: ThreadItem): boolean {
  if (thread.runStatus === "running" || thread.runStatus === "cancelling") {
    return true;
  }
  if (thread.runStatus) {
    return false;
  }
  return Boolean(thread.activeRunId);
}

export function sidebarThreadArchiveMenuState(thread: ThreadItem): {
  disabled: boolean;
  disabledReason?: string;
} {
  if (shouldShowSidebarThreadSpinner(thread)) {
    return { disabled: true, disabledReason: "请先停止正在运行的任务" };
  }
  if (thread.source === "scheduled_task") {
    return { disabled: true, disabledReason: "定时任务会话由定时任务管理" };
  }
  return { disabled: false };
}

export function sidebarThreadMenuItems(
  thread: ThreadItem,
  actions: { onRename: () => void; onArchive: () => void; onDelete: () => void },
): ProjectMenuItem[] {
  const archiveState = sidebarThreadArchiveMenuState(thread);
  return [
    {
      id: "rename-thread",
      label: "重命名",
      onSelect: actions.onRename,
    },
    {
      id: "archive-thread",
      label: "归档",
      disabled: archiveState.disabled,
      disabledReason: archiveState.disabledReason,
      onSelect: actions.onArchive,
    },
    {
      id: "delete-thread",
      label: "删除会话",
      tone: "danger",
      onSelect: actions.onDelete,
    },
  ];
}

export function shouldResetActiveThreadForWorkspaceChange(
  activeThreadWorkspacePath?: string,
  nextWorkspacePath?: string,
): boolean {
  if (!activeThreadWorkspacePath || !nextWorkspacePath) {
    return false;
  }
  return !isSameWorkspacePath(activeThreadWorkspacePath, nextWorkspacePath);
}

export function resolveWorkspaceRootForRun(args: {
  workspacePath?: string;
  workspaceTargetPath?: string;
  conversationScope?: boolean;
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  defaultProjectRemoved?: boolean;
  activeThreadWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): string {
  const {
    workspacePath,
    workspaceTargetPath,
    conversationScope,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  } = args;

  // A project-less conversation is deliberately unbound. Core owns its
  // thread-keyed managed workspace, so neither a remembered default folder nor
  // an existing thread's resolved path may turn it back into a project run.
  if (conversationScope) {
    return "";
  }

  const explicitTarget = workspaceTargetPath?.trim();
  if (explicitTarget) {
    return explicitTarget;
  }

  const selectedProject =
    selectedProjectId === undefined || selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
      ? undefined
      : projectsFromWorkspace.find((project) => project.id === selectedProjectId);

  // A project the user created without picking a folder anchors nothing on disk.
  // Returning empty lets Core resolve that project's session workspace, instead
  // of falling through and silently borrowing the previous thread's directory.
  if (selectedProject && selectedProject.folders.length === 0) {
    return "";
  }

  const selectedProjectPath =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : selectedProject?.path;

  return (
    [
      selectedProjectPath,
      activeThreadWorkspacePath,
      workspacePath,
      defaultProjectRemoved ? undefined : defaultWorkspacePath,
    ]
      .find((candidate) => candidate?.trim())
      ?.trim() ?? ""
  );
}

/**
 * 决定一批选中的目录里哪些能加入项目。
 *
 * 文件夹选择器允许一次选中多个目录，所以整批因为其中一个不合格而失败是没必要的：
 * 不能加入的逐个说明原因，其余照常加入。已接受的目录也参与后续判重，这样同一批里
 * 指向同一位置的两个选择不会被加入两次。
 *
 * 目录属于哪些项目是用户的事，所以这里唯一会跳过的情况是它已经在当前这一组里，
 * 也就是加入了也不会产生任何变化。被别的项目持有不构成理由。
 */
export function partitionPickedProjectFolders(args: {
  candidates: string[];
  /** 已经挂在目标项目上的文件夹。 */
  attachedPaths: string[];
}): { accepted: string[]; skipped: string[] } {
  const accepted: string[] = [];
  const skipped: string[] = [];
  for (const candidate of args.candidates) {
    const label = workspaceLabelForPath(candidate);
    if (
      [...args.attachedPaths, ...accepted].some((taken) => isSameWorkspacePath(taken, candidate))
    ) {
      skipped.push(`“${label}”已经在这个项目里。`);
      continue;
    }
    accepted.push(candidate);
  }
  return { accepted, skipped };
}

/**
 * 废纸篓确认文案里的共享说明。
 *
 * 移到废纸篓真的会删掉磁盘目录，所以这个文件夹会同时从每一个持有它的项目里消失。
 * 别的项目也在用它时必须在确认之前说明，否则用户是在替另一个项目做决定而不自知。
 */
export function sharedFolderTrashWarning(args: {
  folderPath?: string;
  actingProjectId?: string;
  projects: { id: string; label: string; folders: { path: string }[] }[];
}): string {
  if (!args.folderPath) {
    return "";
  }
  const others = args.projects
    .filter(
      (project) =>
        project.id !== args.actingProjectId &&
        project.folders.some((folder) => isSameWorkspacePath(folder.path, args.folderPath)),
    )
    .map((project) => `“${project.label}”`);
  if (others.length === 0) {
    return "";
  }
  return `该文件夹同时属于${others.join("、")}，也会一并从这些项目中移除。`;
}

/**
 * run 的执行根集合：cwd 所属项目的全部文件夹。
 *
 * 匹配按「任意文件夹」而不是「主文件夹」进行。锚在次文件夹上的 run（继续旧线程、
 * 或切换过主文件夹之后）同样属于这个项目，只认主文件夹会让它塌回单根，把主文件夹
 * 和其余文件夹挤出 scope。cwd 排在首位，因为搜索类工具按这个顺序遍历根。
 *
 * 共享文件夹下同一个根会命中多个项目，这时候 projectId 决定用哪一组：run 的归属已经
 * 由 resolveRunProjectForRun 定过，roots 必须跟它取同一个项目，否则模型会拿到另一个
 * 项目的文件夹组。
 */
export function resolveRunRootsForRun(args: {
  workspaceRoot: string;
  projectsFromWorkspace: WorkspaceProject[];
  projectId?: string;
}): string[] {
  const root = normalizeWorkspacePath(args.workspaceRoot ?? "");
  if (!root) {
    return [];
  }
  const project =
    args.projectsFromWorkspace.find(
      (candidate) =>
        candidate.id === args.projectId &&
        candidate.folders.some((folder) => isSameWorkspacePath(folder.path, root)),
    ) ?? workspaceProjectForFolderPath(args.projectsFromWorkspace, root);
  if (!project) {
    return [root];
  }
  const roots = workspaceProjectRunRoots(project).filter(
    (candidate) => !isSameWorkspacePath(candidate, root),
  );
  return [root, ...roots];
}

/**
 * run 归属的项目身份与展示名。
 *
 * 两者一起解析，因为它们一起下发：`project_id` 让零文件夹项目找到自己的会话目录，
 * `project_name` 是模型侧「当前项目」那一行。只解析出 id 而漏掉名字，模型就只剩一堆
 * 路径，会重新退回到只认 cwd 的行为。
 *
 * 同一个目录可以同时属于多个项目，所以路径本身不再能确定身份：选中的项目只要确实
 * 持有这个根就优先采信，只有在选中项与这个根无关时（比如继续一条旧线程）才回退到
 * 按路径查找。匹配按「任意文件夹」进行，锚在次文件夹上的线程同样算这个项目。
 */
export function resolveRunProjectForRun(args: {
  workspaceRoot: string;
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  effectiveDefaultWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): { projectId?: string; projectName?: string } {
  const {
    workspaceRoot,
    selectedProjectId,
    defaultWorkspacePath,
    effectiveDefaultWorkspacePath,
    projectsFromWorkspace,
  } = args;
  const selectedHoldsRoot =
    selectedProjectId !== undefined &&
    selectedProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
    projectsFromWorkspace
      .find((project) => project.id === selectedProjectId)
      ?.folders.some((folder) => isSameWorkspacePath(folder.path, workspaceRoot)) === true;
  // 没有根就只能靠选中项来定身份，那正是零文件夹项目的情形。
  const projectId = !workspaceRoot
    ? selectedProjectId
    : selectedHoldsRoot
      ? selectedProjectId
      : isSameWorkspacePath(workspaceRoot, effectiveDefaultWorkspacePath)
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : workspaceProjectForFolderPath(projectsFromWorkspace, workspaceRoot)?.id;
  if (!projectId) {
    return {};
  }
  if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
    return {
      projectId,
      projectName: defaultWorkspacePath ? workspaceLabelForPath(defaultWorkspacePath) : "awaken",
    };
  }
  return {
    projectId,
    projectName: projectsFromWorkspace.find((project) => project.id === projectId)?.label,
  };
}

function sameWorkspaceProjectFolders(
  current: WorkspaceProjectFolder[],
  next: WorkspaceProjectFolder[],
): boolean {
  return (
    next.length === current.length &&
    next.every((folder, index) => {
      const existing = current[index];
      return (
        existing?.path === folder.path &&
        existing.role === folder.role &&
        existing.isPrimary === folder.isPrimary
      );
    })
  );
}

function sameWorkspaceProjects(current: WorkspaceProject[], next: WorkspaceProject[]): boolean {
  return (
    next.length === current.length &&
    next.every((project, index) => {
      const existing = current[index];
      return (
        existing?.id === project.id &&
        existing.backendProjectId === project.backendProjectId &&
        existing.path === project.path &&
        existing.label === project.label &&
        sameWorkspaceProjectFolders(existing.folders, project.folders)
      );
    })
  );
}

export function resolveBackendProjectOrder(
  ids: string[],
  defaultBackendProjectId: string | undefined,
  projects: WorkspaceProject[],
): string[] | undefined {
  const backendById = new Map(projects.map((project) => [project.id, project.backendProjectId]));
  const backendIds = ids.map((id) =>
    id === DEFAULT_WORKSPACE_PROJECT_ID ? defaultBackendProjectId : backendById.get(id),
  );
  return backendIds.every((id): id is string => Boolean(id)) ? backendIds : undefined;
}

function formatTokenCount(value?: number): string | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value >= 1_000_000) {
    const amount = value / 1_000_000;
    return `${Number.isInteger(amount) ? amount.toFixed(0) : amount.toFixed(1)}M`;
  }
  if (value >= 1000) {
    const amount = value / 1000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}k`;
  }
  return String(value);
}

export function subagentElapsedMs(
  task: Pick<SubagentNarrativeTask, "startedAt" | "finishedAt">,
  nowMs: number,
): number | undefined {
  const startedAt = task.startedAt ? Date.parse(task.startedAt) : Number.NaN;
  const finishedAt = task.finishedAt ? Date.parse(task.finishedAt) : nowMs;
  if (!Number.isFinite(startedAt) || !Number.isFinite(finishedAt) || finishedAt < startedAt) {
    return undefined;
  }
  return finishedAt - startedAt;
}

export function subagentWorkspaceModeLabel(mode?: string): string | undefined {
  switch (mode) {
    case "shared":
      return "共享可写";
    case "git_worktree":
      return "Git worktree";
    case "shared_readonly":
      return "只读共享";
    default:
      return mode || undefined;
  }
}

function formatSubagentElapsed(elapsedMs?: number): string | undefined {
  if (typeof elapsedMs !== "number" || !Number.isFinite(elapsedMs) || elapsedMs < 0) {
    return undefined;
  }
  const totalSeconds = Math.max(1, Math.round(elapsedMs / 1000));
  if (totalSeconds < 60) return `${totalSeconds} 秒`;
  return `${Math.floor(totalSeconds / 60)} 分 ${totalSeconds % 60} 秒`;
}

export function subagentAgentTypeLabel(
  agentType: SubagentNarrativeTask["agentType"],
): string {
  switch (agentType) {
    case "review":
      return "复核智能体";
    case "plan":
      return "规划智能体";
    case "general":
      return "通用智能体";
    default:
      return "探索智能体";
  }
}

function rawToolId(selectionId: string): string {
  if (selectionId.startsWith("tool:")) {
    return selectionId.slice("tool:".length);
  }
  if (selectionId.startsWith("explore:")) {
    return selectionId.slice("explore:".length);
  }
  return selectionId;
}

export function toolRecordIdForNarrativeItem(item: {
  id: string;
  toolCallId?: string;
}): string {
  return item.toolCallId || rawToolId(item.id);
}

type RuntimeContextWindow = ReturnType<
  typeof createInitialRuntimeState
>["inspectorSummary"]["contextWindow"];

type RuntimeModelUsage = NonNullable<NonNullable<RuntimeContextWindow>["latestModelUsage"]>;

type RuntimeModelIdentity = {
  id?: string;
  providerId?: string;
  credentialRevision?: number;
  modelId?: string;
  capabilityModelId?: string;
  endpointSelector?: string;
  modelSelectorKind?: "model" | "endpoint";
  protocol?: string;
  reasoningEffort?: ReasoningEffortLevel;
};

function normalizedTokenCount(value: number | undefined): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    return undefined;
  }
  return Math.trunc(value);
}

function safeTokenSum(...values: number[]): number | undefined {
  const total = values.reduce((sum, value) => sum + value, 0);
  return Number.isSafeInteger(total) && total >= 0 ? total : undefined;
}

function providerCurrentInputTokens(usage: RuntimeModelUsage | undefined): number | undefined {
  if (
    !usage ||
    usage.purpose !== "agent_turn" ||
    usage.status !== "succeeded" ||
    !usage.usageComplete
  ) {
    return undefined;
  }

  const input = normalizedTokenCount(usage.inputTokens);
  if (input !== undefined) {
    return input;
  }

  const cached = normalizedTokenCount(usage.cachedInputTokens);
  const uncached = normalizedTokenCount(usage.uncachedInputTokens);
  if (cached === undefined || uncached === undefined) {
    return undefined;
  }
  const cacheWrite = normalizedTokenCount(usage.cacheWriteTokens) ?? 0;
  return safeTokenSum(cached, uncached, cacheWrite);
}

const PROVIDER_INPUT_COMPATIBLE_EPOCH_BOUNDARIES = new Set([
  "system_prompt_changed",
  "tool_removed",
  "tool_schema_changed",
  "canonical_tool_added",
]);

function usageMatchesSnapshot(
  usage: RuntimeModelUsage | undefined,
  snapshot: NonNullable<RuntimeContextWindow>["snapshot"],
  currentModel?: RuntimeModelIdentity,
): boolean {
  if (!usage) {
    return false;
  }
  if (
    currentModel &&
    ((usage.profileId && currentModel.id && usage.profileId !== currentModel.id) ||
      (usage.providerId &&
        currentModel.providerId &&
        usage.providerId !== currentModel.providerId) ||
      (usage.credentialRevision !== undefined &&
        currentModel.credentialRevision !== undefined &&
        usage.credentialRevision !== currentModel.credentialRevision) ||
      (usage.modelId && currentModel.modelId && usage.modelId !== currentModel.modelId) ||
      (usage.capabilityModelId &&
        currentModel.capabilityModelId &&
        usage.capabilityModelId !== currentModel.capabilityModelId) ||
      (usage.endpointSelector &&
        currentModel.endpointSelector &&
        usage.endpointSelector !== currentModel.endpointSelector) ||
      (usage.modelSelectorKind &&
        currentModel.modelSelectorKind &&
        usage.modelSelectorKind !== currentModel.modelSelectorKind) ||
      (usage.protocolFamily &&
        currentModel.protocol &&
        usage.protocolFamily !== currentModel.protocol) ||
      (usage.reasoningPolicy &&
        currentModel.reasoningEffort &&
        usage.reasoningPolicy !== currentModel.reasoningEffort))
  ) {
    return false;
  }
  if (!snapshot) {
    return true;
  }
  if (usage.modelId && snapshot.modelId && usage.modelId !== snapshot.modelId) {
    return false;
  }
  if (
    (usage.capabilityModelId &&
      snapshot.capabilityModelId &&
      usage.capabilityModelId !== snapshot.capabilityModelId) ||
    (usage.endpointSelector &&
      snapshot.endpointSelector &&
      usage.endpointSelector !== snapshot.endpointSelector) ||
    (usage.modelSelectorKind &&
      snapshot.modelSelectorKind &&
      usage.modelSelectorKind !== snapshot.modelSelectorKind)
  ) {
    return false;
  }
  if (
    usage.contextEpochId &&
    snapshot.contextEpochId &&
    usage.contextEpochId !== snapshot.contextEpochId &&
    snapshot.contextEpochBoundaryReason !== undefined &&
    !PROVIDER_INPUT_COMPATIBLE_EPOCH_BOUNDARIES.has(
      snapshot.contextEpochBoundaryReason,
    )
  ) {
    return false;
  }
  return !(
    usage.protocolFamily &&
    snapshot.protocolFamily &&
    usage.protocolFamily !== snapshot.protocolFamily
  );
}

function compatibleProviderInputTokens(
  usage: RuntimeModelUsage | undefined,
  snapshot: NonNullable<RuntimeContextWindow>["snapshot"],
  currentModel?: RuntimeModelIdentity,
): number | undefined {
  if (!usageMatchesSnapshot(usage, snapshot, currentModel)) {
    return undefined;
  }
  return providerCurrentInputTokens(usage);
}

function currentContextTokens(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): number | undefined {
  const usage = contextWindow?.latestModelUsage;
  const providerInput = compatibleProviderInputTokens(
    usage,
    contextWindow?.snapshot,
    currentModel,
  );
  return (
    providerInput ??
    normalizedTokenCount(contextWindow?.snapshot?.estimatedInputTokens) ??
    normalizedTokenCount(contextWindow?.estimatedTokens)
  );
}

export function contextUsageMeta(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const usageValue = currentContextTokens(contextWindow, currentModel);
  const usage = formatTokenCount(usageValue);
  const effectiveWindowValue =
    contextWindow?.snapshot?.effectiveInputWindowTokens ??
    contextWindow?.effectiveWindow ??
    (typeof contextWindow?.modelWindow === "number"
      ? Math.max(0, contextWindow.modelWindow - (contextWindow.outputReserve ?? 0))
      : undefined);
  const effectiveWindow = formatTokenCount(effectiveWindowValue);
  if (usage && effectiveWindow) {
    const progress =
      typeof usageValue === "number" &&
      typeof effectiveWindowValue === "number" &&
      effectiveWindowValue > 0
        ? usageValue / effectiveWindowValue
        : undefined;
    return {
      label: "上下文窗口",
      value: `${usage} / ${effectiveWindow}`,
      progress,
    };
  }
  if (usage) {
    return {
      label: "上下文窗口",
      value: usage,
    };
  }
  return undefined;
}

const CONTEXT_PANEL_CONTRIBUTIONS = [
  { id: "system_prompt", label: "系统提示词" },
  { id: "builtin_tools", label: "内置工具" },
  { id: "mcp", label: "MCP" },
  { id: "skills", label: "技能" },
  { id: "memory", label: "记忆" },
  { id: "conversation", label: "对话" },
] as const;

const CONTEXT_PANEL_OTHER_CONTRIBUTION = {
  id: "other",
  label: "其他/协议开销",
} as const;

function proportionallyAllocateTokens(values: number[], target: number): number[] {
  const sourceTotal = values.reduce((sum, value) => sum + value, 0);
  if (target <= 0) {
    return values.map(() => 0);
  }
  if (sourceTotal <= 0) {
    return values.map((_, index) => (index === values.length - 1 ? target : 0));
  }

  const exact = values.map((value) => (value * target) / sourceTotal);
  const allocated = exact.map((value) => Math.floor(value));
  let remainder = target - allocated.reduce((sum, value) => sum + value, 0);
  const byFraction = exact
    .map((value, index) => ({ index, fraction: value - Math.floor(value) }))
    .sort((left, right) => right.fraction - left.fraction || left.index - right.index);
  for (let index = 0; remainder > 0; index += 1, remainder -= 1) {
    allocated[byFraction[index % byFraction.length].index] += 1;
  }
  return allocated;
}

function reconcileContributionTokens(
  values: number[],
  currentTokens: number,
): { values: number[]; otherTokens: number } {
  const sourceTotal = values.reduce((sum, value) => sum + value, 0);
  const delta = currentTokens - sourceTotal;
  if (delta === 0) {
    return { values, otherTokens: 0 };
  }
  if (delta > 0) {
    return { values, otherTokens: delta };
  }
  return {
    values: proportionallyAllocateTokens(values, currentTokens),
    otherTokens: 0,
  };
}

export function contextUsagePanelData(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): Omit<AwakenContextUsagePanelProps, "id" | "onClose"> | undefined {
  if (!contextWindow) {
    return undefined;
  }
  const snapshot = contextWindow.snapshot;
  const currentInputTokens = currentContextTokens(contextWindow, currentModel);
  const outputReserveTokens = snapshot?.outputReserveTokens ?? contextWindow.outputReserve ?? 0;
  const modelWindowTokens = snapshot?.modelWindowTokens ?? contextWindow.modelWindow;
  const effectiveInputWindowTokens =
    snapshot?.effectiveInputWindowTokens ??
    contextWindow.effectiveWindow ??
    (typeof modelWindowTokens === "number"
      ? Math.max(0, modelWindowTokens - outputReserveTokens)
      : undefined);
  if (
    typeof currentInputTokens !== "number" ||
    typeof modelWindowTokens !== "number" ||
    typeof effectiveInputWindowTokens !== "number"
  ) {
    return undefined;
  }
  const rawContributions = snapshot?.contributions ?? [];
  const supportedKinds = new Set<string>(
    CONTEXT_PANEL_CONTRIBUTIONS.map((contribution) => contribution.id),
  );
  const hasRetainedContributionContract =
    ((snapshot?.schemaVersion === 4 && snapshot.estimatorId === "awaken_leaf_v4") ||
      (snapshot?.schemaVersion === 3 && snapshot.estimatorId === "awaken_leaf_v3")) &&
    rawContributions.every((contribution) => supportedKinds.has(contribution.kind));
  const tokensByKind = new Map<string, number>();
  if (hasRetainedContributionContract) {
    for (const contribution of rawContributions) {
      tokensByKind.set(
        contribution.kind,
        (tokensByKind.get(contribution.kind) ?? 0) + contribution.estimatedTokens,
      );
    }
  }
  const classifiedTokens = Array.from(tokensByKind.values()).reduce(
    (sum, tokens) => sum + tokens,
    0,
  );
  const hasCompleteBreakdown =
    rawContributions.length > 0 &&
    hasRetainedContributionContract &&
    classifiedTokens === snapshot?.estimatedInputTokens;
  const reconciled = hasCompleteBreakdown
    ? reconcileContributionTokens(
        CONTEXT_PANEL_CONTRIBUTIONS.map((contribution) => tokensByKind.get(contribution.id) ?? 0),
        currentInputTokens,
      )
    : { values: [], otherTokens: 0 };

  const contributions: Array<{
    id: string;
    label: string;
    estimatedTokens: number;
  }> = hasCompleteBreakdown
    ? CONTEXT_PANEL_CONTRIBUTIONS.map((contribution, index) => ({
        ...contribution,
        estimatedTokens: reconciled.values[index],
      }))
    : [];
  if (hasCompleteBreakdown && reconciled.otherTokens > 0) {
    contributions.push({
      ...CONTEXT_PANEL_OTHER_CONTRIBUTION,
      estimatedTokens: reconciled.otherTokens,
    });
  }
  return {
    currentInputTokens,
    effectiveInputWindowTokens,
    modelWindowTokens,
    outputReserveTokens,
    autoCompactThresholdTokens:
      snapshot?.autoCompactThresholdTokens ?? contextWindow.autoCompactThreshold ?? 0,
    contributions,
    latestModelUsage: contextWindow.latestModelUsage
      ? {
          status: contextWindow.latestModelUsage.status,
          cacheMode: contextWindow.latestModelUsage.cacheMode,
          cacheKeyApplied: contextWindow.latestModelUsage.cacheKeyApplied,
          cachedInputTokens: contextWindow.latestModelUsage.cachedInputTokens,
          uncachedInputTokens: contextWindow.latestModelUsage.uncachedInputTokens,
          cacheWriteTokens: contextWindow.latestModelUsage.cacheWriteTokens,
          inputTokens: contextWindow.latestModelUsage.inputTokens,
          outputTokens: contextWindow.latestModelUsage.outputTokens,
          reasoningTokens: contextWindow.latestModelUsage.reasoningTokens,
          totalTokens: contextWindow.latestModelUsage.totalTokens,
          calculatedCostUsd: contextWindow.latestModelUsage.calculatedCostUsd,
          currency: contextWindow.latestModelUsage.currency,
          invalidationReason:
            contextWindow.latestModelUsage.cacheMode &&
            contextWindow.latestModelUsage.invalidationReason !== "initial_baseline"
              ? contextWindow.latestModelUsage.invalidationReason
              : undefined,
          purpose: contextWindow.latestModelUsage.purpose,
          errorCode: contextWindow.latestModelUsage.errorCode,
        }
      : null,
  };
}

export function modelProfileLabel(
  profile: Pick<
    RuntimeModelProfileSummary,
    "modelId" | "modelDisplayName" | "displayName"
  >,
): string {
  return profile.modelDisplayName || profile.modelId || profile.displayName;
}

export function modelPreferenceChangeForOption(
  profileId: string,
  selectedProfileId: string,
  reasoningEffort?: ReasoningEffortLevel,
): { modelProfileId: string; reasoningEffort?: ReasoningEffortLevel } | undefined {
  if (profileId === selectedProfileId) return undefined;
  return { modelProfileId: profileId, reasoningEffort };
}

export function modelPreferenceRecoveryKey(
  threadId: string | undefined,
  persistedModelProfileId: string | undefined,
  selectedModelProfileId: string | undefined,
  persistedReasoningEffort: ReasoningEffortLevel | undefined,
  selectedReasoningEffort: ReasoningEffortLevel | undefined,
): string {
  if (!threadId || !selectedModelProfileId) return "";

  const persistedModelIsUnavailable =
    persistedModelProfileId !== undefined && persistedModelProfileId !== selectedModelProfileId;
  const persistedEffortIsUnavailable =
    persistedReasoningEffort !== undefined &&
    persistedReasoningEffort !== selectedReasoningEffort;
  if (!persistedModelIsUnavailable && !persistedEffortIsUnavailable) return "";

  return `${threadId}:${persistedModelProfileId ?? ""}:${persistedReasoningEffort ?? ""}`;
}

function modelProfileIcon(profile: RuntimeModelProfileSummary) {
  const brand = providerBrandForId(profile.providerId);
  return brand ? (
    <AwakenProviderLogo brand={brand} name={profile.displayName} />
  ) : profile.displayName.trim().slice(0, 1) || "M";
}

function firstContentLine(value?: string): string | undefined {
  return value
    ?.split("\n")
    .map((line) => line.trim())
    .find(Boolean);
}

function compactToolContext(value?: string): string | undefined {
  const line = firstContentLine(value);
  if (!line || line === "{" || line === "[" || /^[\[{]\s*"/.test(line)) return undefined;
  return line;
}

type ToolTraceRecordView = {
  title?: string;
  content?: string;
  args?: string;
  commandOutput?: string;
  commandOutputTruncated?: boolean;
  status?: ToolDisplayState;
  diagnostic?: Record<string, unknown>;
  providerId?: string;
  providerSource?: string;
};

type ToolTracePatchView = {
  title?: string;
  patch: string;
  summary?: string;
  affectedPaths: string[];
  truncated?: boolean;
};

const SENSITIVE_TOOL_KEY = /(?:password|token|secret|api[_-]?key|authorization|cookie)/i;
const SHELL_TOOL_TITLE = /(?:bash|shell|terminal|command|命令)/i;
const READ_TOOL_TITLE = /(?:read|读取|list|列出|search|搜索|grep|glob|find)/i;
const FILE_TOOL_TITLE = /(?:write|edit|patch|create|delete|move|copy|写入|编辑|修改|创建|删除)/i;

function parseToolArguments(value?: string): unknown {
  if (!value?.trim()) return undefined;
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function redactSensitiveValue(value: unknown, key = ""): unknown {
  if (SENSITIVE_TOOL_KEY.test(key)) return "••••";
  if (Array.isArray(value)) return value.map((entry) => redactSensitiveValue(entry));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([childKey, childValue]) => [
        childKey,
        redactSensitiveValue(childValue, childKey),
      ]),
    );
  }
  return value;
}

function redactShellText(value: string): string {
  return value.replace(
    /\b([A-Z0-9_]*(?:PASSWORD|TOKEN|SECRET|API_KEY|AUTHORIZATION|COOKIE)[A-Z0-9_]*)=(?:"[^"]*"|'[^']*'|[^\s]+)/gi,
    "$1=••••",
  );
}

function argumentRecord(value?: string): Record<string, unknown> | undefined {
  const parsed = parseToolArguments(value);
  return parsed && typeof parsed === "object" && !Array.isArray(parsed)
    ? (parsed as Record<string, unknown>)
    : undefined;
}

function firstStringArgument(
  args: Record<string, unknown> | undefined,
  keys: string[],
): string | undefined {
  for (const key of keys) {
    const value = args?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return undefined;
}

function safeFileToolArguments(
  args: Record<string, unknown> | undefined,
): Record<string, string> | undefined {
  const path = firstStringArgument(args, ["path", "file_path", "filePath", "target"]);
  return path ? { path } : undefined;
}

function compactInlineLabel(value: string, maxLength = 72): string {
  const compact = value.replace(/\s+/g, " ").trim();
  if (compact.length <= maxLength) return compact;
  return `${compact.slice(0, Math.max(1, maxLength - 1)).trimEnd()}…`;
}

function compactPathLabel(value: string): string {
  const normalized = value.trim().replace(/[\\/]+$/, "");
  if (!normalized) return value.trim();
  const segments = normalized.split(/[\\/]/).filter(Boolean);
  return compactInlineLabel(segments.at(-1) || normalized);
}

function compactToolFailureContext(value?: string): string | undefined {
  const raw = value?.trim();
  if (!raw) return undefined;
  if (/permission|denied|outside writable roots?|sensitive path/i.test(raw)) {
    return "权限被拒绝";
  }
  if (/timed?\s*out|timeout/i.test(raw)) return "执行超时";
  if (/not found|no such file|enoent/i.test(raw)) return "目标不存在";
  if (/cancelled|canceled/i.test(raw)) return "已取消";
  if (/interrupted/i.test(raw)) return "已中断";
  const withoutQuotedPaths = redactShellText(raw).replace(
    /(["'])(?:\/|[A-Za-z]:\\)[^"'\n]+\1/g,
    "$1…$1",
  );
  return compactInlineLabel(withoutQuotedPaths);
}

function detailCardFromSections(
  title: string,
  sections: ToolTraceDetailCard["sections"],
): ToolTraceDetailCard | undefined {
  const visible = sections.filter((section) => section.content.trim());
  if (visible.length === 0) return undefined;
  return {
    title,
    sections: visible,
    copyText: visible.map((section) => `${section.label}\n${section.content}`).join("\n\n"),
  };
}

function naturalToolTitle(
  title: string,
  kind: "tool" | "mcp" | "terminal",
  args?: Record<string, unknown>,
): string {
  if (kind === "terminal") {
    const description = firstStringArgument(args, ["description", "label", "summary"]);
    if (description) return `运行 ${compactInlineLabel(description)}`;
    const command = firstStringArgument(args, ["command", "cmd", "script"]);
    if (command) return `运行 ${compactInlineLabel(redactShellText(command))}`;
    return "运行命令";
  }
  if (READ_TOOL_TITLE.test(title)) {
    const path = firstStringArgument(args, ["path", "file_path", "filePath"]);
    const query = firstStringArgument(args, ["query", "pattern", "url"]);
    const glob = firstStringArgument(args, ["glob"]);
    if (/search|grep|glob|find|搜索/i.test(title)) {
      const compactTitle = title.replace(/\s*\{\s*$/, "").trim();
      if (/^搜索/.test(compactTitle) && !/[*?[\]{}]/.test(compactTitle)) {
        return compactTitle;
      }
      const queryLabel = query
        ? /[*?[\]{}]/.test(query)
          ? describeGlobPattern(query)
          : `"${compactInlineLabel(query, 48)}"`
        : undefined;
      const scopeLabel = glob
        ? describeGlobPattern(glob)
        : path
          ? compactPathLabel(path)
          : undefined;
      if (queryLabel && scopeLabel) {
        return `搜索 ${queryLabel} · ${scopeLabel}`;
      }
      if (queryLabel) return `搜索 ${queryLabel}`;
      if (scopeLabel) return `搜索 ${scopeLabel}`;
      return "搜索文件";
    }
    if (path) {
      return `${/list|列出/i.test(title) ? "列出" : "读取"} ${compactPathLabel(path)}`;
    }
    return /list|列出/i.test(title) ? "列出文件" : "读取文件";
  }
  if (FILE_TOOL_TITLE.test(title)) {
    const target = firstStringArgument(args, ["path", "file_path", "filePath", "target"]);
    if (!target && /^(?:写入|编辑|修改|创建|删除|移动|复制)(?:文件|\s)/.test(title)) {
      return compactInlineLabel(title);
    }
    const action = /delete|删除/i.test(title)
      ? "删除"
      : /move|移动/i.test(title)
        ? "移动"
        : /copy|复制/i.test(title)
          ? "复制"
          : /write|create|写入|创建/i.test(title)
            ? "写入"
            : "编辑";
    if (target) return `${action} ${compactPathLabel(target)}`;
    return `${action}文件`;
  }
  return title.replace(/\s*\{\s*$/, "").trim() || "使用工具";
}

export function toolTracePresentation(
  tool: ToolTraceRecordView | undefined,
  fallback?: string,
  options: {
    title?: string;
    patch?: ToolTracePatchView;
    allowLegacyFallbackOutput?: boolean;
    status?: ToolDisplayState;
  } = {},
) {
  if (!tool) {
    const legacyContent = fallback?.trim() || undefined;
    const statusNeedsContext = Boolean(
      options.status && options.status !== "done",
    );
    const detailCard = options.allowLegacyFallbackOutput && legacyContent
      ? detailCardFromSections("工具详情", [
          {
            label: options.status && options.status !== "active" && options.status !== "done"
              ? "错误与恢复信息"
              : "结果",
            content: legacyContent,
            format: "text",
            ...(options.status && options.status !== "active" && options.status !== "done"
              ? { tone: "danger" as const }
              : {}),
          },
        ])
      : undefined;
    return {
      ...(options.title ? { title: options.title.replace(/\s*\{\s*$/, "").trim() } : {}),
      detail: options.status === "active"
        ? compactToolContext(fallback)
        : statusNeedsContext
          ? compactToolFailureContext(fallback)
          : undefined,
      output: undefined,
      outputLabel: "工具输出",
      detailCard,
      kind: "tool" as const,
    };
  }
  const commandOutput = tool.commandOutput?.trim();
  const result = tool.content?.trim();
  const active = !tool.status || tool.status === "active";
  const failed = Boolean(tool.status && tool.status !== "active" && tool.status !== "done");
  const output = active
    ? commandOutput || undefined
    : commandOutput || result || undefined;
  const providerSource = tool.providerSource?.trim().toLowerCase() || "";
  const mcpProvider = providerSource === "mcp" || providerSource === "connector";
  const sourceTitle = options.title || tool.title || "使用工具";
  const args = argumentRecord(tool.args);
  const shellTool = Boolean(commandOutput) || SHELL_TOOL_TITLE.test(sourceTitle);
  const kind = shellTool ? "terminal" : mcpProvider ? "mcp" : "tool";
  const providerLabel = mcpProvider
    ? tool.providerId?.replace(/^(mcp|connector):/i, "")
    : undefined;
  const progress = active
    ? compactToolContext(fallback) || compactToolContext(commandOutput) || compactToolContext(result)
    : compactToolContext(result) || compactToolContext(commandOutput) || compactToolContext(fallback);
  const command = shellTool
    ? firstStringArgument(args, ["command", "cmd", "script"])
    : undefined;
  const knownReadableTool = READ_TOOL_TITLE.test(sourceTitle);
  const knownFileTool = FILE_TOOL_TITLE.test(sourceTitle) || Boolean(options.patch);
  const sanitizedArgs = mcpProvider
    ? args
      ? redactSensitiveValue(args)
      : undefined
    : knownFileTool
      ? safeFileToolArguments(args)
      : args
        ? redactSensitiveValue(args)
        : undefined;
  const argumentContent = sanitizedArgs ? JSON.stringify(sanitizedArgs, null, 2) : "";
  const errorContent = tool.diagnostic ? JSON.stringify(redactSensitiveValue(tool.diagnostic), null, 2) : "";
  const sections: ToolTraceDetailCard["sections"] = [];

  if (command) {
    sections.push({
      label: "命令",
      content: redactShellText(command),
      format: "code",
      tone: "command",
    });
  } else if ((mcpProvider || knownReadableTool || knownFileTool) && argumentContent) {
    sections.push({ label: mcpProvider ? "脱敏参数" : "目标与参数", content: argumentContent, format: "json" });
  }
  if (options.patch) {
    if (options.patch.affectedPaths.length > 0) {
      sections.push({
        label: "文件",
        content: options.patch.affectedPaths.join("\n"),
        format: "text",
      });
    }
    sections.push({ label: "变更", content: options.patch.patch, format: "diff" });
  }
  if (commandOutput) {
    sections.push({ label: "stdout / stderr", content: redactShellText(commandOutput), format: "code" });
  } else if (result) {
    sections.push({ label: mcpProvider ? "结果" : knownReadableTool ? "匹配摘要" : "结果", content: result, format: "text" });
  }
  if (errorContent) {
    sections.push({
      label: failed ? "错误与恢复信息" : "执行环境",
      content: errorContent,
      format: "json",
      tone: failed ? "danger" : "muted",
    });
  }
  const detailCard = detailCardFromSections(
    kind === "terminal" ? "Shell 详情" : mcpProvider ? `${providerLabel || "MCP"} 详情` : "工具详情",
    sections,
  );
  const patchStats = options.patch ? diffLineStats(options.patch.patch) : undefined;
  const lineChangeDetail = patchStats
    ? `+${patchStats.added} −${patchStats.removed} 行`
    : undefined;
  const preparationDetail = active && /^\+\d+(?: −\d+)? 行$/.test(progress ?? "")
    ? progress
    : undefined;
  return {
    title: naturalToolTitle(sourceTitle, kind, args),
    detail:
      providerLabel ||
      (failed ? compactToolFailureContext(progress) : lineChangeDetail || preparationDetail),
    output,
    outputLabel: kind === "terminal" ? "终端输出" : mcpProvider ? "MCP 输出" : "工具输出",
    outputTruncated: tool.commandOutputTruncated,
    detailCard,
    kind: kind as "tool" | "mcp" | "terminal",
  };
}

type ActivitySummaryInput = {
  kind: "tool" | "skill" | "terminal" | "mcp";
  title: string;
  status: ToolDisplayState;
  fileCount?: number;
};

export function activityBatchSummary(actions: ActivitySummaryInput[]): {
  label: string;
  detail?: string;
} {
  const active = actions.some((action) => action.status === "active");
  const failed = actions.filter((action) => action.status !== "active" && action.status !== "done").length;
  const commandCount = actions.filter((action) => action.kind === "terminal").length;
  const skillActions = actions.filter((action) => action.kind === "skill");
  const checkedCount = actions.filter((action) => READ_TOOL_TITLE.test(action.title)).length;
  const editedFiles = actions.reduce((count, action) => count + (action.fileCount || 0), 0);
  let label: string;
  if (commandCount === actions.length) {
    label = active ? `正在运行 ${commandCount} 条命令` : `运行了 ${commandCount} 条命令`;
  } else if (checkedCount === actions.length) {
    label = active ? `正在检查 ${checkedCount} 项` : `检查了 ${checkedCount} 项`;
  } else if (editedFiles > 0 && actions.every((action) => FILE_TOOL_TITLE.test(action.title))) {
    label = active ? `正在编辑 ${editedFiles} 个文件` : `编辑了 ${editedFiles} 个文件`;
  } else if (skillActions.length > 0) {
    const skillName = skillActions[0].title.replace(/^使用\s*/i, "");
    const toolCount = actions.length - skillActions.length;
    const toolSuffix = toolCount > 0 ? ` · ${toolCount} 个工具` : "";
    label = `${active ? "正在使用" : "使用了"} ${skillName}${toolSuffix}`;
  } else {
    label = active ? `正在运行 ${actions.length} 个工具` : `使用了 ${actions.length} 个工具`;
  }
  return { label, detail: failed > 0 ? `${failed} 个异常` : undefined };
}

function composerThreadLabel(
  prompt: string,
  attachments: Array<{ displayName?: string }>,
  tokens: ComposerToken[] = [],
): string {
  const trimmed = prompt.trim();
  if (trimmed) {
    return trimmed;
  }
  const attachmentName = attachments
    .map((attachment) => attachment.displayName?.trim())
    .find(Boolean);
  if (attachmentName) {
    return attachmentName;
  }
  const tokenLabel = tokens.map((token) => token.label.trim()).find(Boolean);
  return tokenLabel || "附件任务";
}

const DEFAULT_SIDEBAR_WIDTH = 300;
const MIN_SIDEBAR_WIDTH = 228;
const MAX_SIDEBAR_WIDTH = 420;
const LAYOUT_STEP = 12;
const INSPECTOR_RESPONSIVE_COLLAPSE_QUERY = "(max-width: 1180px)";
const INSPECTOR_FILE_ITEMS = [
  {
    id: "files",
    label: "文件",
    tone: "neutral" as const,
    icon: <AwakenIcon concept="project" size={16} />,
  },
  {
    id: "browser",
    label: "浏览器",
    tone: "neutral" as const,
    icon: <AwakenIcon concept="browser" size={16} />,
  },
];
const STORED_THEME_KEY = "awaken:appearance:theme";
const LEGACY_STORED_FONT_SCALE_KEY = "awaken:appearance:fontScale";
const DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY = "awaken:memory:dreamReceiptThreadAssignments";
const SYSTEM_THEME_QUERY = "(prefers-color-scheme: dark)";

function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function readStoredNumber(key: string, fallback: number, min: number, max: number): number {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = Number(window.localStorage.getItem(key));
    if (!Number.isFinite(value)) {
      return fallback;
    }
    return clampNumber(value, min, max);
  } catch {
    return fallback;
  }
}

function readStoredBoolean(key: string, fallback: boolean): boolean {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    if (value === null) {
      return fallback;
    }
    return value === "true";
  } catch {
    return fallback;
  }
}

function readStoredString(key: string, fallback = ""): string {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    return typeof value === "string" ? value : fallback;
  } catch {
    return fallback;
  }
}

function normalizeThemePreference(value: string): AwakenThemePreference {
  return value === "system" || value === "dark" ? value : "light";
}

function readStoredThemePreference(): AwakenThemePreference {
  return normalizeThemePreference(readStoredString(STORED_THEME_KEY));
}

function readSystemTheme(): AwakenTheme {
  if (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(SYSTEM_THEME_QUERY).matches
  ) {
    return "dark";
  }
  return "light";
}

function effectiveThemeForPreference(
  preference: AwakenThemePreference,
  systemTheme: AwakenTheme,
): AwakenTheme {
  return preference === "system" ? systemTheme : preference;
}

function writeStoredValue(key: string, value: string): void {
  if (typeof window === "undefined" || !window.localStorage) {
    return;
  }
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Layout persistence is best-effort.
  }
}

function dreamTaskReceiptEvent(task: MemoryDreamTask): AguiEvent | undefined {
  if (!["completed", "failed", "cancelled", "interrupted"].includes(task.status)) {
    return undefined;
  }
  const suffix = task.status.toUpperCase();
  return {
    type: `AWAKEN_MEMORY_DREAM_${suffix}` as AguiEvent["type"],
    recordId: task.taskId,
    timestamp: Date.parse(task.finishedAt ?? task.createdAt),
    errorCode: task.errorCode,
    message: task.errorMessage,
    diagnostic: { reports: task.reports ?? {}, projectId: task.projectId },
  };
}

export function App() {
  const runtimeClientRef = useRef(createRuntimeClient());
  const unsubscribeRef = useRef<null | (() => void)>(null);
  const terminalSeenRef = useRef(false);
  const rendererReadyNotifiedRef = useRef(false);
  const threadLoadSeqRef = useRef(0);
  const inspectorCollapsedValueRef = useRef(false);
  const inspectorCollapsedBeforeResponsiveRef = useRef<boolean | null>(null);
  const projectOperationPollFailuresRef = useRef<Set<string>>(new Set());
  const threadTitlePollsRef = useRef<Map<string, () => void>>(new Map());
  const workspaceTargetPathRef = useRef("");
  const runtimeClient = runtimeClientRef.current;
  const slashProviders = useMemo(
    () => [skillSlashProvider((workspaceRoot) => runtimeClient.listSkills(workspaceRoot))],
    [runtimeClient],
  );
  const mentionProviders = useMemo(() => [computerUseMentionProvider()], []);

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const activeThreadIdRef = useRef(runtimeState.activeThreadId);
  activeThreadIdRef.current = runtimeState.activeThreadId;
  const dreamReceiptThreadAssignmentsRef = useRef<DreamReceiptThreadAssignments>(
    parseDreamReceiptThreadAssignments(
      readStoredString(DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY),
    ),
  );
  const backgroundThreadActivationRef = useRef<(threadId: string) => void>(() => {});
  backgroundThreadActivationRef.current = async (threadId: string) => {
    // Refresh the thread authority before selecting: scheduled tasks can create
    // a dedicated thread in the background while other pages are mounted.
    let runtimeThreads: RuntimeThread[] | undefined;
    try {
      runtimeThreads = await loadThreadsIntoState();
    } catch {
      setNotice("无法加载线程列表");
      return;
    }
    const threads = runtimeThreads.map(threadItemFromRuntime);
    // A scheduled-task thread can already be the reducer's active thread while
    // its cached view is still the empty shell created by a background thread
    // list refresh. Force a transcript replay when activation comes from a
    // notification or execution history.
    await handleSelectConversation(threadId, threads, true);
  };

  useEffect(() => {
    const snapshot = projectPetActivity(runtimeState);
    runtimeClient.updatePetActivity(snapshot);
  }, [runtimeState, runtimeClient]);

  useEffect(
    () =>
      runtimeClient.onPetThreadActivationRequested((threadId) => {
        backgroundThreadActivationRef.current(threadId);
      }),
    [runtimeClient],
  );

  useEffect(
    () =>
      runtimeClient.onScheduledTaskThreadActivation((threadId) => {
        backgroundThreadActivationRef.current(threadId);
      }),
    [runtimeClient],
  );

  // Desktop 更新：先取当前快照再订阅变更；Main 是唯一 authority，
  // 推送先到时忽略迟到的 getState 返回，不写入 localStorage。
  const [desktopUpdateSnapshot, setDesktopUpdateSnapshot] =
    useState<DesktopUpdateSnapshot | null>(null);
  const [updateRestartDialogOpen, setUpdateRestartDialogOpen] = useState(false);
  const [updateInstallBlockedCode, setUpdateInstallBlockedCode] = useState<
    DesktopUpdateErrorCode | undefined
  >(undefined);
  const [updateInstallStarting, setUpdateInstallStarting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let pushReceived = false;
    const unsubscribe = runtimeClient.onDesktopUpdateStateChanged((snapshot) => {
      pushReceived = true;
      if (!cancelled) {
        setDesktopUpdateSnapshot(snapshot);
      }
    });
    runtimeClient
      .getDesktopUpdateState()
      .then((snapshot) => {
        if (!cancelled && !pushReceived) {
          setDesktopUpdateSnapshot(snapshot);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [runtimeClient]);

  const sidebarUpdateAction = useMemo(
    () =>
      buildSidebarUpdateAction(desktopUpdateSnapshot, {
        onDownload: () => {
          void runtimeClient
            .downloadDesktopUpdate()
            .then(setDesktopUpdateSnapshot)
            .catch(() => undefined);
        },
        onRestartRequested: () => {
          setUpdateInstallBlockedCode(undefined);
          setUpdateRestartDialogOpen(true);
        },
        onRetry: (retryFrom) => {
          const request =
            retryFrom === "download"
              ? runtimeClient.downloadDesktopUpdate()
              : runtimeClient.checkDesktopUpdate();
          void request.then(setDesktopUpdateSnapshot).catch(() => undefined);
        },
      }),
    [desktopUpdateSnapshot, runtimeClient],
  );

  const updateInstallBlockedNotice = desktopUpdateInstallBlockedMessage(updateInstallBlockedCode);
  const updateInstallRetryBlocked =
    updateInstallBlockedCode === "active_runs" || updateInstallBlockedCode === "shutdown_failed";

  const handleConfirmUpdateRestart = async () => {
    setUpdateInstallStarting(true);
    try {
      const snapshot = await runtimeClient.restartAndInstallDesktopUpdate();
      setDesktopUpdateSnapshot(snapshot);
      if (snapshot.phase === "downloaded") {
        // active_runs / shutdown_failed：留在确认框说明原因，不强制退出、不丢弃任务。
        setUpdateInstallBlockedCode(snapshot.errorCode);
        setUpdateInstallStarting(false);
      }
      // phase === "installing" 表示清理完成、安装器已接管，App 即将退出。
    } catch {
      setUpdateInstallBlockedCode("shutdown_failed");
      setUpdateInstallStarting(false);
    }
  };

  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [resolvingExternalInteractionId, setResolvingExternalInteractionId] = useState<
    string | null
  >(null);
  const [openingArtifactId, setOpeningArtifactId] = useState<string | null>(null);
  const [mediaPreview, setMediaPreview] = useState<MediaArtifactPreviewState | null>(null);
  const [pinnedMedia, setPinnedMedia] = useState<MediaArtifactPreviewItem | null>(null);
  const [artifactMediaSources, setArtifactMediaSources] = useState<Record<string, string | null>>({});
  const [artifactMediaMetadata, setArtifactMediaMetadata] = useState<
    Record<string, AwakenMediaMetadata>
  >({});
  const [artifactMediaHydrationEpoch, setArtifactMediaHydrationEpoch] = useState(0);
  const artifactMediaSourcesRef = useRef<Record<string, string | null>>({});
  const artifactMediaSourcePendingRef = useRef<Set<string>>(new Set());
  const artifactMediaSourceGenerationRef = useRef(0);
  const artifactMediaSourceRetryRef = useRef<Map<string, number>>(new Map());
  const workbenchMediaSurfaceActiveRef = useRef(true);
  const [answeringUserQuestionId, setAnsweringUserQuestionId] = useState<string | null>(null);
  const [resolvingComputerReviewId, setResolvingComputerReviewId] = useState<string | null>(null);
  const [controllingComputerHandoffId, setControllingComputerHandoffId] = useState<string | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [modelProfiles, setModelProfiles] = useState<RuntimeModelProfileSummary[]>([]);
  const [modelProviders, setModelProviders] = useState<ModelProvider[]>([]);
  const [newThreadModelPreference, setNewThreadModelPreference] = useState<{
    selectedModelProfileId?: string;
    reasoningEffortByProfile: Record<string, ReasoningEffortLevel>;
  }>({ reasoningEffortByProfile: {} });
  const recoveredModelPreferenceRef = useRef<string>("");
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [modelSettingsInitialModelSection, setModelSettingsInitialModelSection] =
    useState<ModelAccessType>("coding_plan");
  const [appearanceThemePreference, setAppearanceThemePreference] =
    useState<AwakenThemePreference>(readStoredThemePreference);
  const [systemAppearanceTheme, setSystemAppearanceTheme] = useState<AwakenTheme>(readSystemTheme);
  const appearanceTheme = effectiveThemeForPreference(
    appearanceThemePreference,
    systemAppearanceTheme,
  );
  const [appearanceSaving, setAppearanceSaving] = useState(false);
  const [appearanceMessage, setAppearanceMessage] = useState<string | null>(null);
  const [profilePopoverOpen, setProfilePopoverOpen] = useState(false);
  const profileButtonRef = useRef<HTMLButtonElement>(null);
  const [contextUsagePanelOpen, setContextUsagePanelOpen] = useState(false);
  const contextUsageTriggerRef = useRef<HTMLButtonElement>(null);
  const contextUsagePanelRef = useRef<HTMLDivElement>(null);
  const [workspacePath, setWorkspacePath] = useState("");
  const [createProjectDialogOpen, setCreateProjectDialogOpen] = useState(false);
  const [renameDialogOpen, setRenameDialogOpen] = useState(false);
  const [renamingThreadId, setRenamingThreadId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  const [renameSubmitting, setRenameSubmitting] = useState(false);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const [pendingProjectMode, setPendingProjectMode] = useState<"local">("local");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  const [draftProjectFolders, setDraftProjectFolders] = useState<string[]>([]);
  const [draftPrimaryFolder, setDraftPrimaryFolder] = useState<string>("");
  const [draftProjectName, setDraftProjectName] = useState<string>("");
  const [createProjectSubmitting, setCreateProjectSubmitting] = useState(false);
  const [editingProjectId, setEditingProjectId] = useState<string | null>(null);
  const [projectFolderBusyPath, setProjectFolderBusyPath] = useState<string | undefined>(undefined);
  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<WorkspaceProject[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState<string | undefined>(undefined);
  const [conversationScope, setConversationScope] = useState(true);
  useEffect(
    () =>
      runtimeClient.onMemoryDreamReceipt((task) => {
        if (task.projectId && task.projectId !== selectedProjectId) return;
        const threadId = activeThreadIdRef.current;
        if (!threadId) return;
        const assignments = assignDreamReceiptToThread(
          dreamReceiptThreadAssignmentsRef.current,
          task.taskId,
          threadId,
        );
        dreamReceiptThreadAssignmentsRef.current = assignments;
        writeStoredValue(DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY, JSON.stringify(assignments));
        const event = dreamTaskReceiptEvent(task);
        if (event) dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
      }),
    [runtimeClient, selectedProjectId],
  );
  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>([]);
  const [conversationSectionExpanded, setConversationSectionExpanded] = useState(true);
  const [conversationBatchMode, setConversationBatchMode] = useState(false);
  const [selectedConversationThreadIds, setSelectedConversationThreadIds] = useState<string[]>(
    [],
  );
  const [pendingThreadDelete, setPendingThreadDelete] = useState<PendingThreadDelete | null>(null);
  const [threadDeleteSubmitting, setThreadDeleteSubmitting] = useState(false);
  const [projectOrderIds, setProjectOrderIds] = useState<string[]>([]);
  const [projectReorderSubmitting, setProjectReorderSubmitting] = useState(false);
  const projectReorderSubmittingRef = useRef(false);
  const [removedProjectIds, setRemovedProjectIds] = useState<string[]>([]);
  const [defaultBackendProjectId, setDefaultBackendProjectId] = useState<string | undefined>(
    undefined,
  );
  const [defaultProjectFolders, setDefaultProjectFolders] = useState<WorkspaceProjectFolder[]>([]);
  const [pendingProjectDelete, setPendingProjectDelete] = useState<PendingProjectDelete | null>(
    null,
  );
  const [projectDeleteSubmitting, setProjectDeleteSubmitting] = useState(false);
  const [trackedTrashOperations, setTrackedTrashOperations] = useState<TrackedTrashOperation[]>(
    () => parseTrackedTrashOperations(readStoredString(TRACKED_TRASH_OPERATIONS_STORAGE_KEY)),
  );
  const defaultProjectRemoved = removedProjectIds.includes(DEFAULT_WORKSPACE_PROJECT_ID);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(() =>
    readStoredNumber(
      "awaken:layout:sidebarWidth",
      DEFAULT_SIDEBAR_WIDTH,
      MIN_SIDEBAR_WIDTH,
      MAX_SIDEBAR_WIDTH,
    ),
  );
  const [inspectorWidth, setInspectorWidth] = useState(() =>
    sanitizeInspectorPreference(
      readStoredNumber(
        "awaken:layout:inspectorWidth",
        DEFAULT_INSPECTOR_WIDTH,
        MIN_INSPECTOR_WIDTH,
        MAX_INSPECTOR_WIDTH,
      ),
    ),
  );
  const [fileWorkspaceInspectorWidth, setFileWorkspaceInspectorWidth] = useState<
    number | undefined
  >();
  const [windowWidth, setWindowWidth] = useState(readWindowWidth);
  const [inspectorCollapsed, setInspectorCollapsed] = useState(() =>
    readStoredBoolean("awaken:layout:inspectorCollapsed", false),
  );
  const [fileWorkspaceOpen, setFileWorkspaceOpen] = useState(false);
  const [fileWorkspaceExpanded, setFileWorkspaceExpanded] = useState(false);
  const [expandedWorkspaceSurface, setExpandedWorkspaceSurface] =
    useState<ExpandedWorkspaceSurface>("file");
  const [expandedFileComposerIdleMode, setExpandedFileComposerIdleMode] =
    useState<ExpandedFileComposerIdleMode>("full");
  const [expandedFileComposerTrackedRun, setExpandedFileComposerTrackedRun] =
    useState<ExpandedFileComposerTrackedRun>();
  const [fileWorkspaceMode, setFileWorkspaceMode] = useState<ProjectFileWorkspaceMode>("files");
  const [fileWorkspaceTabsState, dispatchFileWorkspaceTabs] = useReducer(
    projectFileTabsReducer,
    undefined,
    createProjectFileTabsState,
  );
  const [deliverablePreview, setDeliverablePreview] =
    useState<ArtifactPreviewSelection>();
  const [selectedSubagentTaskId, setSelectedSubagentTaskId] = useState<string | undefined>();
  const [openSubagentTaskId, setOpenSubagentTaskId] = useState<string | undefined>();
  const [cancellingSubagentTaskId, setCancellingSubagentTaskId] = useState<string | undefined>();
  const [subagentNowMs, setSubagentNowMs] = useState(() => Date.now());
  const [promptBubbleModeByTurnId, setPromptBubbleModeByTurnId] = useState<
    Record<string, "collapsed" | "expanded">
  >({});
  const [executionProcessOpenByTurnId, setExecutionProcessOpenByTurnId] = useState<
    Record<string, boolean>
  >({});
  const [activityBatchOpenById, setActivityBatchOpenById] = useState<
    Record<string, boolean | undefined>
  >({});
  const [toolTraceOpenById, setToolTraceOpenById] = useState<
    Record<string, boolean | undefined>
  >({});
  const composerDraftKey = composerDraftOwnerKey(runtimeState.activeThreadId);
  const {
    draft: composerDraft,
    setValue: setComposerValue,
    setAttachments: setComposerAttachments,
    setTokens: setComposerTokens,
    setTokenPlacements: setComposerTokenPlacements,
    setTokenLeadingTexts: setComposerTokenLeadingTexts,
    setSelection: setComposerInputSelection,
    replaceDraft: replaceComposerDraft,
    clearDraft: clearThreadComposerDraft,
  } = useThreadComposerDraft(composerDraftKey);
  const composerValue = composerDraft.value;
  const composerAttachments = composerDraft.attachments;
  const composerTokens = composerDraft.tokens;
  const composerTokenPlacements = composerDraft.tokenPlacements;
  const composerTokenLeadingTexts = composerDraft.tokenLeadingTexts;
  const composerInputSelection = composerDraft.selection;
  const [slashActions, setSlashActions] = useState<SlashAction[]>([]);
  const [slashLoading, setSlashLoading] = useState(false);
  const [slashError, setSlashError] = useState<string | null>(null);
  const [slashActiveActionId, setSlashActiveActionId] = useState<string | undefined>();
  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  // Latest plan snapshot of the active run (contract 运行计划与任务进度设计
  // 契约 §8.2/§8.3): live frames only feed this slice; the trigger/panel are
  // its sole live projection and the terminal history lives in the narrative.
  const activeRunPlan = runtimeState.runId
    ? runtimeState.runPlansById[runtimeState.runId]
    : undefined;
  const runPlanEntry = useRunPlanEntry({
    threadId: runtimeState.activeThreadId,
    runId: runtimeState.runId,
    plan: activeRunPlan,
    isBusy,
  });
  const activeRunPlanItems = useMemo(
    () =>
      (activeRunPlan?.items ?? []).map((item) => ({
        step: item.step,
        activeForm: item.activeForm,
        status: runPlanStepStatusForDisplay(item.status),
      })),
    [activeRunPlan],
  );
  const runPlanPanelId = "awaken-run-plan-panel";
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem ? [runtimeState.narrative.activeItem] : []),
    ];
    return projectVisibleNarrativeItems({ items: narrativeItems, isBusy });
  }, [isBusy, runtimeState.narrative]);
  const replayedNarrativeItemIds = useMemo(
    () => new Set(runtimeState.narrative.replayedItemIds),
    [runtimeState.narrative.replayedItemIds],
  );
  const toolResultsById = useMemo(
    () => new Map(runtimeState.toolResults.map((record) => [record.id, record])),
    [runtimeState.toolResults],
  );
  const patchPreviewsById = useMemo(
    () => new Map(runtimeState.patchPreviews.map((preview) => [preview.id, preview])),
    [runtimeState.patchPreviews],
  );
  const artifactMediaRequests = useMemo(() => {
    const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
    if (!threadId) return [];
    const byPath = new Map(
      runtimeState.artifacts
        .filter((artifact) => artifact.path)
        .map((artifact) => [artifact.path!.replace(/\\/g, "/").toLowerCase(), artifact]),
    );
    const requests = new Map<string, { threadId: string; runId: string; artifactId: string }>();
    for (const item of visibleNarrativeItems) {
      if (
        item.kind === "mediaGeneration" &&
        (item.status === "ready" || item.status === "partial") &&
        item.runId
      ) {
        const readyPaths = item.mediaItems?.length
          ? item.mediaItems.flatMap((mediaItem) =>
              mediaItem.status === "ready" && mediaItem.path ? [mediaItem.path] : [],
            )
          : item.artifactPaths;
        for (const artifactPath of readyPaths) {
          const artifact = byPath.get(artifactPath.replace(/\\/g, "/").toLowerCase());
          const request = mediaArtifactRequestForPath(threadId, item.runId, artifactPath, artifact);
          requests.set(artifactMediaSourceKey(request), request);
        }
      }
      if (item.kind === "deliverables") {
        for (const file of item.files) {
          if (file.fileType !== "image" && file.fileType !== "video") continue;
          const key = `${threadId}:${file.runId}:${file.artifactId}`;
          requests.set(key, { threadId, runId: file.runId, artifactId: file.artifactId });
        }
      }
    }
    return [...requests.entries()].map(([key, request]) => ({ key, request }));
  }, [runtimeState.activeThreadId, runtimeState.artifacts, runtimeState.threadId, visibleNarrativeItems]);

  useEffect(() => {
    artifactMediaSourceGenerationRef.current += 1;
    artifactMediaSourcePendingRef.current.clear();
    artifactMediaSourceRetryRef.current.clear();
    artifactMediaSourcesRef.current = {};
    setArtifactMediaSources({});
    setArtifactMediaMetadata({});
    setMediaPreview(null);
    setPinnedMedia(null);
    setDeliverablePreview(undefined);
  }, [runtimeState.activeThreadId]);

  useEffect(() => {
    const sourceGeneration = artifactMediaSourceGenerationRef.current;
    for (const { key, request } of artifactMediaRequests) {
      if (Object.prototype.hasOwnProperty.call(artifactMediaSourcesRef.current, key)) continue;
      if (artifactMediaSourcePendingRef.current.has(key)) continue;
      artifactMediaSourcePendingRef.current.add(key);
      void runtimeClient
        .getArtifactMediaSource(request)
        .then((result) => {
          artifactMediaSourcePendingRef.current.delete(key);
          if (sourceGeneration !== artifactMediaSourceGenerationRef.current) return;
          const nextSource = result.ok ? result.src : null;
          if (result.ok) {
            artifactMediaSourceRetryRef.current.delete(key);
          }
          artifactMediaSourcesRef.current = {
            ...artifactMediaSourcesRef.current,
            [key]: nextSource,
          };
          setArtifactMediaSources((current) => ({
            ...current,
            [key]: nextSource,
          }));
        })
        .catch(() => {
          artifactMediaSourcePendingRef.current.delete(key);
          if (sourceGeneration !== artifactMediaSourceGenerationRef.current) return;
          artifactMediaSourcesRef.current = {
            ...artifactMediaSourcesRef.current,
            [key]: null,
          };
          setArtifactMediaSources((current) => ({ ...current, [key]: null }));
        });
    }
  }, [artifactMediaHydrationEpoch, artifactMediaRequests, runtimeClient]);
  const subagentTasks = useMemo<SubagentTaskView[]>(
    () =>
      runtimeState.subagentOrder.flatMap((taskId) => {
        const task = runtimeState.subagentsById[taskId];
        return task ? [task] : [];
      }),
    [runtimeState.subagentOrder, runtimeState.subagentsById],
  );
  const selectedSubagent =
    subagentTasks.find((task) => task.taskId === selectedSubagentTaskId) ??
    (selectedSubagentTaskId ? undefined : subagentTasks[subagentTasks.length - 1]);
  const openSubagent = openSubagentTaskId
    ? runtimeState.subagentsById[openSubagentTaskId]
    : undefined;
  const openSubagentNarrativeItems = openSubagent
    ? projectVisibleNarrativeItems({
        items: [
          ...openSubagent.narrative.committedItems,
          ...(openSubagent.narrative.activeItem ? [openSubagent.narrative.activeItem] : []),
        ],
        isBusy: ["queued", "running", "cancelling"].includes(openSubagent.status),
      })
    : [];
  const openSubagentHasAnswer = openSubagentNarrativeItems.some(
    (candidate) => candidate.kind === "answer",
  );
  const openSubagentMetrics = openSubagent
    ? [
        openSubagent.toolUseCount > 0 ? `${openSubagent.toolUseCount} 次工具` : undefined,
        (openSubagent.totalTokens ?? openSubagent.estimatedTokens)
          ? `${new Intl.NumberFormat("zh-CN").format(
              openSubagent.totalTokens ?? openSubagent.estimatedTokens ?? 0,
            )} tokens`
          : undefined,
        formatSubagentElapsed(subagentElapsedMs(openSubagent, subagentNowMs)),
      ].filter((value): value is string => Boolean(value))
    : [];
  const runningSubagentCount = subagentTasks.filter((task) =>
    ["queued", "running", "cancelling"].includes(task.status),
  ).length;

  useEffect(() => {
    setSubagentNowMs(Date.now());
    if (runningSubagentCount === 0) return undefined;
    const interval = window.setInterval(() => setSubagentNowMs(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [runningSubagentCount]);
  const desktopReady = runtimeState.supervisorStatus === "running" && runtimeInfo !== null;
  const desktopStarting =
    runtimeState.supervisorStatus === null ||
    runtimeState.supervisorStatus === "starting" ||
    runtimeState.supervisorStatus === "stopping" ||
    runtimeState.supervisorStatus === "stopped";
  const desktopNeedsRestart = runtimeState.supervisorStatus === "crashed";

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }
    document.documentElement.dataset.awTheme = appearanceTheme;
    document.documentElement.dataset.awThemePreference = appearanceThemePreference;
    delete document.documentElement.dataset.awFontScale;
    document.documentElement.style.colorScheme = appearanceTheme === "dark" ? "dark" : "light";
    writeStoredValue(STORED_THEME_KEY, appearanceThemePreference);
    try {
      window.localStorage.removeItem(LEGACY_STORED_FONT_SCALE_KEY);
    } catch {
      // Leftover scale mirrors are unused after the preference was removed.
    }
  }, [appearanceTheme, appearanceThemePreference]);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }

    const media = window.matchMedia(SYSTEM_THEME_QUERY);
    const updateSystemTheme = (matches: boolean) => {
      setSystemAppearanceTheme(matches ? "dark" : "light");
    };
    const handleChange = (event: MediaQueryListEvent) => {
      updateSystemTheme(event.matches);
    };

    updateSystemTheme(media.matches);
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    if (!renameDialogOpen) {
      return;
    }
    const input = renameInputRef.current;
    if (!input) {
      return;
    }
    const timeoutId = setTimeout(() => {
      input.focus();
      input.select();
    }, 0);
    return () => clearTimeout(timeoutId);
  }, [renameDialogOpen]);

  useEffect(() => {
    let cancelled = false;

    async function loadAppearancePreferences(): Promise<void> {
      try {
        const preferences = await runtimeClient.getDesktopPreferences();
        if (!cancelled) {
          setAppearanceThemePreference(preferences.theme);
        }
      } catch {
        // The localStorage mirror already provides the best available fallback.
      }
    }

    void loadAppearancePreferences();
    return () => {
      cancelled = true;
    };
  }, [runtimeClient]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const handleResize = () => {
      setWindowWidth(readWindowWidth());
    };
    window.addEventListener("resize", handleResize);
    return () => {
      window.removeEventListener("resize", handleResize);
    };
  }, []);

  useEffect(() => {
    writeStoredValue("awaken:layout:sidebarWidth", String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorWidth", String(inspectorWidth));
  }, [inspectorWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorCollapsed", String(inspectorCollapsed));
    inspectorCollapsedValueRef.current = inspectorCollapsed;
  }, [inspectorCollapsed]);

  useEffect(() => {
    writeStoredValue(TRACKED_TRASH_OPERATIONS_STORAGE_KEY, JSON.stringify(trackedTrashOperations));
  }, [trackedTrashOperations]);

  useEffect(() => {
    workspaceTargetPathRef.current = workspacePath;
  }, [workspacePath]);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }

    const media = window.matchMedia(INSPECTOR_RESPONSIVE_COLLAPSE_QUERY);
    const handleResponsiveInspector = (matches: boolean) => {
      if (matches) {
        if (inspectorCollapsedBeforeResponsiveRef.current === null) {
          inspectorCollapsedBeforeResponsiveRef.current = inspectorCollapsedValueRef.current;
          setInspectorCollapsed(true);
        }
        return;
      }

      const previousCollapsed = inspectorCollapsedBeforeResponsiveRef.current;
      inspectorCollapsedBeforeResponsiveRef.current = null;
      if (previousCollapsed !== null) {
        setInspectorCollapsed(previousCollapsed);
      }
    };
    const handleChange = (event: MediaQueryListEvent) => {
      handleResponsiveInspector(event.matches);
    };

    handleResponsiveInspector(media.matches);
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    window.localStorage?.removeItem("awaken:threads:projects");
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      if (runtimeState.supervisorStatus !== "running") {
        setRuntimeInfo(null);
        setModelProfiles([]);
        setModelProviders([]);
        return;
      }
      try {
        const [info, profiles, providers] = await Promise.all([
          runtimeClient.getRuntimeInfo(),
          runtimeClient.listModelProfiles().catch(() => []),
          runtimeClient.listModelProviders().catch(() => []),
        ]);
        if (!cancelled) {
          setRuntimeInfo(info);
          setModelProfiles(profiles);
          setModelProviders(providers);
        }
      } catch {
        if (!cancelled) {
          setRuntimeInfo(null);
          setModelProfiles([]);
          setModelProviders([]);
        }
      }
    }

    void poll();
    const interval = setInterval(() => {
      void poll();
    }, 3000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [runtimeClient, runtimeState.supervisorStatus]);

  useEffect(() => {
    let cancelled = false;

    async function initSupervisor(): Promise<void> {
      try {
        const info = await runtimeClient.getSupervisorStatus();
        if (!cancelled) {
          dispatch({ type: "supervisor/status-changed", info });
        }
      } catch {
        // best-effort: supervisor may not be available yet
      }
    }

    void initSupervisor();

    const unsubscribeStatus = runtimeClient.onSupervisorStatusChange((info) => {
      if (!cancelled) {
        dispatch({ type: "supervisor/status-changed", info });
      }
    });

    return () => {
      cancelled = true;
      unsubscribeStatus();
    };
  }, [runtimeClient]);

  useEffect(() => {
    return () => {
      unsubscribeRef.current?.();
      unsubscribeRef.current = null;
    };
  }, []);

  useEffect(() => {
    return () => {
      stopThreadTitlePoll();
    };
  }, []);

  useEffect(() => {
    if (rendererReadyNotifiedRef.current || (!desktopReady && !desktopNeedsRestart)) {
      return;
    }

    rendererReadyNotifiedRef.current = true;
    let cancelled = false;

    async function notifyWhenStable(): Promise<void> {
      try {
        await document.fonts?.ready;
      } catch {
        // Font readiness is a polish signal only; rendering should continue.
      }
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      if (!cancelled) {
        runtimeClient.notifyRendererReady();
      }
    }

    void notifyWhenStable();

    return () => {
      cancelled = true;
    };
  }, [desktopNeedsRestart, desktopReady, runtimeClient]);

  useEffect(() => {
    if (
      runtimeInfo &&
      runtimeInfo.defaultWorkspace &&
      workspacePath === "" &&
      !defaultProjectRemoved
    ) {
      setWorkspacePath(runtimeInfo.defaultWorkspace);
    }
  }, [defaultProjectRemoved, runtimeInfo, workspacePath]);

  useEffect(() => {
    if (!notice) {
      return;
    }

    const timer = window.setTimeout(() => {
      setNotice(null);
    }, 2000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    return window.awaken?.onSelectAllRequested(() => {
      selectAllInFocusedEditor(document);
    });
  }, []);

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent): void {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "b") {
        event.preventDefault();
        setSidebarCollapsed((collapsed) => !collapsed);
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  function applyRuntimeProjects(projects: RuntimeProject[]): void {
    const configuredDefaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;
    const defaultProject = projects.find((project) =>
      isSameWorkspacePath(project.workspaceRoot, configuredDefaultWorkspacePath),
    );
    setDefaultBackendProjectId(defaultProject?.projectId);
    const nextDefaultFolders =
      defaultProject?.folders.map<WorkspaceProjectFolder>((folder) => ({
        path: folder.path,
        role: folder.role,
        isPrimary: folder.isPrimary,
      })) ?? [];
    setDefaultProjectFolders((current) =>
      sameWorkspaceProjectFolders(current, nextDefaultFolders) ? current : nextDefaultFolders,
    );
    setRemovedProjectIds(
      defaultProject && defaultProject.state !== "active" ? [DEFAULT_WORKSPACE_PROJECT_ID] : [],
    );

    const nextProjectOrderIds = projects
      .filter((project) => project.state === "active")
      .map((project) =>
        isSameWorkspacePath(project.workspaceRoot, configuredDefaultWorkspacePath)
          ? DEFAULT_WORKSPACE_PROJECT_ID
          : project.projectId,
      );
    setProjectOrderIds((current) =>
      current.length === nextProjectOrderIds.length &&
      current.every((id, index) => id === nextProjectOrderIds[index])
        ? current
        : nextProjectOrderIds,
    );

    const nextProjects = projects
      .filter(
        (project) =>
          project.state === "active" &&
          !isSameWorkspacePath(project.workspaceRoot, configuredDefaultWorkspacePath),
      )
      .map<WorkspaceProject>((project) => ({
        id: project.projectId,
        backendProjectId: project.projectId,
        label: project.displayName || workspaceLabelForPath(project.workspaceRoot),
        path: project.workspaceRoot,
        folders: project.folders.map<WorkspaceProjectFolder>((folder) => ({
          path: folder.path,
          role: folder.role,
          isPrimary: folder.isPrimary,
        })),
      }));
    setProjectsFromWorkspace((current) =>
      sameWorkspaceProjects(current, nextProjects) ? current : nextProjects,
    );
  }

  async function refreshProjects(): Promise<RuntimeProject[]> {
    const projects = await runtimeClient.listProjects(true);
    applyRuntimeProjects(projects);
    return projects;
  }

  async function refreshThreads(): Promise<void> {
    if (!desktopReady) {
      return;
    }
    try {
      const threads = await runtimeClient.listThreads();
      dispatch({ type: "run/threadsLoaded", threads });
    } catch {
      // Thread listing is best-effort; runtime reachability is already surfaced
      // by the runtime info poll.
    }
  }

  async function loadThreadsIntoState(): Promise<RuntimeThread[]> {
    const threads = await runtimeClient.listThreads();
    dispatch({ type: "run/threadsLoaded", threads });
    return threads;
  }

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    let cancelled = false;
    const reconcileScheduledThreads = async (startedThreadId?: string) => {
      try {
        const threads = await runtimeClient.listThreads();
        if (!cancelled) {
          dispatch({ type: "run/threadsLoaded", threads });
          if (startedThreadId) {
            const startedThread = threads.find((thread) => thread.threadId === startedThreadId);
            if (startedThread) {
              const projectId = startedThread.projectId;
              if (projectId) {
                setExpandedProjectIds((current) =>
                  current.includes(projectId) ? current : [...current, projectId],
                );
              } else {
                setConversationSectionExpanded(true);
              }
            }
          }
        }
      } catch {
        // The runtime reachability surface owns the visible error. Reconnect
        // or the next scheduled event retries this durable reconciliation.
      }
    };
    void reconcileScheduledThreads();
    const unsubscribe = runtimeClient.subscribeScheduledTaskEvents((event) => {
      if (
        (event.type === "execution_started" || event.type === "execution_finished") &&
        event.threadId
      ) {
        void reconcileScheduledThreads(event.threadId);
      }
    });
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [desktopReady, runtimeClient]);

  useEffect(() => {
    if (!desktopReady || trackedTrashOperations.length === 0) {
      return;
    }

    let cancelled = false;
    let timer: number | undefined;

    async function pollTrackedTrashOperations(): Promise<void> {
      const checked = await Promise.all(
        trackedTrashOperations.map(async (tracked) => {
          try {
            const operation = await runtimeClient.getProjectOperation(tracked.operationId);
            projectOperationPollFailuresRef.current.delete(tracked.operationId);
            return {
              tracked,
              operation,
              state: projectOperationReconciliationState(operation),
            };
          } catch (error) {
            if (!projectOperationPollFailuresRef.current.has(tracked.operationId)) {
              projectOperationPollFailuresRef.current.add(tracked.operationId);
              console.warn("project_operation_poll_failed", {
                operationId: tracked.operationId,
                error,
              });
            }
            return undefined;
          }
        }),
      );
      if (cancelled) {
        return;
      }

      const terminal = checked.filter(
        (result): result is Exclude<(typeof checked)[number], undefined> =>
          Boolean(result && result.state !== "pending"),
      );
      if (terminal.length > 0) {
        try {
          const projects = await runtimeClient.listProjects(true);
          if (cancelled) {
            return;
          }
          const reconciled = terminal.filter(({ tracked, state }) => {
            const project = projects.find(
              (candidate) => candidate.projectId === tracked.backendProjectId,
            );
            return canReconcileTrackedTrashOperation(state, project, tracked.folderPath);
          });

          if (reconciled.length > 0) {
            applyRuntimeProjects(projects);
            const reconciledIds = new Set(reconciled.map(({ tracked }) => tracked.operationId));
            const failed = reconciled.filter(({ state }) => state === "failed");
            if (failed.length > 0) {
              setExpandedProjectIds((current) => {
                const next = [...current];
                for (const { tracked } of failed) {
                  if (tracked.wasExpanded && !next.includes(tracked.projectId)) {
                    next.push(tracked.projectId);
                  }
                }
                return next;
              });
              setNotice(
                `未能将“${failed[0].tracked.label}”移到废纸篓，文件夹、项目历史和磁盘文件均保持不变。`,
              );
            } else {
              setNotice(
                `已将“${reconciled[0].tracked.label}”移到废纸篓，项目的其余文件夹不受影响。`,
              );
            }
            setTrackedTrashOperations((current) =>
              current.filter((tracked) => !reconciledIds.has(tracked.operationId)),
            );
            for (const operationId of reconciledIds) {
              projectOperationPollFailuresRef.current.delete(operationId);
            }

            void runtimeClient
              .listThreads()
              .then((threads) => {
                dispatch({ type: "run/threadsLoaded", threads });
              })
              .catch((error) => {
                console.warn("project_operation_thread_refresh_failed", error);
              });
          }
        } catch (error) {
          console.warn("project_operation_reconcile_failed", error);
        }
      }

      if (!cancelled) {
        timer = window.setTimeout(() => {
          void pollTrackedTrashOperations();
        }, 1000);
      }
    }

    void pollTrackedTrashOperations();
    return () => {
      cancelled = true;
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, [
    desktopReady,
    runtimeClient,
    runtimeInfo?.defaultWorkspace,
    trackedTrashOperations,
    workspacePath,
  ]);

  async function refreshRuntimeInfoNow(): Promise<void> {
    if (runtimeState.supervisorStatus !== "running") {
      setRuntimeInfo(null);
      setModelProfiles([]);
      setModelProviders([]);
      return;
    }
    try {
      const [info, profiles, providers] = await Promise.all([
        runtimeClient.getRuntimeInfo(),
        runtimeClient.listModelProfiles().catch(() => []),
        runtimeClient.listModelProviders().catch(() => []),
      ]);
      setRuntimeInfo(info);
      setModelProfiles(profiles);
      setModelProviders(providers);
    } catch {
      setRuntimeInfo(null);
      setModelProfiles([]);
      setModelProviders([]);
    }
  }

  function stopThreadTitlePoll(threadId?: string): void {
    if (threadId) {
      const cancel = threadTitlePollsRef.current.get(threadId);
      cancel?.();
      threadTitlePollsRef.current.delete(threadId);
      return;
    }
    for (const cancel of threadTitlePollsRef.current.values()) {
      cancel();
    }
    threadTitlePollsRef.current.clear();
  }

  function startThreadTitlePoll(threadId: string): void {
    stopThreadTitlePoll(threadId);

    let cancelled = false;
    let activeTimeoutId: ReturnType<typeof setTimeout> | null = null;

    const cancel = () => {
      cancelled = true;
      if (activeTimeoutId !== null) {
        clearTimeout(activeTimeoutId);
        activeTimeoutId = null;
      }
    };

    threadTitlePollsRef.current.set(threadId, cancel);

    const delays = [500, 1000, 2000, 4000, 8000, 12000, 20000, 30000, 60000, 90000];

    async function pollOnce(): Promise<void> {
      for (const delay of delays) {
        if (cancelled) {
          return;
        }
        await new Promise<void>((resolve) => {
          activeTimeoutId = setTimeout(() => {
            activeTimeoutId = null;
            resolve();
          }, delay);
        });
        if (cancelled) {
          return;
        }
        try {
          const threads = await runtimeClient.listThreads();
          if (cancelled) {
            return;
          }
          const thread = threads.find((t) => t.threadId === threadId);
          if (!thread) {
            continue;
          }
          if (
            thread.titleSource === "manual" ||
            (thread.titleStatus === "ready" && thread.titleSource === "llm")
          ) {
            dispatch({ type: "run/threadsLoaded", threads });
            stopThreadTitlePoll(threadId);
            return;
          }
          if (thread.titleStatus === "failed") {
            stopThreadTitlePoll(threadId);
            return;
          }
        } catch {
          // Best-effort polling; transient failures are ignored.
        }
      }
    }

    void pollOnce().finally(() => {
      if (threadTitlePollsRef.current.get(threadId) === cancel) {
        threadTitlePollsRef.current.delete(threadId);
      }
    });
  }

  async function handleApplyModelPreference(
    profileId: string,
    reasoningEffort?: ReasoningEffortLevel,
  ): Promise<void> {
    if (isBusy) return;
    try {
      setNotice(null);
      if (activeThread?.threadId) {
        await runtimeClient.updateThreadModelPreference(activeThread.threadId, {
          modelProfileId: profileId,
          reasoningEffort,
        });
        await refreshThreads();
      } else {
        setNewThreadModelPreference((current) => ({
          selectedModelProfileId: profileId,
          reasoningEffortByProfile: {
            ...current.reasoningEffortByProfile,
            ...(reasoningEffort ? { [profileId]: reasoningEffort } : {}),
          },
        }));
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to switch model");
    }
  }

  async function handleAppearanceThemeChange(theme: AwakenThemePreference): Promise<void> {
    if (theme === appearanceThemePreference || appearanceSaving) {
      return;
    }

    const previousTheme = appearanceThemePreference;
    setAppearanceThemePreference(theme);
    setAppearanceSaving(true);
    setAppearanceMessage(null);
    try {
      const preferences = await runtimeClient.saveDesktopPreferences({ theme });
      setAppearanceThemePreference(preferences.theme);
    } catch (error) {
      setAppearanceThemePreference(previousTheme);
      setAppearanceMessage(error instanceof Error ? error.message : "Failed to save theme");
    } finally {
      setAppearanceSaving(false);
    }
  }

  async function resolveActiveExternalInteraction(
    resolution: "completed" | "cancelled" | "expired" | "failed",
  ): Promise<void> {
    if (!runtimeState.runId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveExternalInteraction(
        runtimeState.runId,
        activeExternalInteraction.id,
        resolution,
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to resolve external interaction");
    } finally {
      setResolvingExternalInteractionId(null);
    }
  }

  async function resolveActiveACPPermission(optionId: string, cancelled = false): Promise<void> {
    if (!runtimeState.runId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveACPPermission(
        runtimeState.runId,
        activeExternalInteraction.id,
        optionId,
        cancelled,
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to resolve ACP permission");
    } finally {
      setResolvingExternalInteractionId(null);
    }
  }

  async function resolveActiveComputerReview(decision: "approved_once" | "denied"): Promise<void> {
    if (!runtimeState.runId || !activeComputerReview) {
      return;
    }
    if (resolvingComputerReviewId === activeComputerReview.id) {
      return;
    }
    setNotice(null);
    setResolvingComputerReviewId(activeComputerReview.id);
    try {
      await runtimeClient.resolveComputerReview(
        runtimeState.runId,
        activeComputerReview.reviewId,
        decision,
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to resolve computer review");
    } finally {
      setResolvingComputerReviewId(null);
    }
  }

  async function returnComputerControl(): Promise<void> {
    if (!runtimeState.runId || !activeComputerHandoff) {
      return;
    }
    if (controllingComputerHandoffId === activeComputerHandoff.id) {
      return;
    }
    setNotice(null);
    setControllingComputerHandoffId(activeComputerHandoff.id);
    try {
      await runtimeClient.computerSessionControl(runtimeState.runId, "return");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to return computer control");
    } finally {
      setControllingComputerHandoffId(null);
    }
  }

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    void refreshProjects().catch((error) => {
      console.error("project_list_refresh_failed", error);
    });
    void refreshThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient]);

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    const persistedRunId = localStorage.getItem("awaken:runId");
    if (!persistedRunId) return;

    let cancelled = false;

    async function recover(): Promise<void> {
      try {
        const run = await runtimeClient.getRun(persistedRunId!);
        if (cancelled) return;
        try {
          const threads = await runtimeClient.listThreads();
          if (cancelled) return;
          dispatch({ type: "run/threadsLoaded", threads });
        } catch {
          // Recovery can still rebuild the thread transcript even if thread
          // listing fails temporarily.
        }
        const loadSeq = threadLoadSeqRef.current + 1;
        threadLoadSeqRef.current = loadSeq;
        prepareForThreadSwitch();
        await replayThreadTranscript(run.threadId, loadSeq);
      } catch {
        if (!cancelled) {
          localStorage.removeItem("awaken:runId");
        }
      }
    }

    void recover();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient]);

  function resetActiveThreadContext(): void {
    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    setNewThreadModelPreference({ reasoningEffortByProfile: {} });
    localStorage.removeItem("awaken:runId");
    prepareForNewTask();
    dispatch({ type: "run/reset" });
  }

  const enabledModelProfiles = modelProfiles.filter((profile) => profile.enabled);
  const activeThread = runtimeState.activeThreadId
    ? runtimeState.threads.find((thread) => thread.threadId === runtimeState.activeThreadId)
    : undefined;
  const requestedModelProfileId =
    activeThread?.selectedModelProfileId ?? newThreadModelPreference.selectedModelProfileId;
  const selectedModelProfile =
    enabledModelProfiles.find((profile) => profile.id === requestedModelProfileId) ??
    enabledModelProfiles.find((profile) => profile.id === runtimeInfo?.modelProfile?.id) ??
    enabledModelProfiles.find((profile) => profile.isDefault) ??
    enabledModelProfiles[0];
  const selectedModelProvider = selectedModelProfile
    ? modelProviders.find((provider) => provider.id === selectedModelProfile.providerId)
    : undefined;
  const selectedReasoningCapability = selectedModelProfile
    ? selectedModelProfile.effectiveCapability?.reasoningEffort ??
      selectedModelProvider?.modelCapabilities[selectedModelProfile.modelId]?.reasoningEffort
    : undefined;
  const persistedThreadReasoningEffort = selectedModelProfile
    ? activeThread?.reasoningEffortByProfile?.[selectedModelProfile.id]
    : undefined;
  const storedReasoningEffort = selectedModelProfile
    ? persistedThreadReasoningEffort ??
      newThreadModelPreference.reasoningEffortByProfile[selectedModelProfile.id] ??
      selectedModelProfile.advanced?.defaultReasoningEffort
    : undefined;
  const selectedReasoningEffort =
    storedReasoningEffort && selectedReasoningCapability?.levels.includes(storedReasoningEffort)
      ? storedReasoningEffort
      : selectedReasoningCapability?.defaultLevel;
  const selectedReasoningEffortLabel = selectedReasoningEffort
    ? selectedReasoningCapability?.displayLabels?.[selectedReasoningEffort] ?? selectedReasoningEffort
    : undefined;
  const recoveredPreferenceKey = modelPreferenceRecoveryKey(
    activeThread?.threadId,
    activeThread?.selectedModelProfileId,
    selectedModelProfile?.id,
    persistedThreadReasoningEffort,
    selectedReasoningEffort,
  );
  useEffect(() => {
    if (!recoveredPreferenceKey || recoveredModelPreferenceRef.current === recoveredPreferenceKey) {
      return;
    }
    recoveredModelPreferenceRef.current = recoveredPreferenceKey;
    setNotice("原会话的模型或 effort 已失效，已恢复为当前可用默认值。");
  }, [recoveredPreferenceKey]);
  const defaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;
  const effectiveDefaultWorkspacePath = defaultProjectRemoved ? undefined : defaultWorkspacePath;
  // Without a user-selected workspace there is no implicit default project;
  // 项目工作区 shows a true empty state until the user opens/adds a project.
  const hasDefaultWorkspaceProject = Boolean(effectiveDefaultWorkspacePath?.trim());
  const activeThreadWorkspacePath = activeThread?.workspaceRoot;
  const activeThreadProjectId = activeThread
    ? projectIdForThread(activeThread, effectiveDefaultWorkspacePath)
    : undefined;
  const pathForSelectedProject =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === selectedProjectId)?.path;
  const workspaceRootForRun = resolveWorkspaceRootForRun({
    workspacePath,
    workspaceTargetPath: workspaceTargetPathRef.current,
    conversationScope,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  });
  const selectedFolderlessProject =
    selectedProjectId !== undefined &&
    selectedProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
    projectsFromWorkspace.find((project) => project.id === selectedProjectId)?.folders.length === 0;
  const isWorkspaceValid =
    conversationScope ||
    selectedFolderlessProject ||
    (workspaceRootForRun !== "" && /^([A-Za-z]:[\\/]|\/)/.test(workspaceRootForRun));
  const {
    composerFocusRequest,
    composerFocusOffset,
    requestComposerFocus,
    composerBarState,
    composerRuntimePanels,
  } = useComposerLifecycle({
    composerValue,
    desktopReady,
    desktopStarting,
    desktopNeedsRestart,
    isBusy,
    isWorkspaceValid,
    hasAttachments: composerAttachments.length > 0 || composerTokens.length > 0,
    inputAttachmentBlocked: false,
  });
  const composerInlineTextPrefix = useMemo(
    () =>
      composerTokens
        .map((token) => token.leadingText ?? composerTokenLeadingTexts[token.id] ?? "")
        .join(""),
    [composerTokenLeadingTexts, composerTokens],
  );
  const trimmedEffectiveComposerValue = `${composerInlineTextPrefix}${composerValue}`.trim();
  const composerActionTrigger = composerActionTriggerFromDraft({
    value: composerValue,
    tokens: composerTokens,
    leadingTexts: composerTokenLeadingTexts,
    selection: composerInputSelection,
  });
  const activeComposerActionProviders =
    composerActionTrigger?.marker === "@" ? mentionProviders : slashProviders;
  const slashPaletteOpen =
    Boolean(composerActionTrigger) && desktopReady && isWorkspaceValid && !isBusy;
  const filteredSlashActions = useMemo(
    () => filterSlashActions(slashActions, composerActionTrigger?.query ?? "").slice(0, 24),
    [composerActionTrigger?.query, slashActions],
  );
  const composerBarTokens = useMemo(
    () =>
      composerTokens.map((token) => ({
        id: token.id,
        type: token.type,
        label: token.label,
        placement: composerTokenPlacements[token.id] ?? "before",
        leadingText: token.leadingText ?? composerTokenLeadingTexts[token.id],
        title:
          typeof token.payload?.skillFilePath === "string"
            ? token.payload.skillFilePath
            : token.label,
      })),
    [composerTokenLeadingTexts, composerTokenPlacements, composerTokens],
  );
  const composerBarSlashActions = useMemo(
    () =>
      filteredSlashActions.map(
        ({ id, type, label, description, icon, disabled, disabledReason, preview }) => ({
          id,
          type,
          label,
          description,
          icon,
          disabled,
          disabledReason,
          preview,
        }),
      ),
    [filteredSlashActions],
  );
  const composerBarSlashPalette = {
    open: slashPaletteOpen,
    actions: composerBarSlashActions,
    activeActionId: slashActiveActionId,
    loading: slashLoading,
    error: slashError,
    emptyLabel: "没有匹配项",
    onSelectAction: handleSlashActionSelect,
    onActiveActionChange: setSlashActiveActionId,
  };

  useEffect(() => {
    if (!slashPaletteOpen) {
      setSlashLoading(false);
      setSlashError(null);
      setSlashActiveActionId(undefined);
      return;
    }

    let cancelled = false;
    setSlashLoading(true);
    setSlashError(null);
    setSlashActions([]);
    void actionsFromSlashProviders(activeComposerActionProviders, { workspaceRootForRun })
      .then((actions) => {
        if (cancelled) {
          return;
        }
        setSlashActions(actions);
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setSlashActions([]);
        setSlashError(error instanceof Error ? error.message : "命令菜单加载失败");
      })
      .finally(() => {
        if (!cancelled) {
          setSlashLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [activeComposerActionProviders, slashPaletteOpen, workspaceRootForRun]);

  useEffect(() => {
    if (!slashPaletteOpen) {
      return;
    }
    const activeStillVisible = filteredSlashActions.some(
      (action) => action.id === slashActiveActionId,
    );
    if (!activeStillVisible) {
      setSlashActiveActionId(firstSelectableSlashAction(filteredSlashActions)?.id);
    }
  }, [filteredSlashActions, slashActiveActionId, slashPaletteOpen]);

  const activeUserQuestion = runtimeState.userQuestions[0];
  const activeExternalInteraction = runtimeState.externalInteractions[0];
  const activeComputerReview = runtimeState.computerReviews[0];
  const activeComputerHandoff = runtimeState.computerHandoffs.find((item) => item.active);
  const navItems: NavItem[] = [
    {
      id: "connectors",
      label: "连接器",
      icon: <AwakenIcon concept="connector" size={20} />,
    },
    {
      id: "skills",
      label: "技能",
      icon: <AwakenIcon concept="skill" size={20} />,
    },
    {
      id: "scheduled-tasks",
      label: "定时任务",
      icon: <AwakenIcon concept="scheduled-task" size={20} />,
    },
  ];
  const topbarStatusLabel = desktopNeedsRestart
    ? "Needs restart"
    : desktopStarting
      ? "Starting"
      : undefined;
  useEffect(() => {
    const defaultProjectAvailable = !defaultProjectRemoved && hasDefaultWorkspaceProject;
    const knownProjectIds = new Set([
      ...(defaultProjectAvailable ? [DEFAULT_WORKSPACE_PROJECT_ID] : []),
      ...projectsFromWorkspace.map((project) => project.id),
    ]);
    const fallbackProjectId = defaultProjectAvailable
      ? DEFAULT_WORKSPACE_PROJECT_ID
      : projectsFromWorkspace[0]?.id;
    setSelectedProjectId((current) =>
      current === undefined
        ? undefined
        : knownProjectIds.has(current)
          ? current
          : fallbackProjectId,
    );
    setExpandedProjectIds((current) => {
      const next = current.filter((id) => knownProjectIds.has(id));
      return !defaultProjectAvailable || next.includes(DEFAULT_WORKSPACE_PROJECT_ID)
        ? next
        : [DEFAULT_WORKSPACE_PROJECT_ID, ...next];
    });
  }, [defaultProjectRemoved, hasDefaultWorkspaceProject, projectsFromWorkspace]);

  useEffect(() => {
    if (pathForSelectedProject && !isSameWorkspacePath(workspacePath, pathForSelectedProject)) {
      workspaceTargetPathRef.current = pathForSelectedProject;
      setWorkspacePath(pathForSelectedProject);
    }
  }, [pathForSelectedProject, workspacePath]);

  const projectLabelById = new Map<string, string>([
    [
      DEFAULT_WORKSPACE_PROJECT_ID,
      defaultWorkspacePath ? workspaceLabelForPath(defaultWorkspacePath) : "awaken",
    ],
    ...projectsFromWorkspace.map((project) => [project.id, project.label] as const),
  ]);
  const removedProjectIdSet = new Set(removedProjectIds);
  const workspaceLabelProjectId = activeThreadProjectId ?? selectedProjectId;
  useEffect(() => {
    dispatchFileWorkspaceTabs({ type: "reset" });
  }, [workspaceLabelProjectId]);
  const workspaceLabelPath = activeThreadWorkspacePath ?? pathForSelectedProject;
  const activeProjectLabel = conversationScope
    ? "选择项目"
    : ((workspaceLabelProjectId ? projectLabelById.get(workspaceLabelProjectId) : undefined) ??
      (workspaceLabelPath ? workspaceLabelForPath(workspaceLabelPath) : "awaken"));
  const selectedEndpointSelector = selectedModelProfile?.advanced?.endpointId?.trim();
  const selectedRuntimeModelIdentity = selectedModelProfile
    ? {
        ...selectedModelProfile,
        modelId: selectedEndpointSelector || selectedModelProfile.modelId,
        capabilityModelId: selectedModelProfile.modelId,
        endpointSelector: selectedEndpointSelector,
        modelSelectorKind: selectedEndpointSelector ? ("endpoint" as const) : ("model" as const),
        reasoningEffort: selectedReasoningEffort,
      }
    : undefined;
  const contextMeta = contextUsageMeta(
    runtimeState.inspectorSummary.contextWindow,
    selectedRuntimeModelIdentity,
  );
  const contextPanelData = contextUsagePanelData(
    runtimeState.inspectorSummary.contextWindow,
    selectedRuntimeModelIdentity,
  );
  const contextUsagePanelId = "awaken-context-usage-panel";

  function closeContextUsagePanel(returnFocus: boolean): void {
    setContextUsagePanelOpen(false);
    if (returnFocus && typeof window !== "undefined") {
      window.requestAnimationFrame(() => contextUsageTriggerRef.current?.focus());
    }
  }

  useEffect(() => {
    if (!contextUsagePanelOpen) {
      return;
    }
    function handlePointerDown(event: MouseEvent): void {
      const target = event.target as Node;
      if (
        !contextUsagePanelRef.current?.contains(target) &&
        !contextUsageTriggerRef.current?.contains(target)
      ) {
        closeContextUsagePanel(true);
      }
    }
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        closeContextUsagePanel(true);
      }
    }
    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [contextUsagePanelOpen]);

  useEffect(() => {
    setContextUsagePanelOpen(false);
    setSelectedSubagentTaskId(undefined);
    setOpenSubagentTaskId(undefined);
    setCancellingSubagentTaskId(undefined);
  }, [runtimeState.activeThreadId]);

  useEffect(() => {
    if (!contextPanelData) {
      setContextUsagePanelOpen(false);
    }
  }, [contextPanelData]);
  const modelMetaItem: ComposerBarMetaItem =
    enabledModelProfiles.length > 0 && selectedModelProfile
      ? {
          id: "model",
          label: `${modelProfileLabel(selectedModelProfile)}${selectedReasoningEffortLabel ? ` · ${selectedReasoningEffortLabel}` : ""}`,
          icon: modelProfileIcon(selectedModelProfile),
          disabled: isBusy,
          menuItems: enabledModelProfiles.map((profile) => {
            const provider = modelProviders.find((item) => item.id === profile.providerId);
            const capability =
              profile.effectiveCapability?.reasoningEffort ??
              provider?.modelCapabilities[profile.modelId]?.reasoningEffort;
            const storedEffort =
              activeThread?.reasoningEffortByProfile?.[profile.id] ??
              newThreadModelPreference.reasoningEffortByProfile[profile.id] ??
              profile.advanced?.defaultReasoningEffort;
            const effectiveEffort =
              storedEffort && capability?.levels.includes(storedEffort)
                ? storedEffort
                : capability?.defaultLevel;
            return {
              id: profile.id,
              label: modelProfileLabel(profile),
              icon: modelProfileIcon(profile),
              selected: profile.id === selectedModelProfile.id,
              disabled: isBusy,
              onSelect: () => {
                const change = modelPreferenceChangeForOption(
                  profile.id,
                  selectedModelProfile.id,
                  effectiveEffort,
                );
                if (change) {
                  void handleApplyModelPreference(
                    change.modelProfileId,
                    change.reasoningEffort,
                  );
                }
              },
              effort:
                capability && effectiveEffort
                  ? {
                      levels: capability.levels,
                      selectedLevel: effectiveEffort,
                      displayLabels: capability.displayLabels,
                      note: activeThread?.runId
                        ? "切换模型或推理强度会重建对话缓存，下次发送消息时生效。"
                        : undefined,
                      onChange: (level: ReasoningEffortLevel) => {
                        void handleApplyModelPreference(profile.id, level);
                      },
                    }
                  : undefined,
            };
          }),
          menuFooterAction: {
            label: "添加模型",
            icon: <AwakenIcon concept="settings" size={16} />,
            onClick: () => {
              setModelSettingsInitialModelSection("coding_plan");
              setModelSettingsOpen(true);
            },
          },
        }
      : {
          id: "model",
          label: "请点击头像配置模型",
          disabled: true,
        };
  const composerMetaItems: ComposerBarMetaItem[] = [
    modelMetaItem,
    { id: "workspace", label: activeProjectLabel },
    ...(contextMeta
      ? [
          {
            id: "context",
            ...contextMeta,
            onClick: () => {
              setOpenSubagentTaskId(undefined);
              runPlanEntry.closePanel(false);
              setContextUsagePanelOpen((current) => !current);
            },
            buttonRef: contextUsageTriggerRef,
            ariaHaspopup: "dialog" as const,
            ariaExpanded: contextUsagePanelOpen,
            ariaControls: contextUsagePanelId,
          },
        ]
      : []),
  ];

  function projectAction(projectId: string, label: string) {
    return (
      <SidebarSectionAction
        label={`New session in ${label}`}
        onClick={(event) => {
          event.stopPropagation();
          handleNewProjectSession(projectId);
        }}
      >
        <AwakenIcon concept="thread-new" size={16} />
      </SidebarSectionAction>
    );
  }

  function backendProjectIdFor(projectId: string): string | undefined {
    if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
      return defaultBackendProjectId;
    }
    const project = projectsFromWorkspace.find((item) => item.id === projectId);
    return project?.backendProjectId;
  }

  function requestProjectDelete(projectId: string, action: ProjectDeleteAction): void {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPendingProjectDelete({
      projectId,
      backendProjectId,
      label: projectLabelById.get(projectId) ?? "项目",
      action,
      threadCount: threadsForProject(projectId).length,
    });
  }

  async function handleRevealFolder(folderPath: string): Promise<void> {
    const result = await runtimeClient.revealPath(folderPath);
    if (!result.ok) {
      setNotice(result.message ?? "无法在文件管理器中定位该目录。");
    }
  }

  /**
   * 悬停项目行时展示这个项目由哪些文件夹组成。侧栏树本身不展开文件夹行，
   * 文件夹的完整视图和写操作都留在编辑项目弹窗里。
   */
  function projectHoverCard(
    projectId: string,
    label: string,
    threadCount: number,
  ): ProjectHoverCard {
    return {
      title: label,
      summary: threadCount > 0 ? `${threadCount} 个对话串` : "暂无对话串",
      folders: foldersForProjectId(projectId).map((folder) => ({
        path: folder.path,
        label: workspaceLabelForPath(folder.path),
      })),
      emptyFoldersLabel: "还没有文件夹，对话会在这个项目自己的工作目录里进行",
      revealHint: "在文件管理器中显示：",
      actionLabel: "编辑项目",
      onRevealFolder: (path) => {
        void handleRevealFolder(path);
      },
      onAction: () => {
        setEditingProjectId(projectId);
        setNotice(null);
      },
    };
  }

  /**
   * 项目层菜单只负责项目自身：加文件夹、编辑文件夹列表、从 Awaken 移除、删除历史。
   * 「移到废纸篓」是文件夹粒度的，只出现在编辑项目弹窗里的文件夹行菜单。
   */
  function projectMenuItems(projectId: string) {
    return [
      {
        id: "add-project-folder",
        label: "添加文件夹",
        onSelect: () => {
          void handleAddProjectFolder(projectId);
        },
      },
      {
        id: "edit-project",
        label: "编辑项目",
        onSelect: () => {
          setEditingProjectId(projectId);
          setNotice(null);
        },
      },
      {
        id: "remove-project",
        label: "从 Awaken 移除",
        onSelect: () => {
          requestProjectDelete(projectId, "detach");
        },
      },
      {
        id: "delete-project-history",
        label: "删除 Awaken 历史",
        tone: "danger" as const,
        onSelect: () => {
          requestProjectDelete(projectId, "delete_history");
        },
      },
    ];
  }

  function threadItems(threads: ThreadItem[]): ProjectItem[] {
    return threads.map((thread) => {
      return {
        id: `thread:${thread.threadId}`,
        kind: "thread",
        label: thread.label,
        isLoading: shouldShowSidebarThreadSpinner(thread),
        menuItems: sidebarThreadMenuItems(thread, {
          onRename: () => {
            void handleRenameThread(thread.threadId);
          },
          onArchive: () => {
            void handleArchiveThread(thread.threadId);
          },
          onDelete: () => {
            requestThreadDelete([thread.threadId], "single");
          },
        }),
      };
    });
  }

  function threadItemsForProject(projectId: string): ProjectItem[] {
    return threadItems(threadsForProject(projectId));
  }

  const conversationThreads = runtimeState.threads.filter(
    (thread) => projectIdForThread(thread, effectiveDefaultWorkspacePath) === undefined,
  );
  const eligibleConversationThreadIds = conversationThreads
    .filter((thread) => !shouldShowSidebarThreadSpinner(thread))
    .map((thread) => thread.threadId);
  const eligibleConversationThreadIdsKey = eligibleConversationThreadIds.join("\u0000");
  const eligibleConversationThreadIdSet = new Set(eligibleConversationThreadIds);
  useEffect(() => {
    if (!conversationBatchMode) {
      return;
    }
    setSelectedConversationThreadIds((current) => {
      const eligibleIds = new Set(eligibleConversationThreadIds);
      const next = current.filter((threadId) => eligibleIds.has(threadId));
      return next.length === current.length ? current : next;
    });
  }, [conversationBatchMode, eligibleConversationThreadIdsKey]);
  const selectedConversationThreadIdSet = new Set(
    selectedConversationThreadIds.filter((threadId) =>
      eligibleConversationThreadIdSet.has(threadId),
    ),
  );
  const selectedConversationCount = selectedConversationThreadIdSet.size;
  const allEligibleConversationsSelected =
    eligibleConversationThreadIds.length > 0 &&
    selectedConversationCount === eligibleConversationThreadIds.length;
  const conversationItems = threadItems(conversationThreads).map((item, index) => {
    if (!conversationBatchMode) {
      return item;
    }
    const thread = conversationThreads[index]!;
    const selectionDisabled = shouldShowSidebarThreadSpinner(thread);
    return {
      ...item,
      selection: {
        checked: selectedConversationThreadIdSet.has(thread.threadId),
        disabled: selectionDisabled,
        disabledReason: selectionDisabled ? "正在运行，停止后可删除" : undefined,
        onChange: (checked: boolean) => {
          setSelectedConversationThreadIds((current) => {
            if (checked) {
              return current.includes(thread.threadId) ? current : [...current, thread.threadId];
            }
            return current.filter((threadId) => threadId !== thread.threadId);
          });
        },
      },
    };
  });

  const defaultProjectThreads = threadItemsForProject(DEFAULT_WORKSPACE_PROJECT_ID);
  const unorderedRepositoryProjects: ProjectItem[] = [
    ...(removedProjectIdSet.has(DEFAULT_WORKSPACE_PROJECT_ID) || !hasDefaultWorkspaceProject
      ? []
      : [
          {
            id: DEFAULT_WORKSPACE_PROJECT_ID,
            kind: "project" as const,
            label: projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            path: defaultWorkspacePath,
            action: projectAction(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            ),
            menuItems: projectMenuItems(DEFAULT_WORKSPACE_PROJECT_ID),
            hoverCard: projectHoverCard(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
              defaultProjectThreads.length,
            ),
            children: defaultProjectThreads.length > 0 ? defaultProjectThreads : undefined,
          },
        ]),
    ...projectsFromWorkspace
      .filter((project) => !isSameWorkspacePath(project.path, effectiveDefaultWorkspacePath))
      .map((project) => {
        const threads = threadItemsForProject(project.id);
        return {
          id: project.id,
          kind: "project" as const,
          label: project.label,
          path: project.path,
          action: projectAction(project.id, project.label),
          menuItems: projectMenuItems(project.id),
          hoverCard: projectHoverCard(project.id, project.label, threads.length),
          children: threads.length > 0 ? threads : undefined,
        };
      }),
  ];
  const repositoryProjectById = new Map(
    unorderedRepositoryProjects.map((project) => [project.id, project]),
  );
  const orderedRepositoryProjectIds = new Set(projectOrderIds);
  const repositoryProjects: ProjectItem[] = [
    ...projectOrderIds.flatMap((id) => {
      const project = repositoryProjectById.get(id);
      return project ? [project] : [];
    }),
    ...unorderedRepositoryProjects.filter(
      (project) => !orderedRepositoryProjectIds.has(project.id),
    ),
  ];
  const allProjectsExpanded =
    repositoryProjects.length > 0 &&
    repositoryProjects.every((project) => expandedProjectIds.includes(project.id));
  const composerProjectPicker = {
    label: "选择项目",
    placeholder: "选择项目",
    options: [
      {
        id: "conversation",
        label: "选择项目",
        icon: null,
        selected: conversationScope,
        onSelect: handleSelectConversationScope,
      },
      ...repositoryProjects.map((project) => ({
        id: project.id,
        label: project.label,
        selected: !conversationScope && project.id === selectedProjectId,
        disabled: project.disabled,
        onSelect: () => handleSelectProject(project.id),
      })),
    ],
    emptyLabel: "未选择项目时，对话在电脑全局范围中运行。",
    clearAction: conversationScope
      ? undefined
      : {
          label: "取消选择项目",
          onClick: handleSelectConversationScope,
        },
    createAction: {
      label: "创建项目",
      disabled: !desktopReady || createProjectSubmitting,
      onClick: handleOpenCreateProject,
    },
  };
  const activeProjectId = runtimeState.activeThreadId
    ? `thread:${runtimeState.activeThreadId}`
    : selectedProjectId;
  const fileWorkspaceExpandedActive = fileWorkspaceOpen && fileWorkspaceExpanded;
  const fileWorkspaceSurfaceActive =
    fileWorkspaceExpandedActive && expandedWorkspaceSurface === "file";
  const expandedFileComposerCompletionKeyValue = expandedFileComposerCompletionKey({
    trackedRun: expandedFileComposerTrackedRun,
    currentThreadId: runtimeState.activeThreadId ?? runtimeState.threadId ?? undefined,
    currentRunId: runtimeState.runId ?? undefined,
    status: runtimeState.status,
    completedAtMs: runtimeState.runCompletedAtMs,
  });
  const expandedFileComposerCompleted = Boolean(expandedFileComposerCompletionKeyValue);
  const expandedFileComposerElapsedMs =
    expandedFileComposerCompletionKeyValue &&
    typeof runtimeState.runStartedAtMs === "number" &&
    typeof runtimeState.runCompletedAtMs === "number"
      ? Math.max(0, runtimeState.runCompletedAtMs - runtimeState.runStartedAtMs)
      : undefined;
  const expandedFileComposerForceFull = Boolean(
    composerRuntimePanels.length > 0 || activeUserQuestion || activeExternalInteraction,
  );
  const expandedFileComposerHasDraft = hasExpandedFileComposerDraft({
    effectiveText: trimmedEffectiveComposerValue,
    attachmentCount: composerAttachments.length,
    tokenCount: composerTokens.length,
  });
  const dockComposerMetaItems = composerMetaItemsForFileWorkspaceSurface(
    composerMetaItems,
    fileWorkspaceSurfaceActive,
  );
  const showCenterComposer = shouldUseCenterComposerWithFileWorkspace({
    standardCenterComposer: shouldUseCenterComposer({
      hasNarrative: visibleNarrativeItems.length > 0,
      hasRun: Boolean(runtimeState.runId),
      hasNotice: Boolean(notice),
      hasError: Boolean(runtimeState.errorMessage),
      needsRestart: desktopNeedsRestart,
    }),
    fileWorkspaceOpen,
    fileWorkspaceExpanded,
    expandedWorkspaceSurface,
  });
  const {
    activeWorkbenchTurnId,
    composerBarInsetPx,
    composerBarSafeAreaPx,
    composerContentInsetPx,
    setComposerDockElement,
    composerSafeAreaPx,
    handleReturnToLatestClick,
    handleRunStageScroll,
    handleTurnMarkerClick,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    returnToLatestBottomPx,
    returnToLatestVisible,
    runStageViewportHeightPx,
    setRunStageScrollElement,
  } = useRunStageScrollController({
    showCenterComposer,
    narrativeItems: visibleNarrativeItems,
  });
  const turnMarkerDisplay = turnMarkerRailDisplay({
    showCenterComposer,
    turnIds: narrativeSections.turns.map((turn) => turn.id),
    activeTurnId: activeWorkbenchTurnId,
    latestTurnId: latestPromptId,
    railHeight: turnMarkerRailHeight({
      scrollViewportHeight: runStageViewportHeightPx,
      composerContentInset: composerContentInsetPx,
      bottomSafeArea: composerBarSafeAreaPx,
    }),
  });
  const showTurnMarkerRail = !fileWorkspaceSurfaceActive && turnMarkerDisplay.mode !== "hidden";
  const activeNavSection = workbenchNavSectionForNavId(activeNavId);
  const activeExtensionSection: ExtensionSection | undefined =
    activeNavSection === "connectors" || activeNavSection === "skills"
      ? activeNavSection
      : undefined;
  const scheduledTasksNavActive = activeNavSection === "scheduled-tasks";
  const workbenchMediaSurfaceActive =
    !activeExtensionSection && !scheduledTasksNavActive;
  useEffect(() => {
    const wasActive = workbenchMediaSurfaceActiveRef.current;
    workbenchMediaSurfaceActiveRef.current = workbenchMediaSurfaceActive;
    if (!workbenchMediaSurfaceActive || wasActive) return;
    // Skills/Connectors/Scheduled Tasks unmount Workbench; reissue short-lived opaque sources on return.
    artifactMediaSourcePendingRef.current.clear();
    artifactMediaSourceRetryRef.current.clear();
    artifactMediaSourcesRef.current = {};
    setArtifactMediaSources({});
    setArtifactMediaHydrationEpoch((value) => value + 1);
  }, [workbenchMediaSurfaceActive]);
  const inspectorPresentation = workbenchInspectorPresentation(
    activeNavSection,
    inspectorCollapsed,
  );
  const inspectorCap = inspectorWidthCap({
    fileWorkspaceOpen,
    preferenceWidth: inspectorWidth,
    overrideWidth: fileWorkspaceInspectorWidth,
  });
  const effectiveInspectorWidth = computeEffectiveInspectorWidth({
    windowWidth,
    sidebarWidth,
    sidebarCollapsed,
    inspectorAvailable: inspectorPresentation.available,
    inspectorCollapsed: inspectorPresentation.effectiveCollapsed,
    cap: inspectorCap,
  });

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
        const receivedAtMs = Date.now();
        if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
          terminalSeenRef.current = true;
          localStorage.removeItem("awaken:runId");
        }
        if (event.type === "RUN_STATUS_CHANGED" && event.status === "cancelled") {
          localStorage.removeItem("awaken:runId");
        }
        dispatch({ type: "run/event", event, receivedAtMs });
      },
      onError: (message) => {
        if (message.toLowerCase().includes("run session was not found")) {
          localStorage.removeItem("awaken:runId");
          dispatch({
            type: "run/event",
            event: {
              type: "RUN_STATUS_CHANGED",
              rawType: "RUN_STATUS_CHANGED",
              status: "cancelled",
              message: "你停止生成了本次回答",
            },
            receivedAtMs: Date.now(),
          });
          return;
        }
        dispatch({ type: "run/streamFailed", message });
      },
      onReconnect: () => dispatch({ type: "run/reconnecting" }),
      onClose: () => {
        // Non-terminal close is handled by the client layer's attemptReconnect
        // (which dispatches run/reconnecting and retries). This callback only
        // fires on terminal close; the give-up path (max retries exceeded) is
        // delivered through onError.
        unsubscribeRef.current = null;
      },
    };
  }

  function applyRuntimeActions(actions: RuntimeAction[]): void {
    dispatch({ type: "run/replayBatch", actions });
  }

  function shouldResumeSubscription(status: RuntimeRunStatus): boolean {
    return status === "running" || status === "cancelling";
  }

  async function replayThreadTranscript(
    threadId: string,
    loadSeq: number,
  ): Promise<RuntimeRunStatus | undefined> {
    const transcript = await runtimeClient.getThreadTranscript(threadId);
    if (threadLoadSeqRef.current !== loadSeq) {
      return undefined;
    }

    syncProjectSelectionForThread({
      threadId: transcript.thread.threadId,
      projectId: transcript.thread.projectId,
      workspaceRoot: transcript.thread.workspace.workspaceRoot,
    });
    applyRuntimeActions(actionsForThreadTranscript(transcript));
    void runtimeClient
      .listMemoryDreams(transcript.thread.projectId, 5)
      .then((tasks) => {
        if (threadLoadSeqRef.current !== loadSeq) return;
        const assignedTasks = dreamTasksForThreadReplay(
          tasks,
          threadId,
          dreamReceiptThreadAssignmentsRef.current,
        );
        for (const task of assignedTasks.slice().reverse()) {
          const event = dreamTaskReceiptEvent(task);
          if (event) dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
        }
      })
      .catch(() => {
        // Dream receipts are auxiliary to transcript recovery; runtime errors
        // remain visible through the Settings task status.
      });

    const latestRun = transcript.runs[transcript.runs.length - 1]?.run;
    if (!latestRun) {
      localStorage.removeItem("awaken:runId");
      return undefined;
    }

    if (!shouldResumeSubscription(latestRun.status)) {
      localStorage.removeItem("awaken:runId");
      return latestRun.status;
    }

    // The transcript may report a run as running, but if the runtime process
    // was restarted the in-memory session is gone. Verify the run still exists
    // before subscribing; otherwise mark it as lost instead of retrying.
    try {
      await runtimeClient.getRun(latestRun.runId);
    } catch {
      localStorage.removeItem("awaken:runId");
      dispatch({
        type: "run/event",
        event: {
          type: "RUN_STATUS_CHANGED",
          rawType: "RUN_STATUS_CHANGED",
          status: "cancelled",
          message: "你停止生成了本次回答",
        },
        receivedAtMs: Date.now(),
      });
      return latestRun.status;
    }

    localStorage.setItem("awaken:runId", latestRun.runId);
    const unsubscribe = await runtimeClient.subscribeRun(
      latestRun.runId,
      createSubscribeHandlers(),
      streamCursorForTranscriptRun(transcript, latestRun.runId),
    );
    if (threadLoadSeqRef.current !== loadSeq) {
      unsubscribe();
      return latestRun.status;
    }

    unsubscribeRef.current = unsubscribe;
    return latestRun.status;
  }

  async function handleAddProjectEntryToTask(
    request: ProjectEntryActionRequest,
  ): Promise<{ ok: true } | { ok: false; message: string }> {
    if (isBusy) {
      return { ok: false, message: "任务执行中，结束后再添加文件上下文。" };
    }
    const result = await runtimeClient.resolveProjectEntryForTask(request);
    if (!result.ok) {
      setNotice(result.message);
      return { ok: false, message: result.message };
    }

    const existingToken = composerTokens.find(
      (token) =>
        token.type === "file" &&
        token.payload?.path === result.entry.absolutePath &&
        token.payload?.projectId === result.entry.projectId &&
        token.payload?.folderPath === result.entry.folderPath,
    );
    if (existingToken) {
      const offset = composerTokenEndOffset(
        composerTokens,
        composerTokenLeadingTexts,
        existingToken.id,
      );
      setExpandedFileComposerTrackedRun(undefined);
      setExpandedFileComposerIdleMode("full");
      if (offset !== undefined) {
        setComposerInputSelection({ anchor: offset, focus: offset });
      }
      requestComposerFocus(offset);
      return { ok: true };
    }

    const payload: ProjectPathComposerTokenPayload = {
      path: result.entry.absolutePath,
      entryKind: result.entry.kind,
      projectId: result.entry.projectId,
      folderPath: result.entry.folderPath,
      relativePath: result.entry.relativePath,
    };
    const token: ComposerToken = {
      id: `project-path:${window.crypto.randomUUID()}`,
      type: "file",
      label: result.entry.displayName,
      payload: payload as unknown as Record<string, unknown>,
    };
    const draftLength = composerStructuredDraftLength(
      composerTokens,
      composerTokenLeadingTexts,
      composerValue,
    );
    const offset = composerInputSelection?.focus ?? draftLength;
    const nextDraft = insertComposerTokenAtOffset({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      offset,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("full");
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    requestComposerFocus(nextDraft.focusOffset);
    setNotice(null);
    return { ok: true };
  }

  function handleSlashActionSelect(actionId: string): void {
    const action = filteredSlashActions.find((item) => item.id === actionId);
    if (!action || action.disabled) {
      return;
    }
    const token = composerTokenFromSlashAction(action);
    if (!token) {
      return;
    }
    if (!composerActionTrigger) {
      return;
    }
    const nextDraft = replaceComposerActionTriggerWithToken({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      trigger: composerActionTrigger,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    setSlashActiveActionId(undefined);
    requestComposerFocus(nextDraft.focusOffset);
  }

  function handleRemoveComposerToken(tokenId: string): void {
    const removedTokenIndex = composerTokens.findIndex((item) => item.id === tokenId);
    const removedToken = composerTokens[removedTokenIndex];
    const leadingText = removedToken?.leadingText ?? composerTokenLeadingTexts[tokenId] ?? "";
    const nextToken = composerTokens[removedTokenIndex + 1];
    if (leadingText && nextToken) {
      const nextLeadingText =
        nextToken.leadingText ?? composerTokenLeadingTexts[nextToken.id] ?? "";
      const mergedLeadingText = `${leadingText}${nextLeadingText}`;
      setComposerTokens((current) =>
        current
          .filter((token) => token.id !== tokenId)
          .map((token) =>
            token.id === nextToken.id ? { ...token, leadingText: mergedLeadingText } : token,
          ),
      );
      setComposerTokenLeadingTexts((current) => {
        const next = { ...current, [nextToken.id]: mergedLeadingText };
        delete next[tokenId];
        return next;
      });
    } else if (leadingText) {
      setComposerValue((current) => `${leadingText}${current}`);
      setComposerTokens((current) => current.filter((token) => token.id !== tokenId));
    } else {
      setComposerTokens((current) => current.filter((token) => token.id !== tokenId));
    }
    setComposerTokenPlacements((current) => {
      if (!(tokenId in current)) {
        return current;
      }
      const next = { ...current };
      delete next[tokenId];
      return next;
    });
    if (!nextToken) {
      setComposerTokenLeadingTexts((current) => {
        if (!(tokenId in current)) {
          return current;
        }
        const next = { ...current };
        delete next[tokenId];
        return next;
      });
    }
    requestComposerFocus();
  }

  function handleComposerStructuredInputChange(nextValue: ComposerBarStructuredInputValue): void {
    const nextDraft = reconcileComposerStructuredInput(
      composerTokens,
      composerTokenPlacements,
      nextValue,
    );
    setComposerValue(nextDraft.value);
    setComposerTokens(nextDraft.tokens);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerTokenPlacements(nextDraft.placements);
  }

  function handleComposerInputKeyDown(
    event: ReactKeyboardEvent<HTMLTextAreaElement | HTMLDivElement>,
  ): void {
    if (!slashPaletteOpen) {
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      if (composerActionTrigger) {
        const nextDraft = removeComposerTextRange({
          tokens: composerTokens,
          placements: composerTokenPlacements,
          leadingTexts: composerTokenLeadingTexts,
          value: composerValue,
          start: composerActionTrigger.start,
          end: composerActionTrigger.end,
        });
        setComposerTokens(nextDraft.tokens);
        setComposerTokenPlacements(nextDraft.placements);
        setComposerTokenLeadingTexts(nextDraft.leadingTexts);
        setComposerValue(nextDraft.value);
        setComposerInputSelection({
          anchor: composerActionTrigger.start,
          focus: composerActionTrigger.start,
        });
        requestComposerFocus(composerActionTrigger.start);
      }
      setSlashActiveActionId(undefined);
      return;
    }
    if (
      event.key !== "ArrowDown" &&
      event.key !== "ArrowUp" &&
      event.key !== "Enter" &&
      event.key !== "Tab"
    ) {
      return;
    }
    event.preventDefault();
    const selectableActions = filteredSlashActions.filter((action) => !action.disabled);
    if (selectableActions.length === 0) {
      return;
    }
    const activeIndex = Math.max(
      0,
      selectableActions.findIndex((action) => action.id === slashActiveActionId),
    );
    if (event.key === "ArrowDown") {
      setSlashActiveActionId(selectableActions[(activeIndex + 1) % selectableActions.length]?.id);
      return;
    }
    if (event.key === "ArrowUp") {
      setSlashActiveActionId(
        selectableActions[(activeIndex - 1 + selectableActions.length) % selectableActions.length]
          ?.id,
      );
      return;
    }
    handleSlashActionSelect(slashActiveActionId ?? selectableActions[0]?.id ?? "");
  }

  async function handleSubmitWithPrompt(options?: {
    prompt?: string;
    forceNewThread?: boolean;
  }): Promise<void> {
    if (!desktopReady) {
      setNotice(desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting");
      return;
    }
    const prompt = (options?.prompt ?? trimmedEffectiveComposerValue).trim();
    const forceNewThread = options?.forceNewThread === true;
    const draftComposerValue = composerValue;
    const draftAttachments = composerAttachments.map((attachment) => ({
      ...attachment,
    }));
    const draftComposerTokens = composerTokens.map((token) => ({
      ...token,
      payload: token.payload ? { ...token.payload } : undefined,
    }));
    const draftComposerTokenPlacements = { ...composerTokenPlacements };
    const draftComposerTokenLeadingTexts = { ...composerTokenLeadingTexts };
    const draftComposerState: ThreadComposerDraft = {
      value: draftComposerValue,
      attachments: draftAttachments,
      tokens: draftComposerTokens,
      tokenPlacements: draftComposerTokenPlacements,
      tokenLeadingTexts: draftComposerTokenLeadingTexts,
      selection: composerInputSelection
        ? { anchor: composerInputSelection.anchor, focus: composerInputSelection.focus }
        : undefined,
    };
    const draftPromptTokens = promptNarrativeTokensForComposer(
      draftComposerTokens,
      draftComposerTokenLeadingTexts,
    );
    const runInputAttachments = draftAttachments.map(toRunInputAttachment);
    if (!prompt && runInputAttachments.length === 0 && draftComposerTokens.length === 0) {
      setNotice("请输入内容或添加附件");
      return;
    }
    if (isBusy && !forceNewThread) {
      return;
    }
    if (!isWorkspaceValid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const workspaceRoot = resolveWorkspaceRootForRun({
      workspacePath,
      workspaceTargetPath: workspaceTargetPathRef.current,
      conversationScope,
      selectedProjectId,
      defaultWorkspacePath,
      defaultProjectRemoved,
      activeThreadWorkspacePath,
      projectsFromWorkspace,
    });
    // An empty root means the selected project has no folder bound, so there is
    // no directory to validate; Core resolves its session workspace instead.
    if (workspaceRoot) {
      const validation = await runtimeClient.validateWorkspace(workspaceRoot);
      if (!validation.valid) {
        setNotice(validation.error);
        return;
      }
    }
    const trackExpandedFileRun = fileWorkspaceSurfaceActive;
    setExpandedFileComposerTrackedRun(undefined);
    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    prepareForRunStart();
    dispatch({
      type: "run/requested",
      prompt,
      promptAttachments: forceNewThread ? [] : runInputAttachments,
      promptTokens: forceNewThread ? [] : draftPromptTokens,
      startedAtMs: Date.now(),
    });
    clearThreadComposerDraft();

    try {
      const { projectId: runProjectId, projectName: runProjectName } = resolveRunProjectForRun({
        workspaceRoot,
        selectedProjectId,
        defaultWorkspacePath,
        effectiveDefaultWorkspacePath,
        projectsFromWorkspace,
      });
      const runRoots = resolveRunRootsForRun({
        workspaceRoot,
        projectsFromWorkspace,
        projectId: runProjectId,
      });
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
        threadId: forceNewThread ? undefined : runtimeState.activeThreadId || undefined,
        modelProfileId: selectedModelProfile?.id,
        reasoningEffort: selectedReasoningEffort,
        workspace: {
          // projectId is what lets Core place a folder-less run in that
          // project's own session workspace rather than guess an identity from
          // the path. With no folder there is no root to declare either.
          projectId: runProjectId ? backendProjectIdFor(runProjectId) : undefined,
          // The agent is told which project it is in, so it can answer about the
          // folder group as a whole instead of only seeing its cwd.
          projectName: runProjectName,
          workspaceRoot,
          readableRoots: runRoots.length > 0 ? runRoots : undefined,
          writableRoots: runRoots.length > 0 ? runRoots : undefined,
          outputRoots: runRoots.length > 0 ? runRoots : undefined,
          attachedFiles: attachedFiles.length > 0 ? attachedFiles : undefined,
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      if (trackExpandedFileRun) {
        setExpandedFileComposerTrackedRun({
          threadId: run.threadId,
          runId: run.runId,
        });
      }
      if (runProjectId === DEFAULT_WORKSPACE_PROJECT_ID) {
        setRemovedProjectIds((current) =>
          current.filter((projectId) => projectId !== DEFAULT_WORKSPACE_PROJECT_ID),
        );
      }
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
      localStorage.setItem("awaken:runId", run.runId);
      const unsubscribe = await runtimeClient.subscribeRun(run.runId, createSubscribeHandlers());
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create run";
      replaceComposerDraft(draftComposerState, composerDraftKey);
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleComposerSubmit(): Promise<void> {
    if (isBusy) {
      await handleCancel();
      requestComposerFocus();
      return;
    }
    await handleSubmitWithPrompt();
    requestComposerFocus();
  }

  async function handleSelectConversation(
    threadId: string,
    prefetchedThreads?: ThreadItem[],
    forceReplay = false,
  ): Promise<void> {
    if (!desktopReady) {
      return;
    }
    // Exit any extension-level page (scheduled tasks, connectors, skills) so
    // that activation from system notifications or execution history always
    // lands on the workbench thread view.
    setActiveNavId(undefined);
    if (!forceReplay && threadId === runtimeState.activeThreadId) {
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    const loadSeq = threadLoadSeqRef.current + 1;
    threadLoadSeqRef.current = loadSeq;
    terminalSeenRef.current = false;
    setNotice(null);
    prepareForThreadSwitch();
    dispatch({ type: "run/threadSwitch", threadId });

    const threads = prefetchedThreads ?? runtimeState.threads;
    const thread = threads.find((item) => item.threadId === threadId);
    syncProjectSelectionForThread(thread);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      await replayThreadTranscript(threadId, loadSeq);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load thread");
    }
  }

  function threadsForProject(projectId: string): ThreadItem[] {
    return runtimeState.threads.filter(
      (thread) => projectIdForThread(thread, effectiveDefaultWorkspacePath) === projectId,
    );
  }

  function syncProjectSelectionForThread(
    thread?: Pick<ThreadItem, "threadId" | "projectId" | "workspaceRoot">,
  ): void {
    if (!thread) {
      return;
    }
    const nextProjectId = projectIdForThread(thread, effectiveDefaultWorkspacePath);
    if (!nextProjectId) {
      setConversationScope(true);
      setSelectedProjectId(undefined);
      workspaceTargetPathRef.current = thread.workspaceRoot ?? "";
      setWorkspacePath(thread.workspaceRoot ?? "");
      setConversationSectionExpanded(true);
      return;
    }
    setConversationScope(false);
    setSelectedProjectId(nextProjectId);
    const projectHasNoFolders =
      nextProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
      projectsFromWorkspace.find((project) => project.id === nextProjectId)?.folders.length === 0;
    const nextWorkspacePath = projectHasNoFolders ? "" : (thread.workspaceRoot ?? "");
    workspaceTargetPathRef.current = nextWorkspacePath;
    setWorkspacePath(nextWorkspacePath);
    setExpandedProjectIds((current) =>
      current.includes(nextProjectId) ? current : [...current, nextProjectId],
    );
  }

  async function refreshAfterThreadMutation(removedThreadIds: string[] = []): Promise<void> {
    if (removedThreadIds.includes(runtimeState.activeThreadId ?? "")) {
      resetActiveThreadContext();
    }

    await loadThreadsIntoState();
  }

  async function handleConfirmProjectDelete(): Promise<void> {
    const pending = pendingProjectDelete;
    if (!pending || projectDeleteSubmitting || !desktopReady) {
      return;
    }
    setProjectDeleteSubmitting(true);
    setNotice(null);
    const idempotencyKey = window.crypto.randomUUID();
    try {
      const result =
        pending.action === "detach"
          ? await runtimeClient.detachProject(pending.backendProjectId, idempotencyKey)
          : pending.action === "delete_history"
            ? await runtimeClient.deleteProjectThreads(pending.backendProjectId, idempotencyKey)
            : await runtimeClient.trashProjectFolder(
                pending.backendProjectId,
                pending.folderPath ?? "",
                idempotencyKey,
              );
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      const operation = result.operation;
      const operationState = projectOperationReconciliationState(operation);
      if (operationState === "failed") {
        await Promise.all([refreshProjects(), loadThreadsIntoState()]);
        setNotice(
          pending.action === "trash"
            ? "未能移到废纸篓，项目、历史和磁盘文件均保持不变。"
            : "项目操作未完成，请稍后重试。",
        );
        setPendingProjectDelete(null);
        return;
      }

      if (pending.action === "trash" && operationState === "pending") {
        setTrackedTrashOperations((current) =>
          upsertTrackedTrashOperation(current, {
            operationId: operation.operationId,
            projectId: pending.projectId,
            backendProjectId: pending.backendProjectId,
            folderPath: pending.folderPath ?? "",
            label: pending.label,
            wasExpanded: expandedProjectIds.includes(pending.projectId),
          }),
        );
        if (isSameWorkspacePath(activeThreadWorkspacePath, pending.folderPath)) {
          resetActiveThreadContext();
        }
        setPendingProjectDelete(null);
        setNotice(`正在将“${pending.label}”移到废纸篓，Awaken 会自动完成。`);
        const refreshResults = await Promise.allSettled([
          refreshProjects(),
          loadThreadsIntoState(),
        ]);
        for (const refreshResult of refreshResults) {
          if (refreshResult.status === "rejected") {
            console.warn("project_operation_initial_refresh_failed", refreshResult.reason);
          }
        }
        return;
      }

      if (
        pending.action === "detach" ||
        (pending.action === "trash" &&
          isSameWorkspacePath(activeThreadWorkspacePath, pending.folderPath))
      ) {
        resetActiveThreadContext();
      }
      await Promise.all([refreshProjects(), loadThreadsIntoState()]);
      setExpandedProjectIds((current) =>
        pending.action === "detach" ? current.filter((id) => id !== pending.projectId) : current,
      );
      setPendingProjectDelete(null);
      setNotice(
        pending.action === "detach"
          ? `已从 Awaken 移除“${pending.label}”，磁盘文件和历史均已保留。`
          : pending.action === "delete_history"
            ? `已删除“${pending.label}”的 Awaken 历史，磁盘文件未改变。`
            : `已将“${pending.label}”移到废纸篓，项目的其余文件夹不受影响。`,
      );
    } catch (error) {
      console.error("project_delete_ui_failed", error);
      setNotice("项目操作暂时失败，请稍后重试。");
      await Promise.all([refreshProjects().catch(() => []), refreshThreads()]);
    } finally {
      setProjectDeleteSubmitting(false);
    }
  }

  async function handleRenameThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find((item) => item.threadId === threadId);
    if (!thread) {
      setNotice("会话不存在");
      return;
    }

    setRenamingThreadId(threadId);
    setRenameDraft(thread.label);
    setRenameDialogOpen(true);
  }

  async function handleArchiveThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    setNotice(null);
    try {
      const result = await runtimeClient.setThreadArchived(threadId, true);
      if (!result.ok) {
        // 失败保留原行，不做无法核实的乐观删除。
        setNotice(result.message);
        return;
      }

      if (threadId === runtimeState.activeThreadId) {
        handleCloseFileWorkspace();
        setSelectedSubagentTaskId(undefined);
        setOpenSubagentTaskId(undefined);
      }
      clearThreadComposerDraft(composerDraftOwnerKey(threadId));
      // active authority 列表不再包含该 thread；若它是当前 thread，
      // refreshAfterThreadMutation 会经 reducer 回到“新建任务”空态，项目选择保持不变。
      await refreshAfterThreadMutation([threadId]);
      setNotice("已归档会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "归档会话失败，请稍后重试");
    }
  }

  async function handleConfirmRename(): Promise<void> {
    const threadId = renamingThreadId;
    if (!threadId) {
      return;
    }

    const nextLabel = renameDraft.trim();
    if (!nextLabel) {
      return;
    }

    const thread = runtimeState.threads.find((item) => item.threadId === threadId);
    if (!thread || nextLabel === thread.label) {
      setRenameDialogOpen(false);
      setRenamingThreadId(null);
      setRenameDraft("");
      return;
    }

    setRenameSubmitting(true);
    try {
      const updatedThread = await runtimeClient.renameThread(threadId, nextLabel);
      dispatch({ type: "run/threadRenamed", thread: updatedThread });
      setNotice("已重命名会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "重命名会话失败");
    } finally {
      setRenameSubmitting(false);
      setRenameDialogOpen(false);
      setRenamingThreadId(null);
      setRenameDraft("");
    }
  }

  function handleCancelRename(): void {
    setRenameDialogOpen(false);
    setRenamingThreadId(null);
    setRenameDraft("");
  }

  function enterConversationBatchMode(): void {
    setConversationSectionExpanded(true);
    setSelectedConversationThreadIds([]);
    setConversationBatchMode(true);
    setNotice(null);
  }

  function exitConversationBatchMode(): void {
    setConversationBatchMode(false);
    setSelectedConversationThreadIds([]);
  }

  function requestThreadDelete(
    threadIds: string[],
    source: PendingThreadDelete["source"],
  ): void {
    if (!desktopReady) {
      return;
    }
    const existingThreadIds = new Set(runtimeState.threads.map((thread) => thread.threadId));
    const requestedThreadIds = threadIds.filter((threadId) => existingThreadIds.has(threadId));
    if (requestedThreadIds.length === 0) {
      setNotice("没有可删除的会话");
      return;
    }
    setPendingThreadDelete({ threadIds: requestedThreadIds, source });
    setNotice(null);
  }

  async function handleConfirmThreadDelete(): Promise<void> {
    const pending = pendingThreadDelete;
    if (!pending || threadDeleteSubmitting || !desktopReady) {
      return;
    }

    setThreadDeleteSubmitting(true);
    try {
      const result = await runtimeClient.deleteThreads(pending.threadIds);
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      await refreshAfterThreadMutation(result.deletedThreadIds);
      const failedThreadIds = result.failures.map((failure) => failure.threadId);
      if (pending.source === "batch") {
        setSelectedConversationThreadIds(failedThreadIds);
        if (failedThreadIds.length === 0) {
          setConversationBatchMode(false);
        }
      }

      if (result.failures.length === 0) {
        setNotice(
          result.deletedThreadIds.length === 1
            ? "已删除会话"
            : `已删除 ${result.deletedThreadIds.length} 个对话`,
        );
      } else {
        const busyCount = result.failures.filter(
          (failure) => failure.code === "THREAD_BUSY",
        ).length;
        const failureSummary = busyCount === result.failures.length
          ? `${busyCount} 个会话正在运行，停止后可继续删除`
          : `${result.failures.length} 个会话未能删除，请稍后重试`;
        setNotice(
          result.deletedThreadIds.length > 0
            ? `已删除 ${result.deletedThreadIds.length} 个对话；${failureSummary}`
            : failureSummary,
        );
      }
      setPendingThreadDelete(null);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除会话失败");
      await refreshThreads();
    } finally {
      setThreadDeleteSubmitting(false);
    }
  }

  function handleSelectProject(id: string): void {
    exitConversationBatchMode();
    setActiveNavId(undefined);
    if (id.startsWith("thread:")) {
      void handleSelectConversation(id.slice("thread:".length));
      return;
    }

    setConversationScope(false);

    const projectPath =
      id === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === id)?.path;
    if (
      (runtimeState.activeThreadId && activeThreadProjectId !== id) ||
      shouldResetActiveThreadForWorkspaceChange(activeThreadWorkspacePath, projectPath)
    ) {
      resetActiveThreadContext();
    }
    setSelectedProjectId(id);
    workspaceTargetPathRef.current = projectPath ?? "";
    setWorkspacePath(projectPath ?? "");
  }

  async function handleReorderProjects(ids: string[]): Promise<void> {
    if (!desktopReady || projectReorderSubmittingRef.current) return;
    const backendIds = resolveBackendProjectOrder(
      ids,
      defaultBackendProjectId,
      projectsFromWorkspace,
    );
    if (!backendIds) {
      setNotice("项目顺序暂时无法保存，请刷新后重试。");
      await refreshProjects();
      return;
    }

    const previousOrder = projectOrderIds;
    projectReorderSubmittingRef.current = true;
    setProjectReorderSubmitting(true);
    setProjectOrderIds(ids);
    setNotice(null);
    try {
      const result = await runtimeClient.reorderProjects(backendIds);
      if (!result.ok) {
        setProjectOrderIds(previousOrder);
        setNotice(result.message);
        await refreshProjects();
        return;
      }
      applyRuntimeProjects(result.projects);
    } catch (error) {
      setProjectOrderIds(previousOrder);
      setNotice(
        error instanceof Error ? error.message : "项目顺序暂时未保存，已恢复原顺序，请稍后重试。",
      );
      await refreshProjects();
    } finally {
      projectReorderSubmittingRef.current = false;
      setProjectReorderSubmitting(false);
    }
  }

  function handleOpenCreateProject(): void {
    setPendingProjectMode("local");
    setDraftProjectFolders([]);
    setDraftPrimaryFolder("");
    setDraftProjectName("");
    setCreateProjectDialogOpen(true);
    setNotice(null);
  }

  function foldersForProjectId(projectId: string): WorkspaceProjectFolder[] {
    if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
      return defaultProjectFolders;
    }
    return projectsFromWorkspace.find((project) => project.id === projectId)?.folders ?? [];
  }

  /** 目标项目之外，所有已经占用了某个目录的项目。 */
  /** 选一个或多个目录，逐个通过 Core 校验后再判定哪些能加入。 */
  async function pickProjectFolders(
    excludedPaths: string[],
  ): Promise<{ paths: string[]; skipped: string[] }> {
    const result = await runtimeClient.pickProjectFolders(workspacePath || defaultWorkspacePath);
    if (result.canceled) {
      if (result.error) {
        setNotice(result.error);
      }
      return { paths: [], skipped: [] };
    }
    const readable: string[] = [];
    const skipped: string[] = [...(result.errors ?? [])];
    for (const candidate of result.paths) {
      const validation = await runtimeClient.validateWorkspace(candidate);
      if (validation.valid) {
        readable.push(candidate);
      } else {
        skipped.push(`“${workspaceLabelForPath(candidate)}”：${validation.error}`);
      }
    }
    const partition = partitionPickedProjectFolders({
      candidates: readable,
      attachedPaths: excludedPaths,
    });
    return {
      paths: partition.accepted,
      skipped: [...skipped, ...partition.skipped],
    };
  }

  async function handleAddDraftProjectFolder(): Promise<void> {
    if (pickingWorkspace) {
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);
    try {
      const { paths, skipped } = await pickProjectFolders(draftProjectFolders);
      if (skipped.length > 0) {
        setNotice(skipped.join(" "));
      }
      if (paths.length === 0) {
        return;
      }
      setDraftProjectFolders((current) => [...current, ...paths]);
      setDraftPrimaryFolder((current) => current || paths[0]);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to select folder");
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleRemoveDraftProjectFolder(path: string): void {
    setDraftProjectFolders((current) => {
      const next = current.filter((candidate) => !isSameWorkspacePath(candidate, path));
      setDraftPrimaryFolder((primary) =>
        isSameWorkspacePath(primary, path) ? (next[0] ?? "") : primary,
      );
      return next;
    });
  }

  async function handleCreateProject(): Promise<void> {
    if (createProjectSubmitting || !desktopReady) {
      return;
    }
    setCreateProjectSubmitting(true);
    setNotice(null);
    try {
      const folders = draftProjectFolders.map((path) => ({
        path,
        primary: isSameWorkspacePath(path, draftPrimaryFolder),
      }));
      // The name the user typed wins. Without one, a bound folder still supplies
      // a sensible label, and a project with neither is named by the backend.
      const displayName =
        draftProjectName.trim() ||
        (draftPrimaryFolder ? workspaceLabelForPath(draftPrimaryFolder) : undefined);
      const result = await runtimeClient.createProject(folders, displayName);
      if (!result.ok) {
        setNotice(
          result.code === "PROJECT_OPERATION_CONFLICT" ? "该项目正在移除中" : result.message,
        );
        return;
      }

      const project = result.project;
      const nextProjectId = isSameWorkspacePath(
        project.workspaceRoot,
        effectiveDefaultWorkspacePath,
      )
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : project.projectId;
      await refreshProjects();
      if (
        shouldResetActiveThreadForWorkspaceChange(activeThreadWorkspacePath, project.workspaceRoot)
      ) {
        resetActiveThreadContext();
      }
      // A folder-less project has no path to select. Leaving the target and the
      // workspace path untouched would keep pointing at the previous project, so
      // both are cleared and the selection carries the identity instead.
      workspaceTargetPathRef.current = project.workspaceRoot;
      setWorkspacePath(project.workspaceRoot);
      setConversationScope(false);
      setSelectedProjectId(nextProjectId);
      setExpandedProjectIds((current) =>
        current.includes(nextProjectId) ? current : [...current, nextProjectId],
      );
      setCreateProjectDialogOpen(false);
      setDraftProjectFolders([]);
      setDraftPrimaryFolder("");
      setDraftProjectName("");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to create project");
    } finally {
      setCreateProjectSubmitting(false);
    }
  }

  async function runProjectFolderMutation(
    projectId: string,
    folderPath: string,
    mutate: (backendProjectId: string) => Promise<UpsertProjectResult>,
    successNotice: string,
  ): Promise<void> {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    if (projectFolderBusyPath) {
      return;
    }
    setProjectFolderBusyPath(folderPath);
    setNotice(null);
    try {
      const result = await mutate(backendProjectId);
      if (!result.ok) {
        setNotice(
          result.code === "PROJECT_FOLDER_NOT_FOUND"
            ? "该文件夹已经不在这个项目里。"
            : result.message,
        );
        await refreshProjects();
        return;
      }
      await refreshProjects();
      setNotice(successNotice);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "项目文件夹操作暂时失败。");
      await refreshProjects().catch(() => []);
    } finally {
      setProjectFolderBusyPath(undefined);
    }
  }

  /**
   * 逐个挂载并汇总结果。走 runProjectFolderMutation 的循环会为每个目录刷新一次项目
   * 列表并覆盖一次 notice，选中五个目录时用户只会看到最后一条。
   */
  async function attachProjectFolders(
    backendProjectId: string,
    paths: string[],
    skipped: string[],
  ): Promise<void> {
    const attached: string[] = [];
    const failures: string[] = [...skipped];
    for (const path of paths) {
      const label = workspaceLabelForPath(path);
      setProjectFolderBusyPath(path);
      try {
        const result = await runtimeClient.attachProjectFolder(backendProjectId, path);
        if (result.ok) {
          attached.push(label);
        } else {
          failures.push(`“${label}”：${result.message}`);
        }
      } catch (error) {
        failures.push(`“${label}”：${error instanceof Error ? error.message : "加入失败"}`);
      }
    }
    setProjectFolderBusyPath(undefined);
    await refreshProjects().catch(() => []);
    const success =
      attached.length > 0
        ? `已把${attached.map((label) => `“${label}”`).join("、")}加入项目。`
        : "";
    setNotice([success, ...failures].filter(Boolean).join(" ") || null);
  }

  async function handleAddProjectFolder(projectId: string): Promise<void> {
    if (pickingWorkspace || projectFolderBusyPath) {
      return;
    }
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);
    try {
      const { paths, skipped } = await pickProjectFolders(
        foldersForProjectId(projectId).map((folder) => folder.path),
      );
      if (paths.length === 0) {
        if (skipped.length > 0) {
          setNotice(skipped.join(" "));
        }
        return;
      }
      await attachProjectFolders(backendProjectId, paths, skipped);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to select folder");
    } finally {
      setPickingWorkspace(false);
    }
  }

  async function handleSetPrimaryProjectFolder(projectId: string, path: string): Promise<void> {
    await runProjectFolderMutation(
      projectId,
      path,
      (id) => runtimeClient.setPrimaryProjectFolder(id, path),
      `已把“${workspaceLabelForPath(path)}”设为主文件夹。`,
    );
  }

  async function handleDetachProjectFolder(projectId: string, path: string): Promise<void> {
    await runProjectFolderMutation(
      projectId,
      path,
      (id) => runtimeClient.detachProjectFolder(id, path),
      `已把“${workspaceLabelForPath(path)}”从项目移除，磁盘文件保留。`,
    );
  }

  function requestProjectFolderTrash(projectId: string, path: string): void {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPendingProjectDelete({
      projectId,
      backendProjectId,
      label: workspaceLabelForPath(path),
      action: "trash",
      threadCount: threadsForProject(projectId).length,
      folderPath: path,
    });
  }

  function projectFolderRows(projectId: string): ProjectFolderRow[] {
    // Every folder is one the user picked, so every row offers the same actions.
    // Removing the last one is allowed: the project keeps its name and threads
    // and goes back to running without a folder.
    return foldersForProjectId(projectId).map((folder) => ({
      path: folder.path,
      isPrimary: folder.isPrimary,
      onMakePrimary: (path: string) => {
        void handleSetPrimaryProjectFolder(projectId, path);
      },
      onRemove: (path: string) => {
        void handleDetachProjectFolder(projectId, path);
      },
      // × 只把文件夹移出项目，磁盘文件保留。真正动磁盘的废纸篓留在菜单里，
      // 不和这个非破坏性动作放在同一个一键位置。
      actions: [
        {
          id: "trash-folder",
          label: "移到废纸篓",
          tone: "danger" as const,
          onSelect: (path: string) => {
            requestProjectFolderTrash(projectId, path);
          },
        },
      ],
    }));
  }

  function handleNewProjectSession(projectId: string): void {
    handleSelectProject(projectId);
    setExpandedProjectIds((current) =>
      current.includes(projectId) ? current : [...current, projectId],
    );
    resetNewTaskDraft();
    requestComposerFocus();
  }

  async function handleCancel(): Promise<void> {
    if (!runtimeState.runId || !isBusy) {
      return;
    }

    dispatch({ type: "run/cancelRequested" });

    try {
      const run = await runtimeClient.cancelRun(runtimeState.runId);
      dispatch({ type: "run/cancelSync", run });
      const terminalStatuses: RuntimeRunStatus[] = ["finished", "error", "cancelled"];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to cancel run";
      if (message.includes("RUN_NOT_FOUND")) {
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

  async function handleAnswerUserQuestion(answer: string): Promise<void> {
    if (!runtimeState.runId || !activeUserQuestion) {
      return;
    }
    if (answeringUserQuestionId === activeUserQuestion.id) {
      return;
    }

    const trimmed = answer.trim();
    if (!trimmed) {
      return;
    }

    setNotice(null);
    setAnsweringUserQuestionId(activeUserQuestion.id);
    try {
      await runtimeClient.answerUserQuestion(runtimeState.runId, activeUserQuestion.id, trimmed);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to answer user question");
    } finally {
      setAnsweringUserQuestionId(null);
    }
  }

  function resetNewTaskDraft(): void {
    setActiveNavId(undefined);
    clearThreadComposerDraft(composerDraftOwnerKey());
    resetActiveThreadContext();
  }

  function handleSelectConversationScope(): void {
    exitConversationBatchMode();
    if (runtimeState.activeThreadId && activeThreadProjectId !== undefined) {
      resetActiveThreadContext();
    }
    setConversationScope(true);
    setSelectedProjectId(undefined);
    workspaceTargetPathRef.current = "";
    setWorkspacePath("");
    setConversationSectionExpanded(true);
  }

  function handleNewTask(): void {
    exitConversationBatchMode();
    setConversationScope(true);
    setSelectedProjectId(undefined);
    workspaceTargetPathRef.current = "";
    setWorkspacePath("");
    setConversationSectionExpanded(true);
    resetNewTaskDraft();
  }

  function handleSelectNav(id: string): void {
    exitConversationBatchMode();
    setActiveNavId(id);
  }

  function handleToggleInspectorCollapsed(): void {
    if (fileWorkspaceExpanded) {
      setFileWorkspaceExpanded(false);
      setExpandedWorkspaceSurface("file");
      setExpandedFileComposerIdleMode("full");
    }
    setInspectorCollapsed((collapsed) => {
      const nextCollapsed = !collapsed;
      if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
        inspectorCollapsedBeforeResponsiveRef.current = nextCollapsed;
      }
      return nextCollapsed;
    });
  }

  function handleOpenFileWorkspace(mode: ProjectFileWorkspaceMode = "files"): void {
    setSelectedSubagentTaskId(undefined);
    setOpenSubagentTaskId(undefined);
    runPlanEntry.closePanel(false);
    setFileWorkspaceOpen(true);
    setExpandedWorkspaceSurface("file");
    setFileWorkspaceMode(mode);
    setFileWorkspaceInspectorWidth(undefined);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  function handleExpandFileWorkspace(): void {
    setFileWorkspaceExpanded(true);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setContextUsagePanelOpen(false);
    runPlanEntry.closePanel(false);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  function handleContractFileWorkspace(): void {
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setExpandedFileComposerTrackedRun(undefined);
  }

  function handleShowConversationSurface(): void {
    setExpandedWorkspaceSurface("conversation");
  }

  function handleShowFileWorkspaceSurface(): void {
    if (fileWorkspaceExpanded) {
      setContextUsagePanelOpen(false);
      runPlanEntry.closePanel(false);
      setExpandedWorkspaceSurface("file");
      setExpandedFileComposerIdleMode("full");
    }
  }

  function handleShowExpandedFileComposerFull(): void {
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("full");
    requestComposerFocus();
  }

  function handleExpandedFilePreviewPrimaryClick(): void {
    if (
      !shouldCollapseExpandedFileComposer({
        active: fileWorkspaceSurfaceActive,
        busy: isBusy,
        forceFull: expandedFileComposerForceFull,
        hasDraft: expandedFileComposerHasDraft,
      })
    ) {
      return;
    }
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("rail");
  }

  function handleCloseFileWorkspace(): void {
    setFileWorkspaceOpen(false);
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setExpandedFileComposerTrackedRun(undefined);
    dispatchFileWorkspaceTabs({ type: "reset" });
    setDeliverablePreview(undefined);
    setFileWorkspaceInspectorWidth(undefined);
  }

  function handleOpenProjectFileTab(tab: ProjectFileWorkspaceTab): void {
    setDeliverablePreview(undefined);
    dispatchFileWorkspaceTabs({ type: "open", tab });
    setFileWorkspaceMode(tab.mode);
    handleShowFileWorkspaceSurface();
  }

  function handleActivateProjectFileTab(tab: ProjectFileWorkspaceTab): void {
    setDeliverablePreview(undefined);
    dispatchFileWorkspaceTabs({ type: "activate", tabId: tab.id });
    setFileWorkspaceMode(tab.mode);
    handleShowFileWorkspaceSurface();
  }

  function handleCloseProjectFileTab(tabId: string): void {
    const nextState = projectFileTabsReducer(fileWorkspaceTabsState, {
      type: "close",
      tabId,
    });
    dispatchFileWorkspaceTabs({ type: "close", tabId });
    setFileWorkspaceMode(activeProjectFileTab(nextState)?.mode ?? "files");
  }

  function handleOpenDeliverablePreview(
    file: {
      artifactId: string;
      runId: string;
      title: string;
    },
    threadId: string,
  ): void {
    setDeliverablePreview({
      threadId,
      runId: file.runId,
      artifactId: file.artifactId,
      title: file.title,
    });
    setPinnedMedia(null);
    handleOpenFileWorkspace("files");
  }

  function handleOpenSubagent(taskId?: string): void {
    setContextUsagePanelOpen(false);
    runPlanEntry.closePanel(false);
    setOpenSubagentTaskId(taskId ?? openSubagentTaskId ?? subagentTasks[0]?.taskId);
  }

  function handleToggleSubagentInspector(): void {
    if (fileWorkspaceOpen) {
      handleCloseFileWorkspace();
    }
    setPinnedMedia(null);
    setSelectedSubagentTaskId((currentTaskId) =>
      nextSubagentInspectorTaskId(
        currentTaskId,
        selectedSubagent?.taskId ?? subagentTasks[0]?.taskId,
      ),
    );
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  async function handleCancelSubagent(taskId: string): Promise<void> {
    if (!runtimeState.runId || !runtimeClient.cancelSubagent) return;
    setCancellingSubagentTaskId(taskId);
    setNotice(null);
    try {
      await runtimeClient.cancelSubagent(runtimeState.runId, taskId, "desktop_user");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "取消子任务失败");
    } finally {
      setCancellingSubagentTaskId(undefined);
    }
  }

  async function handleAddContext(): Promise<void> {
    if (composerBarState.disabled) {
      return;
    }
    setNotice(null);
    try {
      const result = await runtimeClient.pickAttachments(
        workspaceRootForRun || defaultWorkspacePath,
      );
      if (result.canceled) {
        if (result.error) {
          setNotice(result.error);
        }
        return;
      }
      const nextAttachments = result.files.map(composerDraftAttachmentFromPickedFile);
      setComposerAttachments((current) => [...current, ...nextAttachments]);
      if (result.errors?.length) {
        setNotice(result.errors.slice(0, 3).join("；"));
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "添加附件失败");
    }
  }

  async function handlePasteImages(files: File[]): Promise<void> {
    if (files.length === 0 || composerBarState.disabled) {
      return;
    }
    setNotice(null);
    const unsupportedFile = files.find(
      (file) =>
        !SUPPORTED_COMPOSER_IMAGE_MIME_TYPES.includes(
          file.type as (typeof SUPPORTED_COMPOSER_IMAGE_MIME_TYPES)[number],
        ),
    );
    if (unsupportedFile) {
      setNotice("仅支持粘贴 PNG、JPEG 或 WebP 图片");
      return;
    }
    const oversizedFile = files.find((file) => file.size > MAX_COMPOSER_IMAGE_BYTES);
    if (oversizedFile) {
      setNotice(`单张图片不能超过 ${MAX_COMPOSER_IMAGE_BYTES / (1024 * 1024)} MiB`);
      return;
    }

    try {
      const startIndex = composerAttachments.length;
      const nextAttachments = await Promise.all(
        files.map((file, index) => readComposerDraftImageAttachment(file, startIndex + index)),
      );
      setComposerAttachments((current) => [...current, ...nextAttachments]);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "粘贴图片失败");
    }
  }

  function handleRemoveComposerAttachment(attachmentId: string): void {
    setComposerAttachments((current) =>
      current.filter((attachment) => attachment.id !== attachmentId),
    );
    setNotice(null);
  }

  function handleSidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = sidebarWidth;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth + moveEvent.clientX - startX,
        MIN_SIDEBAR_WIDTH,
        MAX_SIDEBAR_WIDTH,
      );
      setSidebarWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove("is-layout-resizing", "is-sidebar-resizing");
    }

    document.body.classList.add("is-layout-resizing", "is-sidebar-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleInspectorResizeStart(event: ReactPointerEvent<HTMLDivElement>): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = effectiveInspectorWidth || inspectorWidth;
    const resizeMax = fileWorkspaceOpen ? FILE_WORKSPACE_INSPECTOR_CAP : MAX_INSPECTOR_WIDTH;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth - (moveEvent.clientX - startX),
        MIN_INSPECTOR_WIDTH,
        resizeMax,
      );
      if (fileWorkspaceOpen) {
        setFileWorkspaceInspectorWidth(nextWidth);
        return;
      }
      setInspectorWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove("is-layout-resizing", "is-inspector-resizing");
    }

    document.body.classList.add("is-layout-resizing", "is-inspector-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleSidebarResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? LAYOUT_STEP : -LAYOUT_STEP;
    setSidebarWidth((width) => clampNumber(width + delta, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH));
  }

  function handleInspectorResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowLeft" ? LAYOUT_STEP : -LAYOUT_STEP;
    const resizeMax = fileWorkspaceOpen ? FILE_WORKSPACE_INSPECTOR_CAP : MAX_INSPECTOR_WIDTH;
    if (fileWorkspaceOpen) {
      setFileWorkspaceInspectorWidth((width) =>
        clampNumber((width ?? FILE_WORKSPACE_INSPECTOR_CAP) + delta, MIN_INSPECTOR_WIDTH, resizeMax),
      );
      return;
    }
    setInspectorWidth((width) => clampNumber(width + delta, MIN_INSPECTOR_WIDTH, resizeMax));
  }

  function openMediaPreview(items: MediaArtifactPreviewItem[], activeId: string): void {
    if (items.length === 0) return;
    setMediaPreview({ items, activeId });
    setContextUsagePanelOpen(false);
    setOpenSubagentTaskId(undefined);
    runPlanEntry.closePanel(false);
  }

  function pinMediaInInspector(item: MediaArtifactPreviewItem): void {
    if (fileWorkspaceOpen) {
      handleCloseFileWorkspace();
    }
    setSelectedSubagentTaskId(undefined);
    setPinnedMedia(item);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  function recordArtifactMediaMetadata(
    sourceKey: string,
    metadata: AwakenMediaMetadata,
  ): void {
    setArtifactMediaMetadata((current) => {
      const previous = current[sourceKey];
      if (
        previous?.width === metadata.width &&
        previous.height === metadata.height &&
        previous.durationSeconds === metadata.durationSeconds
      ) {
        return current;
      }
      return { ...current, [sourceKey]: metadata };
    });
  }

  function refreshArtifactMediaSource(key: string, request: ArtifactOpenRequest): void {
    const retries = artifactMediaSourceRetryRef.current.get(key) ?? 0;
    if (retries >= MAX_ARTIFACT_MEDIA_SOURCE_RETRIES) {
      artifactMediaSourcesRef.current = {
        ...artifactMediaSourcesRef.current,
        [key]: null,
      };
      setArtifactMediaSources((current) => ({ ...current, [key]: null }));
      return;
    }
    artifactMediaSourceRetryRef.current.set(key, retries + 1);
    artifactMediaSourcePendingRef.current.delete(key);
    const nextSources = { ...artifactMediaSourcesRef.current };
    delete nextSources[key];
    artifactMediaSourcesRef.current = nextSources;
    setArtifactMediaSources(nextSources);
    setArtifactMediaHydrationEpoch((value) => value + 1);
  }

  async function openArtifactExternally(request: ArtifactOpenRequest): Promise<void> {
    setOpeningArtifactId(request.artifactId);
    try {
      const result = await runtimeClient.openArtifact(request);
      if (!result.ok) setNotice(result.message);
    } finally {
      setOpeningArtifactId((current) =>
        current === request.artifactId ? null : current,
      );
    }
  }

  async function revealArtifact(request: ArtifactOpenRequest): Promise<void> {
    const result = await runtimeClient.revealArtifact(request);
    if (!result.ok) setNotice(result.message);
  }

  function updateToolTraceOpen(id: string, open: boolean): void {
    setToolTraceOpenById((current) => ({ ...current, [id]: open }));
  }

  function copyToolDetail(copyText: string): void {
    void navigator.clipboard.writeText(copyText).catch(() => {
      setNotice("复制失败，请手动选择详情内容。");
    });
  }

  function toolTraceActionFor({
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
  }): ToolTraceAction {
    const toolId = toolRecordIdForNarrativeItem({ id, toolCallId });
    const presentation = toolTracePresentation(toolResultsById.get(toolId), detail, {
      title,
      patch: patchPreviewsById.get(toolId),
      allowLegacyFallbackOutput: status !== "active",
      status,
    });
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
      open: toolTraceOpenById[id],
      onOpenChange: (open) => updateToolTraceOpen(id, open),
      onCopyDetail: copyToolDetail,
    };
  }

  function skillTraceAction(item: Extract<RunNarrativeItem, { kind: "skill" }>): ToolTraceAction {
    const sections: ToolTraceDetailCard["sections"] = [];
    if (item.detail?.trim()) {
      sections.push({ label: "名称、来源与版本", content: item.detail.trim(), format: "text" });
    }
    if (item.output?.trim()) {
      sections.push({ label: "结果", content: item.output.trim(), format: "text" });
    }
    const detailCard = detailCardFromSections("Skill 详情", sections);
    return {
      id: item.id,
      title: item.title.replace(/\s*\{\s*$/, "").trim(),
      status: item.status,
      kind: "skill",
      detailCard,
      output: item.output,
      outputLabel: "Skill 输出",
      open: toolTraceOpenById[item.id],
      onOpenChange: (open) => updateToolTraceOpen(item.id, open),
      onCopyDetail: copyToolDetail,
    };
  }

  function hookTraceAction(item: Extract<RunNarrativeItem, { kind: "hook" }>): ToolTraceAction {
    const sections: ToolTraceDetailCard["sections"] = [];
    if (item.detail?.trim()) sections.push({ label: "事件与来源", content: item.detail.trim(), format: "text" });
    if (item.output?.trim()) sections.push({ label: "安全回执", content: item.output.trim(), format: "text" });
    return {
      id: item.id, title: item.title, status: item.status, kind: "terminal",
      detailCard: detailCardFromSections("Hook 详情", sections),
      output: item.output, outputLabel: "Hook 回执",
      open: toolTraceOpenById[item.id],
      onOpenChange: (open) => updateToolTraceOpen(item.id, open),
      onCopyDetail: copyToolDetail,
    };
  }

  function actionsForActivityBatch(
    batch: NarrativeActivityBatch,
    animateToolEntry: boolean,
  ): ToolTraceAction[] {
    return batch.items.flatMap((item) => {
      if (item.kind === "exploring") {
        return item.actions.map((action) => ({
          ...toolTraceActionFor(action),
          animateEntry:
            animateToolEntry && !replayedNarrativeItemIds.has(action.id),
        }));
      }
      const animateEntry =
        animateToolEntry && !replayedNarrativeItemIds.has(item.id);
      if (item.kind === "skill") {
        return [{ ...skillTraceAction(item), animateEntry }];
      }
      if (item.kind === "hook") {
        return [{ ...hookTraceAction(item), animateEntry }];
      }
      return [{ ...toolTraceActionFor(item), animateEntry }];
    });
  }

  function renderNarrativeActivityItems(
    turnId: string,
    items: RunNarrativeItem[],
    animateToolEntry = true,
  ) {
    return projectNarrativeActivitySegments({ turnId, items }).map((segment) => {
      if (segment.kind === "item") {
        return renderNarrativeItem(segment.item, segment.id, animateToolEntry);
      }
      const actions = actionsForActivityBatch(segment, animateToolEntry);
      const searchOnly = segment.items.every(
        (item) =>
          item.kind === "exploring" &&
          item.actions.length > 0 &&
          item.actions.every((action) => action.visualKind === "search"),
      );
      const summary = activityBatchSummary(
        actions.map((action) => ({
          kind: action.kind || "tool",
          title: action.title,
          status: action.status,
          fileCount: patchPreviewsById.get(toolRecordIdForNarrativeItem(action))?.affectedPaths.length,
        })),
      );
      return (
        <ToolTraceGroup
          key={segment.id}
          actions={actions}
          label={summary.label}
          detail={summary.detail}
          icon={
            searchOnly ? (
              <AwakenIcon concept="search" size={14} />
            ) : undefined
          }
          open={activityBatchOpenById[segment.id]}
          onOpenChange={(open) => {
            setActivityBatchOpenById((current) => ({ ...current, [segment.id]: open }));
          }}
          animateItems={animateToolEntry}
        />
      );
    });
  }

  function renderNarrativeItem(
    item: RunNarrativeItem,
    key: string,
    animateToolEntry = true,
  ) {
    if (item.kind === "planning" || item.kind === "thinking") {
      const thoughtTrace = (
        <ThoughtTrace
          text={item.kind === "planning" ? "" : item.text}
          status={item.kind === "planning" ? "active" : item.status}
          animateEntry={
            animateToolEntry && !replayedNarrativeItemIds.has(item.id)
          }
          label={
            item.kind === "planning" && item.phase
              ? item.text
              : undefined
          }
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
        <div key={key} className="assistant-message-wrapper">
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
    if (item.kind === "runPlan") {
      return (
        <div key={key} className="narrative-run-plan">
          <RunPlanHistorical
            items={item.items}
            completedCount={item.completedCount}
            totalCount={item.totalCount}
          />
        </div>
      );
    }
    if (item.kind === "compactBoundary") {
      return null;
    }
    if (item.kind === "exploring") {
      const actions = item.actions.map((action) => ({
        ...toolTraceActionFor(action),
        animateEntry:
          animateToolEntry && !replayedNarrativeItemIds.has(action.id),
      }));
      if (actions.length === 1) {
        return <ToolTraceLine key={key} {...actions[0]} />;
      }
      return (
        <ToolTraceGroup
          key={key}
          actions={actions}
          animateItems={animateToolEntry}
        />
      );
    }
    if (item.kind === "tool") {
      const action = toolTraceActionFor(item);
      return (
        <ToolTraceLine
          key={key}
          {...action}
          animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)}
        />
      );
    }
    if (item.kind === "skill") {
      const action = skillTraceAction(item);
      return (
        <ToolTraceLine
          key={key}
          {...action}
          animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)}
        />
      );
    }
    if (item.kind === "hook") {
      const action = hookTraceAction(item);
      return <ToolTraceLine key={key} {...action} animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)} />;
    }
    if (item.kind === "mediaGeneration") {
      const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
      const artifactsByPath = new Map(
        runtimeState.artifacts
          .filter((candidate) => Boolean(candidate.path))
          .map((candidate) => [candidate.path!.replace(/\\/g, "/").toLowerCase(), candidate]),
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
            ? mediaArtifactRequestForPath(threadId, item.runId, artifactPath, artifact)
            : undefined;
        const sourceKey = request ? artifactMediaSourceKey(request) : undefined;
        const sourceState = sourceKey ? artifactMediaSources[sourceKey] : undefined;
        const sourceFailed = sourceState === null;
        return {
          id: artifact?.id || request?.artifactId || `media-${resultItem.index}`,
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
      const previewItems: MediaArtifactPreviewItem[] = mediaEntries.flatMap((entry) =>
        entry.src && entry.request && entry.sourceKey
          ? [{
              id: entry.id,
              kind: item.mediaKind,
              title: entry.title,
              alt: entry.title,
              formatLabel: entry.formatLabel,
              sourceKey: entry.sourceKey,
              request: entry.request,
            }]
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
          detail:
            entry.sourceUnavailable
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
              ? () => refreshArtifactMediaSource(sourceKey, entry.request!)
              : undefined,
          onMetadata:
            sourceKey
              ? (metadata: AwakenMediaMetadata) =>
                  recordArtifactMediaMetadata(sourceKey, metadata)
              : undefined,
          onViewInInspector:
            entry.src && entry.request && sourceKey
              ? () =>
                  pinMediaInInspector({
                    id: entry.id,
                    kind: item.mediaKind,
                    title: entry.title,
                    alt: entry.title,
                    formatLabel: entry.formatLabel,
                    sourceKey,
                    request: entry.request!,
                  })
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
        />
      );
    }
    if (item.kind === "scheduledNotice") {
      return (
        <div key={key} className="scheduled-notice-card" role="note" aria-label="定时提醒">
          <span className="scheduled-notice-card__icon" aria-hidden="true">
            <AwakenIcon concept="scheduled-task" size={14} />
          </span>
          <span className="scheduled-notice-card__body">
            <span className="scheduled-notice-card__header">
              <span className="scheduled-notice-card__label">定时提醒</span>
              {item.taskName ? (
                <span className="scheduled-notice-card__title">{item.taskName}</span>
              ) : null}
            </span>
            <span className="scheduled-notice-card__text">{item.noticeContent}</span>
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
          animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)}
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
          animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)}
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
          animateEntry={animateToolEntry && !replayedNarrativeItemIds.has(item.id)}
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
            const isMedia = file.fileType === "image" || file.fileType === "video";
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
                isMedia && typeof mediaSourceState === "string" && mediaRequest && mediaSourceKey
                  ? () =>
                      pinMediaInInspector({
                        id: file.artifactId,
                        kind: file.fileType as "image" | "video",
                        title: file.title,
                        alt: file.title,
                        formatLabel: file.extension,
                        sourceKey: mediaSourceKey,
                        request: mediaRequest,
                      })
                  : undefined,
              onMediaError:
                mediaRequest && mediaSourceKey
                  ? () => refreshArtifactMediaSource(mediaSourceKey, mediaRequest)
                  : undefined,
              onMediaMetadata:
                mediaSourceKey
                  ? (metadata: AwakenMediaMetadata) =>
                      recordArtifactMediaMetadata(mediaSourceKey, metadata)
                  : undefined,
              opening: openingArtifactId === file.artifactId,
              disabled: !threadId,
              onOpen: threadId
                ? isMedia
                  ? () => openArtifactExternally({
                      threadId,
                      runId: file.runId,
                      artifactId: file.artifactId,
                    })
                  : () => handleOpenDeliverablePreview(file, threadId)
                : undefined,
              onOpenExternal: threadId && !isMedia
                ? () => openArtifactExternally({
                    threadId,
                    runId: file.runId,
                    artifactId: file.artifactId,
                  })
                : undefined,
              onReveal: threadId
                ? () => revealArtifact({
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
  ) {
    if (segment.kind === "items") {
      return renderNarrativeActivityItems(turnId, segment.items, true);
    }

    const hasElapsedTime =
      segment.elapsedMs !== undefined ||
      (timing.startedAtMs !== undefined &&
        (timing.running || timing.completedAtMs !== undefined));

    return [
      <div
        key={`${turnId}:${segment.id}`}
        className={`execution-process-disclosure${
          segment.open ? " execution-process-disclosure-open" : ""
        }`}
      >
        <Disclosure
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
          open={segment.open}
          onOpenChange={(open) => {
            setExecutionProcessOpenByTurnId((current) => ({
              ...current,
              [turnId]: open,
            }));
          }}
        >
          <div className="execution-process-disclosure-body">
            {renderNarrativeActivityItems(turnId, segment.items, false)}
          </div>
        </Disclosure>
      </div>,
    ];
  }

  function acpRunnerLabel(runnerId: string): string {
    return runnerId
      .split("-")
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ");
  }

  function acpDisplayStatusToToolStatus(status: ACPDisplayStatus): ToolDisplayState {
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

  const platform = runtimeClient.platform as PlatformKind;
  const mediaRevealLabel = mediaRevealLabelForPlatform(platform);
  const activeMediaPreviewIndex = mediaPreview
    ? Math.max(0, mediaPreview.items.findIndex((item) => item.id === mediaPreview.activeId))
    : -1;
  const activeMediaPreviewRecord =
    mediaPreview && activeMediaPreviewIndex >= 0
      ? mediaPreview.items[activeMediaPreviewIndex]
      : undefined;
  const activeMediaPreviewSourceState = activeMediaPreviewRecord
    ? artifactMediaSources[activeMediaPreviewRecord.sourceKey]
    : undefined;
  const activeMediaPreviewItem = activeMediaPreviewRecord
    ? {
        ...activeMediaPreviewRecord,
        src:
          typeof activeMediaPreviewSourceState === "string"
            ? activeMediaPreviewSourceState
            : undefined,
        metadata: artifactMediaMetadata[activeMediaPreviewRecord.sourceKey],
      }
    : undefined;
  const pinnedMediaSourceState = pinnedMedia
    ? artifactMediaSources[pinnedMedia.sourceKey]
    : undefined;
  const pinnedMediaItem = pinnedMedia
    ? {
        ...pinnedMedia,
        src:
          typeof pinnedMediaSourceState === "string"
            ? pinnedMediaSourceState
            : undefined,
        metadata: artifactMediaMetadata[pinnedMedia.sourceKey],
      }
    : undefined;
  const composerBarAttachments = composerAttachmentViews(composerAttachments);
  const fileWorkspaceProjectId = workspaceLabelProjectId
    ? backendProjectIdFor(workspaceLabelProjectId)
    : undefined;
  const fileWorkspaceProjectLabel = workspaceLabelProjectId
    ? projectLabelById.get(workspaceLabelProjectId)
    : undefined;
  const fileWorkspaceFolders = workspaceLabelProjectId
    ? foldersForProjectId(workspaceLabelProjectId).map((folder) => ({
        path: folder.path,
        label: workspaceLabelForPath(folder.path),
        isPrimary: folder.isPrimary,
      }))
    : [];
  const activeFileWorkspaceTabCandidate = activeProjectFileTab(fileWorkspaceTabsState);
  const activeFileWorkspaceTab =
    activeFileWorkspaceTabCandidate?.projectId === fileWorkspaceProjectId
      ? activeFileWorkspaceTabCandidate
      : undefined;
  const visibleFileWorkspaceTabs = fileWorkspaceTabsState.tabs.filter(
    (tab) => tab.projectId === fileWorkspaceProjectId,
  );
  const fileWorkspaceTabsAreActive =
    !fileWorkspaceExpandedActive || expandedWorkspaceSurface === "file";
  const currentConversationLabel = activeThread?.label?.trim() || "新建任务";
  const layoutStyle = {
    "--aw-sidebar-width": `${sidebarWidth}px`,
    "--aw-inspector-width": `${effectiveInspectorWidth}px`,
  } as CSSProperties;

  function renderExternalActionGate() {
    if (activeExternalInteraction) {
      const resolving = resolvingExternalInteractionId === activeExternalInteraction.id;
      const isACPPermission = activeExternalInteraction.kind === "acp-permission";
      return (
        <ExternalActionGate
          compact
          interaction={activeExternalInteraction}
          resolving={resolving}
          onConfirmDone={() => {
            void resolveActiveExternalInteraction("completed");
          }}
          onCancel={() => {
            if (isACPPermission) {
              void resolveActiveACPPermission("", true);
            } else {
              void resolveActiveExternalInteraction("cancelled");
            }
          }}
          onResolveACPOption={(optionId) => {
            void resolveActiveACPPermission(optionId);
          }}
        />
      );
    }
    return null;
  }

  function renderUserQuestionPrompt() {
    if (!activeUserQuestion) {
      return null;
    }
    const answering = answeringUserQuestionId === activeUserQuestion.id;
    return (
      <UserQuestionPrompt
        question={activeUserQuestion}
        disabled={answering}
        onCancel={() => {
          void handleCancel();
        }}
        onSubmit={(answer) => {
          void handleAnswerUserQuestion(answer);
        }}
      />
    );
  }

  function renderComputerReviewGate() {
    if (!activeComputerReview) {
      return null;
    }
    const resolving = resolvingComputerReviewId === activeComputerReview.id;
    return (
      <ComputerReviewGate
        review={activeComputerReview}
        resolving={resolving}
        onResolve={(decision) => {
          void resolveActiveComputerReview(decision);
        }}
      />
    );
  }

  function renderComputerHandoffIndicator() {
    if (!activeComputerHandoff) {
      return null;
    }
    const busy = controllingComputerHandoffId === activeComputerHandoff.id;
    return (
      <ComputerHandoffIndicator
        handoff={activeComputerHandoff}
        busy={busy}
        onReturn={() => {
          void returnComputerControl();
        }}
      />
    );
  }

  function renderComposerRuntimePanels() {
    if (composerRuntimePanels.length === 0) {
      return null;
    }

    return (
      <div className="composer-runtime-panels">
        {composerRuntimePanels.map((panel) => (
          <div
            key={panel.id}
            className={`runtime-panel composer-runtime-panel${
              panel.tone === "error" ? " runtime-panel-error" : ""
            }`}
            role={panel.tone === "error" ? "alert" : "status"}
          >
            <strong>{panel.title}</strong>
            <p>{panel.message}</p>
          </div>
        ))}
      </div>
    );
  }

  function renderNoticeToast() {
    if (!notice) {
      return null;
    }

    return (
      <div className="desktop-toast" role="status" aria-live="polite">
        {notice}
      </div>
    );
  }

  const projectDeleteDialogTitle =
    pendingProjectDelete?.action === "detach"
      ? "从 Awaken 移除项目"
      : pendingProjectDelete?.action === "delete_history"
        ? "删除 Awaken 历史"
        : "将文件夹移到废纸篓";
  const projectDeleteDialogBody = !pendingProjectDelete
    ? ""
    : pendingProjectDelete.action === "detach"
      ? `“${pendingProjectDelete.label}”会从侧栏移除。${pendingProjectDelete.threadCount} 条历史会话和磁盘文件都会保留。`
      : pendingProjectDelete.action === "delete_history"
        ? `将删除“${pendingProjectDelete.label}”的 ${pendingProjectDelete.threadCount} 条 Awaken 历史会话。项目和磁盘目录都会保留；运行中的任务会先自动停止。`
        : [
            `“${pendingProjectDelete.label}”的磁盘目录将进入系统废纸篓，并从项目中解绑。项目和它的 Awaken 历史都会保留；失败时文件夹、项目和文件都保持原样。`,
            sharedFolderTrashWarning({
              folderPath: pendingProjectDelete.folderPath,
              actingProjectId: pendingProjectDelete.projectId,
              projects: [
                ...projectsFromWorkspace,
                {
                  id: DEFAULT_WORKSPACE_PROJECT_ID,
                  label: projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
                  folders: defaultProjectFolders,
                },
              ],
            }),
          ]
            .filter(Boolean)
            .join(" ");
  const projectDeleteConfirmLabel =
    pendingProjectDelete?.action === "detach"
      ? "确认移除"
      : pendingProjectDelete?.action === "delete_history"
        ? "确认删除历史"
        : "移到废纸篓";
  const pendingThreadDeleteThreads = pendingThreadDelete
    ? pendingThreadDelete.threadIds.flatMap((threadId) => {
        const thread = runtimeState.threads.find((item) => item.threadId === threadId);
        return thread ? [thread] : [];
      })
    : [];
  const pendingThreadDeleteCount = pendingThreadDeleteThreads.length;
  const threadDeleteDialogTitle = pendingThreadDelete?.source === "batch"
    ? `删除 ${pendingThreadDeleteCount} 个对话？`
    : pendingThreadDeleteThreads[0]
      ? `删除会话“${pendingThreadDeleteThreads[0].label}”？`
      : "删除会话？";
  const threadDeleteDialogCopy = pendingThreadDelete?.source === "batch"
    ? "将删除所选对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。"
    : "将删除这条对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。";

  return (
    <>
      <div
        className={`desktop-root desktop-root-electron ${inspectorPresentation.rootClassName}`}
        style={layoutStyle}
        aria-hidden={modelSettingsOpen || undefined}
        inert={modelSettingsOpen || undefined}
      >
        <AppShell
          platform={platform}
          windowChromeGeometry={platform === "macos" ? MACOS_WINDOW_CHROME_GEOMETRY : undefined}
          sidebarCollapsed={sidebarCollapsed}
          sidebarResizeHandleProps={{
            role: "separator",
            "aria-label": "Resize sidebar",
            "aria-orientation": "vertical",
            "aria-valuemin": MIN_SIDEBAR_WIDTH,
            "aria-valuemax": MAX_SIDEBAR_WIDTH,
            "aria-valuenow": sidebarWidth,
            tabIndex: 0,
            onPointerDown: handleSidebarResizeStart,
            onKeyDown: handleSidebarResizeKeyDown,
          }}
          inspectorCollapsed={inspectorPresentation.effectiveCollapsed}
          inspectorResizeHandleProps={
            shouldEnableInspectorResizeHandle({
              inspectorAvailable: inspectorPresentation.available,
              fileWorkspaceExpanded: fileWorkspaceExpandedActive,
            })
              ? {
                  role: "separator",
                  "aria-label": "Resize inspector",
                  "aria-orientation": "vertical",
                  "aria-valuemin": MIN_INSPECTOR_WIDTH,
                  "aria-valuemax": inspectorCap,
                  "aria-valuenow": effectiveInspectorWidth,
                  tabIndex: 0,
                  onPointerDown: handleInspectorResizeStart,
                  onKeyDown: handleInspectorResizeKeyDown,
                }
              : undefined
          }
          chrome={
            <WorkbenchChrome
              platform={platform}
              sidebarCollapsed={sidebarCollapsed}
              onToggleSidebar={() => setSidebarCollapsed((collapsed) => !collapsed)}
              onNewTask={handleNewTask}
              center={
                fileWorkspaceExpandedActive && inspectorPresentation.available ? (
                  <div className="workbench-conversation-tabs" role="tablist" aria-label="当前会话">
                    <button
                      type="button"
                      className={`workbench-conversation-tab${
                        expandedWorkspaceSurface === "conversation" ? " is-active" : ""
                      }`}
                      role="tab"
                      aria-selected={expandedWorkspaceSurface === "conversation"}
                      title={currentConversationLabel}
                      onClick={handleShowConversationSurface}
                    >
                      <AwakenIcon concept="memory-feedback" size={16} />
                      <span>{currentConversationLabel}</span>
                    </button>
                  </div>
                ) : undefined
              }
              right={
                fileWorkspaceOpen && inspectorPresentation.available ? (
                  <div className="workbench-chrome-file-mode">
                    <div className="project-files-tabs" role="tablist" aria-label="打开的文件">
                      {deliverablePreview ? (() => {
                        const iconKind = projectFileIconKind({
                          kind: "file",
                          name: deliverablePreview.title,
                        });
                        return (
                          <div
                            className={`project-files-tab${
                              fileWorkspaceTabsAreActive ? " is-active" : ""
                            }`}
                            role="presentation"
                          >
                            <button
                              type="button"
                              className="project-files-tab__activate"
                              role="tab"
                              aria-selected={fileWorkspaceTabsAreActive}
                              title={deliverablePreview.title}
                              onClick={handleShowFileWorkspaceSurface}
                            >
                              <span
                                className="project-files-tab__file-icon"
                                data-file-icon={iconKind}
                              >
                                <ProjectFileIcon kind={iconKind} size={16} />
                              </span>
                              <span className="project-files-tab__label">
                                {deliverablePreview.title}
                              </span>
                            </button>
                            <button
                              type="button"
                              className="project-files-tab__close"
                              aria-label={`关闭 ${deliverablePreview.title}`}
                              title={`关闭 ${deliverablePreview.title}`}
                              onClick={() => setDeliverablePreview(undefined)}
                            >
                              <AwakenIcon concept="close" size={16} />
                            </button>
                          </div>
                        );
                      })() : null}
                      {visibleFileWorkspaceTabs.map((tab) => {
                        const active =
                          !deliverablePreview &&
                          fileWorkspaceTabsAreActive &&
                          tab.id === activeFileWorkspaceTab?.id;
                        const iconKind = projectFileIconKind({
                          kind: "file",
                          name: tab.name,
                        });
                        return (
                          <div
                            key={tab.id}
                            className={`project-files-tab${active ? " is-active" : ""}`}
                            role="presentation"
                          >
                            <button
                              type="button"
                              className="project-files-tab__activate"
                              role="tab"
                              aria-selected={active}
                              title={tab.name}
                              onClick={() => handleActivateProjectFileTab(tab)}
                            >
                              <span
                                className="project-files-tab__file-icon"
                                data-file-icon={iconKind}
                              >
                                <ProjectFileIcon kind={iconKind} size={16} />
                              </span>
                              <span className="project-files-tab__label">{tab.name}</span>
                            </button>
                            <button
                              type="button"
                              className="project-files-tab__close"
                              aria-label={`关闭 ${tab.name}`}
                              title={`关闭 ${tab.name}`}
                              onClick={() => handleCloseProjectFileTab(tab.id)}
                            >
                              <AwakenIcon concept="close" size={16} />
                            </button>
                          </div>
                        );
                      })}
                      {!deliverablePreview && visibleFileWorkspaceTabs.length === 0 ? (
                        <div
                          className={`project-files-tab is-placeholder${
                            fileWorkspaceTabsAreActive ? " is-active" : ""
                          }`}
                          role="presentation"
                        >
                          <button
                            type="button"
                            className="project-files-tab__activate"
                            role="tab"
                            aria-selected={fileWorkspaceTabsAreActive}
                            onClick={handleShowFileWorkspaceSurface}
                          >
                            <AwakenIcon concept="file-generic" size={16} />
                            <span className="project-files-tab__label">
                              {fileWorkspaceMode === "browser" ? "浏览器" : "打开文件"}
                            </span>
                          </button>
                          <button
                            type="button"
                            className="project-files-tab__close"
                            aria-label="关闭文件视图"
                            onClick={handleCloseFileWorkspace}
                          >
                            <AwakenIcon concept="close" size={16} />
                          </button>
                        </div>
                      ) : null}
                    </div>
                    {fileWorkspaceExpandedActive ? (
                      <Tooltip label="收起面板" placement="bottom">
                        <button
                          type="button"
                          className="workbench-chrome-icon workbench-chrome-expand-toggle"
                          aria-label="收起面板"
                          onClick={handleContractFileWorkspace}
                        >
                          <AwakenIcon concept="exit-full" size={14} />
                        </button>
                      </Tooltip>
                    ) : null}
                    <button
                      type="button"
                      className="workbench-chrome-icon workbench-chrome-panel-toggle"
                      aria-label={inspectorCollapsed ? "Expand inspector" : "Collapse inspector"}
                      onClick={handleToggleInspectorCollapsed}
                    >
                      {inspectorCollapsed ? (
                        <AwakenIcon concept="toggle-right-panel" size={14} />
                      ) : (
                        <AwakenIcon concept="toggle-right-panel" size={14} />
                      )}
                    </button>
                  </div>
                ) : (
                  <div className="workbench-chrome-status">
                    {topbarStatusLabel ? (
                      <span className="workbench-chrome-state">{topbarStatusLabel}</span>
                    ) : null}
                    {inspectorPresentation.available ? (
                      <button
                        type="button"
                        className="workbench-chrome-icon workbench-chrome-panel-toggle"
                        aria-label={inspectorCollapsed ? "Expand inspector" : "Collapse inspector"}
                        onClick={handleToggleInspectorCollapsed}
                      >
                        {inspectorCollapsed ? (
                          <AwakenIcon concept="toggle-right-panel" size={14} />
                        ) : (
                          <AwakenIcon concept="toggle-right-panel" size={14} />
                        )}
                      </button>
                    ) : null}
                  </div>
                )
              }
            />
          }
          sidebar={
            <WorkbenchSidebarFrame>
              <Sidebar
                navItems={navItems}
                activeNavId={activeNavId}
                projects={repositoryProjects}
                conversations={conversationItems}
                activeProjectId={activeProjectId}
                expandedIds={expandedProjectIds}
                defaultExpandedIds={[DEFAULT_WORKSPACE_PROJECT_ID]}
                profileName="Nolan"
                profileAvatarFallback="AW"
                newTaskLabel="新建任务"
                projectSectionLabel="项目工作区"
                projectSectionActions={
                  <span className="sidebar-section-actions">
                    <SidebarSectionAction
                      label={allProjectsExpanded ? "收起全部项目" : "展开全部项目"}
                      onClick={() =>
                        setExpandedProjectIds(
                          allProjectsExpanded
                            ? []
                            : repositoryProjects.map((project) => project.id),
                        )
                      }
                    >
                      {allProjectsExpanded ? (
                        <AwakenIcon concept="collapse-all" size={16} />
                      ) : (
                        <AwakenIcon concept="expand-all" size={16} />
                      )}
                    </SidebarSectionAction>
                    <SidebarSectionAction label="创建项目" onClick={handleOpenCreateProject}>
                      <AwakenIcon concept="project-add" size={16} />
                    </SidebarSectionAction>
                  </span>
                }
                conversationSectionLabel={
                  conversationBatchMode ? `对话 · 已选 ${selectedConversationCount}` : "对话"
                }
                conversationSectionExpanded={conversationSectionExpanded}
                conversationSectionActions={
                  <span className="sidebar-section-actions">
                    {conversationBatchMode ? (
                      <>
                        <SidebarSectionAction
                          label={
                            allEligibleConversationsSelected ? "取消全选" : "全选可删除对话"
                          }
                          disabled={eligibleConversationThreadIds.length === 0}
                          onClick={() => {
                            setSelectedConversationThreadIds(
                              allEligibleConversationsSelected ? [] : eligibleConversationThreadIds,
                            );
                          }}
                        >
                          <AwakenIcon concept="select-all" size={16} />
                        </SidebarSectionAction>
                        <SidebarSectionAction
                          label={
                            selectedConversationCount > 0
                              ? `删除 ${selectedConversationCount} 个对话`
                              : "请选择要删除的对话"
                          }
                          className="conversation-batch-delete-action"
                          disabled={selectedConversationCount === 0}
                          onClick={() => {
                            requestThreadDelete(
                              Array.from(selectedConversationThreadIdSet),
                              "batch",
                            );
                          }}
                        >
                          <AwakenIcon concept="delete" size={16} />
                        </SidebarSectionAction>
                        <SidebarSectionAction
                          label="退出批量管理"
                          onClick={exitConversationBatchMode}
                        >
                          <AwakenIcon concept="close" size={16} />
                        </SidebarSectionAction>
                      </>
                    ) : (
                      <>
                        <SidebarSectionAction
                          label={conversationSectionExpanded ? "收起对话" : "展开对话"}
                          onClick={() => setConversationSectionExpanded((expanded) => !expanded)}
                        >
                          {conversationSectionExpanded ? (
                            <AwakenIcon concept="collapse-all" size={16} />
                          ) : (
                            <AwakenIcon concept="expand-all" size={16} />
                          )}
                        </SidebarSectionAction>
                        <SidebarSectionAction
                          label="批量管理对话"
                          disabled={conversationThreads.length === 0}
                          onClick={enterConversationBatchMode}
                        >
                          <AwakenIcon concept="select-mode" size={16} />
                        </SidebarSectionAction>
                        <SidebarSectionAction label="新增对话" onClick={handleNewTask}>
                          <AwakenIcon concept="thread-new" size={16} />
                        </SidebarSectionAction>
                      </>
                    )}
                  </span>
                }
                showProjectCount={false}
                showConversationCount={false}
                onSelectNav={handleSelectNav}
                onSelectProject={handleSelectProject}
                onSelectConversation={handleSelectProject}
                onReorderProjects={(ids) => {
                  void handleReorderProjects(ids);
                }}
                projectReorderDisabled={!desktopReady || projectReorderSubmitting}
                onExpandedChange={setExpandedProjectIds}
                onNewTask={handleNewTask}
                onProfileClick={() => setProfilePopoverOpen(true)}
                profileButtonRef={profileButtonRef}
                profileUpdateAction={sidebarUpdateAction}
              />
            </WorkbenchSidebarFrame>
          }
          main={
            <div className="desktop-main">
              {activeExtensionSection ? (
                <>
                  {renderNoticeToast()}
                  <ExtensionsView
                    section={activeExtensionSection}
                    desktopReady={desktopReady}
                    runtimeClient={runtimeClient}
                    onNotice={setNotice}
                    activeWorkspaceRoot={workspacePath}
                  />
                </>
              ) : scheduledTasksNavActive ? (
                <ScheduledTasksView
                  runtimeClient={runtimeClient}
                  desktopReady={desktopReady}
                  onOpenThread={(threadId) => {
                    backgroundThreadActivationRef.current(threadId);
                  }}
                />
              ) : (
                <WorkbenchShell
                  inspectorCollapsed={inspectorCollapsed}
                  inspectorExpanded={fileWorkspaceExpandedActive}
                  inspectorSurfaceActive={
                    !fileWorkspaceExpandedActive || fileWorkspaceSurfaceActive
                  }
                  stage={
                    <RunStage
                      contentInactive={fileWorkspaceSurfaceActive || Boolean(activeMediaPreviewItem)}
                      scrollRef={showCenterComposer ? undefined : setRunStageScrollElement}
                      onScroll={showCenterComposer ? undefined : handleRunStageScroll}
                      overlay={
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
                          {!activeMediaPreviewItem &&
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
                                  ["queued", "running", "cancelling"].includes(
                                    openSubagent.status,
                                  )
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
                                  <p className="subagent-execution-empty">
                                    等待子任务执行记录…
                                  </p>
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
                                              activeId: current.items[previousIndex]?.id ?? current.activeId,
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
                                              activeId: current.items[nextIndex]?.id ?? current.activeId,
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
                      }
                      composer={
                        showCenterComposer ? undefined : (
                          <ComposerDock
                            ref={setComposerDockElement}
                            interactionInactive={Boolean(activeMediaPreviewItem)}
                          >
                            <div className="composer-dock-stack">
                              {renderComposerRuntimePanels()}
                              {renderUserQuestionPrompt()}
                              {renderExternalActionGate()}
                              {renderComputerReviewGate()}
                              {renderComputerHandoffIndicator()}
                              {runPlanEntry.entryVisible && activeRunPlan ? (
                                <RunPlanTrigger
                                  ref={runPlanEntry.triggerRef}
                                  items={activeRunPlanItems}
                                  completedCount={activeRunPlan.completedCount}
                                  totalCount={activeRunPlan.totalCount}
                                  currentStep={activeRunPlan.currentStep}
                                  allCompleted={activeRunPlan.allCompleted}
                                  open={runPlanEntry.panelOpen}
                                  onToggle={() => {
                                    if (!runPlanEntry.panelOpen) {
                                      setContextUsagePanelOpen(false);
                                      setOpenSubagentTaskId(undefined);
                                    }
                                    runPlanEntry.togglePanel();
                                  }}
                                />
                              ) : null}
                              <ExpandedFileComposer
                                active={fileWorkspaceSurfaceActive}
                                idleMode={expandedFileComposerIdleMode}
                                running={isBusy}
                                completed={expandedFileComposerCompleted}
                                completedElapsedMs={expandedFileComposerElapsedMs}
                                forceFull={expandedFileComposerForceFull}
                                onShowCompact={() => setExpandedFileComposerIdleMode("compact")}
                                onShowFull={handleShowExpandedFileComposerFull}
                                onStop={() => {
                                  void handleComposerSubmit();
                                }}
                              >
                                <ComposerBar
                                  value={composerValue}
                                  onChange={setComposerValue}
                                  onStructuredInputChange={handleComposerStructuredInputChange}
                                  onSubmit={() => {
                                    void handleComposerSubmit();
                                  }}
                                  onAddContext={handleAddContext}
                                  placeholder={
                                    fileWorkspaceSurfaceActive
                                      ? "输入后续任务…"
                                      : "给当前运行追加任务…"
                                  }
                                  disabled={composerBarState.disabled}
                                  isReady={composerBarState.isReady}
                                  submitMode={composerBarState.submitMode}
                                  submitDisabled={composerBarState.submitDisabled}
                                  variant="dock"
                                  metaItems={dockComposerMetaItems}
                                  focusRequest={composerFocusRequest}
                                  focusOffset={composerFocusOffset}
                                  onInputSelectionChange={(selection) => {
                                    setComposerInputSelection({
                                      anchor: selection.anchor,
                                      focus: selection.focus,
                                    });
                                  }}
                                  attachments={composerBarAttachments}
                                  tokens={composerBarTokens}
                                  slashPalette={composerBarSlashPalette}
                                  onInputKeyDown={handleComposerInputKeyDown}
                                  onRemoveAttachment={handleRemoveComposerAttachment}
                                  onRemoveToken={handleRemoveComposerToken}
                                  onPasteImages={(files) => {
                                    void handlePasteImages(files);
                                  }}
                                />
                              </ExpandedFileComposer>
                            </div>
                          </ComposerDock>
                        )
                      }
                    >
                      {showCenterComposer ? (
                        <div className="run-stage-center-composer">
                          <TaskWelcome />
                          {renderComposerRuntimePanels()}
                          {renderUserQuestionPrompt()}
                          {renderExternalActionGate()}
                          {renderComputerReviewGate()}
                          {renderComputerHandoffIndicator()}
                          <ComposerBar
                            value={composerValue}
                            onChange={setComposerValue}
                            onStructuredInputChange={handleComposerStructuredInputChange}
                            onSubmit={() => {
                              void handleComposerSubmit();
                            }}
                            onAddContext={handleAddContext}
                            placeholder="描述任务，或 / 选择技能"
                            disabled={composerBarState.disabled}
                            isReady={composerBarState.isReady}
                            submitMode={composerBarState.submitMode}
                            submitDisabled={composerBarState.submitDisabled}
                            variant="center"
                            metaItems={composerMetaItems}
                            projectPicker={composerProjectPicker}
                            focusRequest={composerFocusRequest}
                            focusOffset={composerFocusOffset}
                            onInputSelectionChange={(selection) => {
                              setComposerInputSelection({
                                anchor: selection.anchor,
                                focus: selection.focus,
                              });
                            }}
                            attachments={composerBarAttachments}
                            tokens={composerBarTokens}
                            slashPalette={composerBarSlashPalette}
                            onInputKeyDown={handleComposerInputKeyDown}
                            onRemoveAttachment={handleRemoveComposerAttachment}
                            onRemoveToken={handleRemoveComposerToken}
                            onPasteImages={(files) => {
                              void handlePasteImages(files);
                            }}
                          />
                        </div>
                      ) : null}

                      {showCenterComposer ? null : (
                        <ExecutionFeed>
                          {narrativeSections.leadItems.map((item, index) =>
                            renderNarrativeItem(item, `lead:${item.id}:${index}`),
                          )}
                          {narrativeSections.turns.map((turn, index) => {
                            const isLatestPrompt = turn.id === latestPromptId;
                            const isActiveTurn = turn.id === activeWorkbenchTurnId;
                            const settledElapsedMs = narrativeTurnElapsedMs(turn.items);
                            const turnBodySegments = projectNarrativeTurnBodySegments({
                              turnId: turn.id,
                              items: turn.items,
                              executionProcessOpen: executionProcessOpenByTurnId[turn.id] === true,
                            });
                            const hasExecutionProcess = turnBodySegments.some(
                              (segment) => segment.kind === "executionProcess",
                            );
                            const turnTiming = {
                              startedAtMs: isLatestPrompt
                                ? runtimeState.runStartedAtMs
                                : undefined,
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

                            return (
                              <section
                                key={`${turn.id}:${index}`}
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
                                      text={promptTextAfterTokenLeadingText(
                                        turn.prompt.text,
                                        turn.prompt.tokens,
                                      )}
                                      tokens={promptTokenViews(turn.prompt.tokens)}
                                      attachments={promptAttachmentViews(turn.prompt.attachments)}
                                      expanded={promptBubbleModeByTurnId[turn.id] === "expanded"}
                                      onExpandedChange={(expanded) => {
                                        setPromptBubbleModeByTurnId((current) => ({
                                          ...current,
                                          [turn.id]: expanded ? "expanded" : "collapsed",
                                        }));
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
                                      renderTurnBodySegment(turn.id, segment, turnTiming),
                                    )}
                                  </div>
                                </div>
                              </section>
                            );
                          })}
                        </ExecutionFeed>
                      )}
                    </RunStage>
                  }
                  inspector={
                    fileWorkspaceOpen ? (
                      <ProjectFilesWorkspace
                        key={fileWorkspaceProjectId ?? "no-project"}
                        runtimeClient={runtimeClient}
                        projectId={fileWorkspaceProjectId}
                        projectLabel={fileWorkspaceProjectLabel}
                        folders={fileWorkspaceFolders}
                        initialMode={fileWorkspaceMode}
                        activeTab={activeFileWorkspaceTab}
                        artifactPreview={deliverablePreview}
                        expanded={fileWorkspaceExpandedActive}
                        onWorkspaceModeChange={(mode) => {
                          setFileWorkspaceMode(mode);
                          if (mode === "browser") setDeliverablePreview(undefined);
                        }}
                        onOpenTab={handleOpenProjectFileTab}
                        onExpand={handleExpandFileWorkspace}
                        onPreviewPrimaryClick={handleExpandedFilePreviewPrimaryClick}
                        addTaskDisabled={isBusy}
                        onAddTaskContext={handleAddProjectEntryToTask}
                      />
                    ) : (
                      <InspectorRail
                        ariaLabel={pinnedMedia ? "媒体" : selectedSubagentTaskId ? "子任务" : "文件"}
                        activeId={
                          pinnedMedia
                            ? "media"
                            : selectedSubagentTaskId
                              ? "subagents"
                              : undefined
                        }
                        items={[
                          ...INSPECTOR_FILE_ITEMS,
                          ...(pinnedMedia
                            ? [
                                {
                                  id: "media",
                                  label: "媒体",
                                  tone: "neutral" as const,
                                  icon: <AwakenIcon concept="media-image" size={16} />,
                                },
                              ]
                            : []),
                          ...(subagentTasks.length
                            ? [
                                {
                                  id: "subagents",
                                  label: "子任务",
                                  badge: runningSubagentCount || subagentTasks.length,
                                  tone: runningSubagentCount
                                    ? ("warning" as const)
                                    : ("neutral" as const),
                                  icon: <AwakenIcon concept="subagent" size={16} />,
                                },
                              ]
                            : []),
                        ]}
                        onSelect={(id) => {
                          if (id === "files") {
                            handleOpenFileWorkspace("files");
                          } else if (id === "browser") {
                            handleOpenFileWorkspace("browser");
                          } else if (id === "subagents") {
                            handleToggleSubagentInspector();
                          } else if (id === "media") {
                            setSelectedSubagentTaskId(undefined);
                            setInspectorCollapsed(false);
                          }
                        }}
                      >
                        {pinnedMedia && pinnedMediaItem ? (
                          <MediaInspector
                            item={pinnedMediaItem}
                            sourceUnavailable={pinnedMediaSourceState === null}
                            openingExternal={
                              openingArtifactId === pinnedMedia.request.artifactId
                            }
                            localOpenLabel="使用本地应用打开"
                            revealLabel={mediaRevealLabel}
                            onClose={() => setPinnedMedia(null)}
                            onMediaError={() =>
                              refreshArtifactMediaSource(
                                pinnedMedia.sourceKey,
                                pinnedMedia.request,
                              )
                            }
                            onMetadata={(metadata) =>
                              recordArtifactMediaMetadata(pinnedMedia.sourceKey, metadata)
                            }
                            onOpenExternal={() => {
                              void openArtifactExternally(pinnedMedia.request);
                            }}
                            onReveal={() => {
                              void revealArtifact(pinnedMedia.request);
                            }}
                          />
                        ) : selectedSubagentTaskId && selectedSubagent ? (
                          <div className="subagent-inspector">
                            <header className="subagent-inspector__header">
                              <div>
                                <span className="subagent-inspector__eyebrow">
                                  {subagentAgentTypeLabel(selectedSubagent.agentType)}
                                  {subagentWorkspaceModeLabel(selectedSubagent.workspaceMode)
                                    ? ` · ${subagentWorkspaceModeLabel(selectedSubagent.workspaceMode)}`
                                    : ""}
                                </span>
                                <h3>{selectedSubagent.description}</h3>
                              </div>
                              <span
                                className={`subagent-inspector__status subagent-inspector__status--${selectedSubagent.status}`}
                              >
                                {awakenSubagentStatusLabels[selectedSubagent.status]}
                              </span>
                            </header>
                            <div className="subagent-inspector__metrics" aria-label="子任务统计">
                              <span>{selectedSubagent.toolUseCount} 次工具</span>
                              {(selectedSubagent.totalTokens ?? selectedSubagent.estimatedTokens) ? (
                                <span>
                                  {new Intl.NumberFormat("zh-CN").format(
                                    selectedSubagent.totalTokens ??
                                      selectedSubagent.estimatedTokens ??
                                      0,
                                  )}{" "}
                                  tokens
                                </span>
                              ) : null}
                              {formatSubagentElapsed(
                                subagentElapsedMs(selectedSubagent, subagentNowMs),
                              ) ? (
                                <span>
                                  {formatSubagentElapsed(
                                    subagentElapsedMs(selectedSubagent, subagentNowMs),
                                  )}
                                </span>
                              ) : null}
                            </div>
                            <div className="subagent-inspector__switcher">
                              {subagentTasks.map((task) => (
                                <button
                                  key={task.taskId}
                                  type="button"
                                  className={
                                    task.taskId === selectedSubagent.taskId
                                      ? "is-active"
                                      : undefined
                                  }
                                  onClick={() => setSelectedSubagentTaskId(task.taskId)}
                                >
                                  {task.description}
                                </button>
                              ))}
                            </div>
                            <section>
                              <h4>最近活动</h4>
                              <p>
                                {selectedSubagent.activity ||
                                  awakenSubagentStatusLabels[selectedSubagent.status]}
                              </p>
                            </section>
                            {selectedSubagent.resultPreview ? (
                              <section>
                                <h4>结果</h4>
                                <AssistantMessage
                                  markdown={selectedSubagent.resultPreview}
                                  status={selectedSubagent.status === "failed" ? "error" : "done"}
                                  showFooter={false}
                                />
                              </section>
                            ) : null}
                            {selectedSubagent.errorCode ? (
                              <section className="subagent-inspector__error">
                                <h4>错误</h4>
                                <p>{selectedSubagent.errorCode}</p>
                                {selectedSubagent.errorMessage ? (
                                  <p>{selectedSubagent.errorMessage}</p>
                                ) : null}
                              </section>
                            ) : null}
                            {["queued", "running"].includes(selectedSubagent.status) ? (
                              <Button
                                variant="default"
                                size="sm"
                                disabled={cancellingSubagentTaskId === selectedSubagent.taskId}
                                onClick={() => void handleCancelSubagent(selectedSubagent.taskId)}
                              >
                                {cancellingSubagentTaskId === selectedSubagent.taskId
                                  ? "正在取消…"
                                  : "取消子任务"}
                              </Button>
                            ) : null}
                          </div>
                        ) : null}
                      </InspectorRail>
                    )
                  }
                />
              )}
            </div>
          }
        />
      </div>
      <Dialog
        open={createProjectDialogOpen}
        title="创建项目"
        onOpenChange={(open) => {
          if (!open && createProjectSubmitting) {
            return;
          }
          setCreateProjectDialogOpen(open);
        }}
        footer={
          <>
            <div className="create-project-dialog-source">
              <SelectableCard
                compact
                selected={pendingProjectMode === "local"}
                icon={<AwakenIcon concept="theme-system" size={16} />}
                title="Local"
                onSelect={() => setPendingProjectMode("local")}
              />
            </div>
            <Button
              variant="default"
              size="md"
              disabled={createProjectSubmitting}
              onClick={() => setCreateProjectDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={createProjectSubmitting}
              onClick={() => {
                void handleCreateProject();
              }}
            >
              创建项目
            </Button>
          </>
        }
      >
        <div className="create-project-dialog-body">
          <label className="create-project-dialog-field">
            <span className="create-project-dialog-label">项目名称</span>
            <input
              type="text"
              className="create-project-dialog-input"
              value={draftProjectName}
              onChange={(event) => setDraftProjectName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  void handleCreateProject();
                }
              }}
              placeholder="例如：季度复盘"
              disabled={createProjectSubmitting}
            />
          </label>
          <ProjectFoldersEditor
            folders={draftProjectFolders.map((path) => ({
              path,
              isPrimary: isSameWorkspacePath(path, draftPrimaryFolder),
              onMakePrimary: (target: string) => setDraftPrimaryFolder(target),
              onRemove: handleRemoveDraftProjectFolder,
            }))}
            addDisabled={pickingWorkspace || createProjectSubmitting}
            emptyHint="可以先不选文件夹。项目会直接创建，之后随时能加。"
            onAddFolder={() => {
              void handleAddDraftProjectFolder();
            }}
          />
        </div>
      </Dialog>
      <Dialog
        open={Boolean(editingProjectId)}
        title="编辑项目"
        onOpenChange={(open) => {
          if (!open && !projectFolderBusyPath) {
            setEditingProjectId(null);
          }
        }}
        footer={
          <Button
            variant="primary"
            size="md"
            disabled={Boolean(projectFolderBusyPath)}
            onClick={() => setEditingProjectId(null)}
          >
            完成
          </Button>
        }
      >
        <div className="create-project-dialog-body">
          <ProjectFoldersEditor
            folders={editingProjectId ? projectFolderRows(editingProjectId) : []}
            addDisabled={pickingWorkspace || Boolean(projectFolderBusyPath)}
            busyPath={projectFolderBusyPath}
            emptyHint="这个项目还没有文件夹。"
            onAddFolder={() => {
              if (editingProjectId) {
                void handleAddProjectFolder(editingProjectId);
              }
            }}
          />
        </div>
      </Dialog>
      <Dialog
        open={Boolean(pendingProjectDelete)}
        title={projectDeleteDialogTitle}
        onOpenChange={(open) => {
          if (!open && !projectDeleteSubmitting) {
            setPendingProjectDelete(null);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={projectDeleteSubmitting}
              onClick={() => setPendingProjectDelete(null)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={projectDeleteSubmitting}
              onClick={() => {
                void handleConfirmProjectDelete();
              }}
            >
              {projectDeleteConfirmLabel}
            </Button>
          </>
        }
      >
        <p className="project-delete-dialog-copy">{projectDeleteDialogBody}</p>
      </Dialog>
      <Dialog
        open={Boolean(pendingThreadDelete)}
        title={threadDeleteDialogTitle}
        onOpenChange={(open) => {
          if (!open && !threadDeleteSubmitting) {
            setPendingThreadDelete(null);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={threadDeleteSubmitting}
              onClick={() => setPendingThreadDelete(null)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={threadDeleteSubmitting}
              disabled={pendingThreadDeleteCount === 0}
              onClick={() => {
                void handleConfirmThreadDelete();
              }}
            >
              {pendingThreadDelete?.source === "batch"
                ? `删除 ${pendingThreadDeleteCount} 个对话`
                : "删除会话"}
            </Button>
          </>
        }
      >
        <p className="project-delete-dialog-copy">{threadDeleteDialogCopy}</p>
      </Dialog>
      <Dialog
        open={renameDialogOpen}
        title="重命名会话"
        onOpenChange={(open) => {
          if (!open) {
            handleCancelRename();
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={renameSubmitting}
              onClick={handleCancelRename}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={renameSubmitting}
              disabled={renameSubmitting || !renameDraft.trim()}
              onClick={() => {
                void handleConfirmRename();
              }}
            >
              确认
            </Button>
          </>
        }
      >
        <div className="rename-thread-dialog-body">
          <input
            ref={renameInputRef}
            type="text"
            className="rename-thread-dialog-input"
            value={renameDraft}
            onChange={(event) => setRenameDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void handleConfirmRename();
              }
              if (event.key === "Escape") {
                event.preventDefault();
                handleCancelRename();
              }
            }}
            placeholder="请输入会话标题"
            disabled={renameSubmitting}
          />
        </div>
      </Dialog>
      <DesktopSettingsDialog
        open={modelSettingsOpen}
        runtimeClient={runtimeClient}
        currentProjectId={activeThreadProjectId ?? selectedProjectId}
        theme={appearanceTheme}
        themePreference={appearanceThemePreference}
        themeSaving={appearanceSaving}
        themeMessage={appearanceMessage}
        onThemePreferenceChange={(theme) => {
          void handleAppearanceThemeChange(theme);
        }}
        onOpenChange={setModelSettingsOpen}
        initialModelSection={modelSettingsInitialModelSection}
        onProfilesChanged={() => {
          void refreshRuntimeInfoNow();
        }}
        onThreadsChanged={() => {
          void refreshThreads();
        }}
      />
      <ProfileBalancePopover
        open={profilePopoverOpen}
        anchorRef={profileButtonRef}
        runtimeClient={runtimeClient}
        themePreference={appearanceThemePreference}
        themeSaving={appearanceSaving}
        onThemePreferenceChange={(theme) => {
          void handleAppearanceThemeChange(theme);
        }}
        onOpenSettings={() => {
          setProfilePopoverOpen(false);
          setModelSettingsInitialModelSection("api");
          setModelSettingsOpen(true);
        }}
        onClose={() => setProfilePopoverOpen(false)}
      />
      <Dialog
        open={updateRestartDialogOpen}
        title="重启以完成更新"
        onOpenChange={(open) => {
          if (!open && !updateInstallStarting) {
            // “稍后”只关闭确认框；Main 仍保持 downloaded，侧栏保留重启入口。
            setUpdateRestartDialogOpen(false);
            setUpdateInstallBlockedCode(undefined);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={updateInstallStarting}
              onClick={() => {
                setUpdateRestartDialogOpen(false);
                setUpdateInstallBlockedCode(undefined);
              }}
            >
              稍后
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={updateInstallStarting}
              disabled={updateInstallStarting || updateInstallRetryBlocked}
              onClick={() => {
                void handleConfirmUpdateRestart();
              }}
            >
              立即重启
            </Button>
          </>
        }
      >
        <div className="desktop-update-dialog-body">
          <p>
            当前版本 v{desktopUpdateSnapshot?.currentVersion ?? "--"}，将更新到 v
            {desktopUpdateSnapshot?.availableVersion ?? "--"}。
          </p>
          {desktopUpdateSnapshot?.releaseName ? (
            <p>{desktopUpdateSnapshot.releaseName}</p>
          ) : null}
          {updateInstallBlockedNotice ? (
            <p role="alert">{updateInstallBlockedNotice}</p>
          ) : null}
        </div>
      </Dialog>
    </>
  );
}
