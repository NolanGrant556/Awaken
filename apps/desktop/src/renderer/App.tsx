import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  ChevronDown,
  ChevronUp,
  FolderPlus,
  ListFilter,
  MessageCirclePlus,
  Monitor,
  PanelRightClose,
  PanelRightOpen,
  Plug,
  Settings,
  Wrench,
} from "lucide-react";
import {
  AwakenAppShell as AppShell,
  AwakenAssistantMessage as AssistantMessage,
  AwakenButton as Button,
  AwakenComposerBar as ComposerBar,
  AwakenDisclosure as Disclosure,
  AwakenDialog as Dialog,
  AwakenDeliverableList as DeliverableList,
  AwakenExecutionFeed as ExecutionFeed,
  AwakenLightInspectorRail as LightInspectorRail,
  AwakenSelectableCard as SelectableCard,
  AwakenSidebar as Sidebar,
  AwakenSidebarSectionAction as SidebarSectionAction,
  AwakenTaskWelcome as TaskWelcome,
  AwakenThoughtTrace as ThoughtTrace,
  AwakenToolTraceGroup as ToolTraceGroup,
  AwakenToolTraceLine as ToolTraceLine,
  AwakenUserPromptBlock as UserPromptBlock,
  AwakenWorkbenchChrome as WorkbenchChrome,
  AwakenWorkbenchShell as WorkbenchShell,
  AwakenWorkbenchSidebarFrame as WorkbenchSidebarFrame,
  AwakenRunStage as RunStage,
  AwakenComposerDock as ComposerDock,
  type ComposerBarMetaItem,
  type ComposerBarStructuredInputValue,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/design-system";
import {
  createRuntimeClient,
  desktopOperationIssueFromError,
  type SubscribeHandlers,
} from "./runtime/client";
import type {
  AwakenTheme,
  AwakenThemePreference,
  ComposerToken,
  DesktopOperationIssue,
  RuntimeInfo,
  RuntimeProject,
  RunInputAttachment,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  RuntimeThread,
  StartRunRequest,
} from "../shared/runtime";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  type ThreadItem,
  type RuntimeAction,
  type UserQuestionItem,
} from "./runtime/reducer";
import type { PromptNarrativeToken, RunNarrativeItem } from "./run-narrative";
import { projectPetActivity } from "./pets/pet-activity-projector";
import {
  projectNarrativeTurnBodySegments,
  shouldShowAssistantAnswerFooter,
  shouldUseCenterComposer,
  turnMarkerRailHeight,
  turnMarkerRailDisplay,
  type NarrativeTurnBodySegment,
} from "./workbenchPresentation";
import { useComposerLifecycle } from "./useComposerLifecycle";
import { useRunStageScrollController } from "./useRunStageScrollController";
import {
  actionsForThreadTranscript,
  streamCursorForTranscriptRun,
} from "./runtime/thread-transcript";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  TRACKED_TRASH_OPERATIONS_STORAGE_KEY,
  canReconcileTrackedTrashOperation,
  isSameWorkspacePath,
  parseTrackedTrashOperations,
  projectIdForThread,
  projectOperationReconciliationState,
  upsertTrackedTrashOperation,
  workspaceLabelForPath,
  type TrackedTrashOperation,
  type WorkspaceProject,
} from "./projectWorkspace";
import { DesktopSettingsDialog } from "./settings/DesktopSettingsDialog";
import { ProfileBalancePopover } from "./ProfileBalancePopover";
import { ExtensionsView, type ExtensionSection } from "./ExtensionsView";
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
  clearSlashTriggerFromComposerValue,
  composerTokenPlacementForSlashTrigger,
  composerTokenFromSlashAction,
  filterSlashActions,
  firstSelectableSlashAction,
  skillSlashProvider,
  slashTriggerFromComposerValue,
  type ComposerTokenPlacement,
  type SlashAction,
} from "./composerSlash";

type UserQuestionPromptProps = {
  question: UserQuestionItem;
  disabled?: boolean;
  onSubmit: (answer: string) => void;
};

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
};

export type WorkbenchInspectorPresentation = {
  available: boolean;
  effectiveCollapsed: boolean;
  rootClassName:
    | "desktop-root-inspector-expanded"
    | "desktop-root-inspector-collapsed"
    | "desktop-root-inspector-hidden";
};

export function workbenchInspectorPresentation(
  activeExtensionSection: ExtensionSection | undefined,
  inspectorCollapsed: boolean,
): WorkbenchInspectorPresentation {
  if (activeExtensionSection) {
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

const emptyUserQuestionSelection = (): UserQuestionSelection => ({
  selected: [],
  otherText: "",
});

export function selectedUserQuestionLabels(
  selection: UserQuestionSelection,
): string[] {
  const labels = selection.selected.filter(
    (label) => label !== USER_QUESTION_OTHER_OPTION,
  );
  if (
    selection.selected.includes(USER_QUESTION_OTHER_OPTION) &&
    selection.otherText.trim()
  ) {
    labels.push(selection.otherText.trim());
  }
  return labels;
}

export function isUserQuestionSelectionAnswered(
  selection: UserQuestionSelection,
): boolean {
  return selectedUserQuestionLabels(selection).length > 0;
}

export function nextUserQuestionPromptIndex(
  currentIndex: number,
  questionCount: number,
): number {
  if (questionCount <= 0) {
    return 0;
  }
  return Math.min(currentIndex + 1, questionCount - 1);
}

function clampUserQuestionPromptIndex(
  index: number,
  questionCount: number,
): number {
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
    nextValue.tokens
      .map((token) => token.id)
      .filter((tokenId) => currentTokenIds.has(tokenId)),
  );
  const nextTokens = tokens
    .filter((token) => remainingTokenIds.has(token.id))
    .map((token) => {
      const leadingText = leadingTextByTokenId.get(token.id) ?? "";
      const tokenWithoutLeadingText = { ...token };
      delete tokenWithoutLeadingText.leadingText;
      return leadingText
        ? { ...tokenWithoutLeadingText, leadingText }
        : tokenWithoutLeadingText;
    });

  return {
    tokens: nextTokens,
    placements: Object.fromEntries(
      Object.entries(placements).filter(([tokenId]) =>
        remainingTokenIds.has(tokenId),
      ),
    ),
    leadingTexts: Object.fromEntries(
      nextValue.tokens
        .filter(
          (token) => remainingTokenIds.has(token.id) && token.leadingText,
        )
        .map((token) => [token.id, token.leadingText]),
    ),
    value: nextValue.value,
  };
}

export function promptTokenViews(tokens?: PromptNarrativeToken[]) {
  return tokens?.map((token) => ({
    id: token.id,
    type: token.type,
    label: token.label,
    leadingText: token.leadingText || undefined,
    title:
      typeof token.payload?.skillFilePath === "string"
        ? token.payload.skillFilePath
        : token.label,
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
      (candidate, index, values) =>
        candidate.length > 0 && values.indexOf(candidate) === index,
    );
    const matchedLeadingText = candidates.find((candidate) =>
      remainingText.startsWith(candidate),
    );
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
  onSubmit,
}: UserQuestionPromptProps) {
  const [answers, setAnswers] = useState<Record<number, UserQuestionSelection>>(
    {},
  );
  const [activeQuestionIndex, setActiveQuestionIndex] = useState(0);
  const otherInputRefs = useRef<Record<number, HTMLInputElement | null>>({});
  const questions = question.questions;

  useEffect(() => {
    setAnswers({});
    setActiveQuestionIndex(0);
  }, [question.id]);

  useEffect(() => {
    setActiveQuestionIndex((current) =>
      clampUserQuestionPromptIndex(current, questions.length),
    );
  }, [questions.length]);

  if (questions.length === 0) {
    return null;
  }

  const questionCount = questions.length;
  const currentPageIndex = clampUserQuestionPromptIndex(
    activeQuestionIndex,
    questionCount,
  );
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
          current === index
            ? nextUserQuestionPromptIndex(index, questionCount)
            : current,
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

  const canSubmit = questions.every(
    (_item, index) => selectedLabelsForQuestion(index).length > 0,
  );
  const currentQuestionAnswered = isUserQuestionSelectionAnswered(
    currentAnswer(currentPageIndex),
  );
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
  const options = Array.isArray(activeQuestion.options)
    ? activeQuestion.options
    : [];
  const hasOther = isSelected(currentPageIndex, USER_QUESTION_OTHER_OPTION);
  const primaryActionDisabled =
    disabled || (hasNextQuestion ? !currentQuestionAnswered : !canSubmit);
  const primaryActionLabel = hasNextQuestion ? "下一题" : "提交回答";

  return (
    <div
      className="user-question-card"
      aria-label={`需要回答的问题 ${currentPageIndex + 1} of ${questionCount}`}
    >
      {activeQuestion.header || hasMultipleQuestions ? (
        <div className="user-question-card__topbar">
          {activeQuestion.header ? (
            <div className="user-question-card__header">
              {activeQuestion.header}
            </div>
          ) : (
            <span aria-hidden="true" />
          )}
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
                <ChevronUp size={15} strokeWidth={1.9} />
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
                <ChevronDown size={15} strokeWidth={1.9} />
              </button>
            </div>
          ) : null}
        </div>
      ) : null}
      <div className="user-question-card__body">
        <div
          className="user-question-card__question"
          key={`${currentPageIndex}:${activeQuestion.question}`}
        >
          <div className="user-question-card__title">
            {activeQuestion.question}
          </div>
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
                onClick={() =>
                  toggleOption(currentPageIndex, isMulti, option.label)
                }
                disabled={disabled}
              >
                <span className="user-question-card__option-label">
                  {option.label}
                </span>
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
                  toggleOption(
                    currentPageIndex,
                    isMulti,
                    USER_QUESTION_OTHER_OPTION,
                  );
                }
              }}
              onKeyDown={(event) => {
                if (disabled || (event.key !== "Enter" && event.key !== " ")) {
                  return;
                }
                event.preventDefault();
                toggleOption(
                  currentPageIndex,
                  isMulti,
                  USER_QUESTION_OTHER_OPTION,
                );
              }}
              role="button"
              aria-pressed={hasOther}
              tabIndex={disabled ? -1 : 0}
            >
              <span className="user-question-card__option-label">
                其他（输入你的答案）
              </span>
              {hasOther ? (
                <input
                  ref={(node) => {
                    otherInputRefs.current[currentPageIndex] = node;
                  }}
                  type="text"
                  className="user-question-card__other-input"
                  placeholder="请输入你的回答..."
                  value={currentAnswer(currentPageIndex).otherText}
                  onChange={(event) =>
                    setOtherText(currentPageIndex, event.target.value)
                  }
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
  if (
    thread.runStatus === "running" ||
    thread.runStatus === "cancelling"
  ) {
    return true;
  }
  if (thread.runStatus) {
    return false;
  }
  return Boolean(thread.activeRunId);
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
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  defaultProjectRemoved?: boolean;
  activeThreadWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): string {
  const {
    workspacePath,
    workspaceTargetPath,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  } = args;

  const explicitTarget = workspaceTargetPath?.trim();
  if (explicitTarget) {
    return explicitTarget;
  }

  const selectedProjectPath =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : projectsFromWorkspace.find(
            (project) => project.id === selectedProjectId,
          )?.path;

  return (
    [
      selectedProjectPath,
      activeThreadWorkspacePath,
      workspacePath,
      defaultProjectRemoved ? undefined : defaultWorkspacePath,
    ].find((candidate) => candidate?.trim())?.trim() ?? ""
  );
}

function sameWorkspaceProjects(
  current: WorkspaceProject[],
  next: WorkspaceProject[],
): boolean {
  return (
    next.length === current.length &&
    next.every((project, index) => {
      const existing = current[index];
      return (
        existing?.id === project.id &&
        existing.backendProjectId === project.backendProjectId &&
        existing.path === project.path &&
        existing.label === project.label
      );
    })
  );
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

function rawToolId(selectionId: string): string {
  if (selectionId.startsWith("tool:")) {
    return selectionId.slice("tool:".length);
  }
  if (selectionId.startsWith("explore:")) {
    return selectionId.slice("explore:".length);
  }
  return selectionId;
}

function contextUsageMeta(
  contextWindow: ReturnType<
    typeof createInitialRuntimeState
  >["inspectorSummary"]["contextWindow"],
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const usageValue =
    typeof contextWindow?.actualTokens === "number"
      ? contextWindow.actualTokens
      : contextWindow?.estimatedTokens;
  const usage = formatTokenCount(usageValue);
  const modelWindow = formatTokenCount(contextWindow?.modelWindow);
  if (usage && modelWindow) {
    const progress =
      typeof usageValue === "number" &&
      typeof contextWindow?.modelWindow === "number" &&
      contextWindow.modelWindow > 0
        ? usageValue / contextWindow.modelWindow
        : undefined;
    return {
      label: "context windows",
      value: `${usage} / ${modelWindow}`,
      progress,
    };
  }
  if (usage) {
    return {
      label: "context windows",
      value: usage,
    };
  }
  return undefined;
}

function modelProfileLabel(profile: RuntimeModelProfileSummary): string {
  return profile.modelId || profile.displayName;
}

function modelProfileIcon(profile: RuntimeModelProfileSummary): string {
  return profile.displayName.trim().slice(0, 1) || "M";
}

function firstContentLine(value?: string): string | undefined {
  return value
    ?.split("\n")
    .map((line) => line.trim())
    .find(Boolean);
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
const DEFAULT_INSPECTOR_WIDTH = 300;
const MIN_INSPECTOR_WIDTH = 260;
const MAX_INSPECTOR_WIDTH = 520;
const LAYOUT_STEP = 12;
const INSPECTOR_RESPONSIVE_COLLAPSE_QUERY = "(max-width: 1180px)";
const STORED_THEME_KEY = "awaken:appearance:theme";
const SYSTEM_THEME_QUERY = "(prefers-color-scheme: dark)";

function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function readStoredNumber(
  key: string,
  fallback: number,
  min: number,
  max: number,
): number {
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
    () => [
      skillSlashProvider((workspaceRoot) =>
        runtimeClient.listSkills(workspaceRoot),
      ),
    ],
    [runtimeClient],
  );

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const petThreadActivationRef = useRef<(threadId: string) => void>(() => {});
  petThreadActivationRef.current = (threadId: string) => {
    void handleSelectConversation(threadId);
  };

  useEffect(() => {
    const snapshot = projectPetActivity(runtimeState);
    runtimeClient.updatePetActivity(snapshot);
  }, [runtimeState, runtimeClient]);

  useEffect(
    () =>
      runtimeClient.onPetThreadActivationRequested((threadId) => {
        petThreadActivationRef.current(threadId);
      }),
    [runtimeClient],
  );

  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [openingArtifactId, setOpeningArtifactId] = useState<string | null>(null);
  const [answeringUserQuestionId, setAnsweringUserQuestionId] =
    useState<string | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [modelProfiles, setModelProfiles] = useState<
    RuntimeModelProfileSummary[]
  >([]);
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [modelSettingsInitialModelSection, setModelSettingsInitialModelSection] =
    useState<"configure" | "configured">("configure");
  const [runCreateIssue, setRunCreateIssue] =
    useState<DesktopOperationIssue | null>(null);
  const [appearanceThemePreference, setAppearanceThemePreference] =
    useState<AwakenThemePreference>(readStoredThemePreference);
  const [systemAppearanceTheme, setSystemAppearanceTheme] =
    useState<AwakenTheme>(readSystemTheme);
  const appearanceTheme = effectiveThemeForPreference(
    appearanceThemePreference,
    systemAppearanceTheme,
  );
  const [appearanceSaving, setAppearanceSaving] = useState(false);
  const [appearanceMessage, setAppearanceMessage] = useState<string | null>(null);
  const [profilePopoverOpen, setProfilePopoverOpen] = useState(false);
  const profileButtonRef = useRef<HTMLDivElement>(null);
  const [workspacePath, setWorkspacePath] = useState("");
  const [createProjectDialogOpen, setCreateProjectDialogOpen] = useState(false);
  const [renameDialogOpen, setRenameDialogOpen] = useState(false);
  const [renamingThreadId, setRenamingThreadId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  const [renameSubmitting, setRenameSubmitting] = useState(false);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const [pendingProjectMode, setPendingProjectMode] =
    useState<"local">("local");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<
    WorkspaceProject[]
  >([]);
  const [selectedProjectId, setSelectedProjectId] = useState<
    string | undefined
  >(undefined);
  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>([]);
  const [removedProjectIds, setRemovedProjectIds] = useState<string[]>([]);
  const [defaultBackendProjectId, setDefaultBackendProjectId] = useState<
    string | undefined
  >(undefined);
  const [pendingProjectDelete, setPendingProjectDelete] =
    useState<PendingProjectDelete | null>(null);
  const [projectDeleteSubmitting, setProjectDeleteSubmitting] = useState(false);
  const [trackedTrashOperations, setTrackedTrashOperations] = useState<
    TrackedTrashOperation[]
  >(() =>
    parseTrackedTrashOperations(
      readStoredString(TRACKED_TRASH_OPERATIONS_STORAGE_KEY),
    ),
  );
  const defaultProjectRemoved = removedProjectIds.includes(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );
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
    readStoredNumber(
      "awaken:layout:inspectorWidth",
      DEFAULT_INSPECTOR_WIDTH,
      MIN_INSPECTOR_WIDTH,
      MAX_INSPECTOR_WIDTH,
    ),
  );
  const [inspectorCollapsed, setInspectorCollapsed] = useState(() =>
    readStoredBoolean("awaken:layout:inspectorCollapsed", false),
  );
  const [promptBubbleModeByTurnId, setPromptBubbleModeByTurnId] = useState<
    Record<string, "collapsed" | "expanded">
  >({});
  const [executionProcessOpenByTurnId, setExecutionProcessOpenByTurnId] =
    useState<Record<string, boolean>>({});
  const [composerAttachments, setComposerAttachments] = useState<
    ComposerDraftAttachment[]
  >([]);
  const [composerTokens, setComposerTokens] = useState<ComposerToken[]>([]);
  const [composerTokenPlacements, setComposerTokenPlacements] = useState<
    Record<string, ComposerTokenPlacement>
  >({});
  const [composerTokenLeadingTexts, setComposerTokenLeadingTexts] = useState<
    Record<string, string>
  >({});
  const [slashActions, setSlashActions] = useState<SlashAction[]>([]);
  const [slashLoading, setSlashLoading] = useState(false);
  const [slashError, setSlashError] = useState<string | null>(null);
  const [slashActiveActionId, setSlashActiveActionId] = useState<
    string | undefined
  >();
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem
        ? [runtimeState.narrative.activeItem]
        : []),
    ];
    return narrativeItems.filter((item) => item.kind !== "planning");
  }, [runtimeState.narrative]);

  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  const desktopReady =
    runtimeState.supervisorStatus === "running" && runtimeInfo !== null;
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
    document.documentElement.dataset.awThemePreference =
      appearanceThemePreference;
    document.documentElement.style.colorScheme =
      appearanceTheme === "dark" ? "dark" : "light";
    writeStoredValue(STORED_THEME_KEY, appearanceThemePreference);
  }, [appearanceTheme, appearanceThemePreference]);

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
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
    writeStoredValue("awaken:layout:sidebarWidth", String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorWidth", String(inspectorWidth));
  }, [inspectorWidth]);

  useEffect(() => {
    writeStoredValue(
      "awaken:layout:inspectorCollapsed",
      String(inspectorCollapsed),
    );
    inspectorCollapsedValueRef.current = inspectorCollapsed;
  }, [inspectorCollapsed]);

  useEffect(() => {
    writeStoredValue(
      TRACKED_TRASH_OPERATIONS_STORAGE_KEY,
      JSON.stringify(trackedTrashOperations),
    );
  }, [trackedTrashOperations]);

  useEffect(() => {
    workspaceTargetPathRef.current = workspacePath;
  }, [workspacePath]);

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return;
    }

    const media = window.matchMedia(INSPECTOR_RESPONSIVE_COLLAPSE_QUERY);
    const handleResponsiveInspector = (matches: boolean) => {
      if (matches) {
        if (inspectorCollapsedBeforeResponsiveRef.current === null) {
          inspectorCollapsedBeforeResponsiveRef.current =
            inspectorCollapsedValueRef.current;
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
        return;
      }
      try {
        const [info, profiles] = await Promise.all([
          runtimeClient.getRuntimeInfo(),
          runtimeClient.listModelProfiles().catch(() => []),
        ]);
        if (!cancelled) {
          setRuntimeInfo(info);
          setModelProfiles(profiles);
        }
      } catch {
        if (!cancelled) {
          setRuntimeInfo(null);
          setModelProfiles([]);
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
    if (
      rendererReadyNotifiedRef.current ||
      (!desktopReady && !desktopNeedsRestart)
    ) {
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
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
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
    const configuredDefaultWorkspacePath =
      runtimeInfo?.defaultWorkspace ?? workspacePath;
    const defaultProject = projects.find((project) =>
      isSameWorkspacePath(
        project.workspaceRoot,
        configuredDefaultWorkspacePath,
      ),
    );
    setDefaultBackendProjectId(defaultProject?.projectId);
    setRemovedProjectIds(
      defaultProject && defaultProject.state !== "active"
        ? [DEFAULT_WORKSPACE_PROJECT_ID]
        : [],
    );

    const nextProjects = projects
      .filter(
        (project) =>
          project.state === "active" &&
          !isSameWorkspacePath(
            project.workspaceRoot,
            configuredDefaultWorkspacePath,
          ),
      )
      .map<WorkspaceProject>((project) => ({
        id: project.projectId,
        backendProjectId: project.projectId,
        label: project.displayName || workspaceLabelForPath(project.workspaceRoot),
        path: project.workspaceRoot,
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
    if (!desktopReady || trackedTrashOperations.length === 0) {
      return;
    }

    let cancelled = false;
    let timer: number | undefined;

    async function pollTrackedTrashOperations(): Promise<void> {
      const checked = await Promise.all(
        trackedTrashOperations.map(async (tracked) => {
          try {
            const operation = await runtimeClient.getProjectOperation(
              tracked.operationId,
            );
            projectOperationPollFailuresRef.current.delete(tracked.operationId);
            return {
              tracked,
              operation,
              state: projectOperationReconciliationState(operation),
            };
          } catch (error) {
            if (
              !projectOperationPollFailuresRef.current.has(tracked.operationId)
            ) {
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
        (
          result,
        ): result is Exclude<(typeof checked)[number], undefined> =>
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
              (candidate) =>
                candidate.projectId === tracked.backendProjectId,
            );
            return canReconcileTrackedTrashOperation(state, project?.state);
          });

          if (reconciled.length > 0) {
            applyRuntimeProjects(projects);
            const reconciledIds = new Set(
              reconciled.map(({ tracked }) => tracked.operationId),
            );
            const failed = reconciled.filter(
              ({ state }) => state === "failed",
            );
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
                `未能将“${failed[0].tracked.label}”移到废纸篓，项目、历史和磁盘文件均保持不变。`,
              );
            } else {
              setNotice(
                `已将“${reconciled[0].tracked.label}”移到废纸篓。`,
              );
            }
            setTrackedTrashOperations((current) =>
              current.filter(
                (tracked) => !reconciledIds.has(tracked.operationId),
              ),
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
      return;
    }
    try {
      const [info, profiles] = await Promise.all([
        runtimeClient.getRuntimeInfo(),
        runtimeClient.listModelProfiles().catch(() => []),
      ]);
      setRuntimeInfo(info);
      setModelProfiles(profiles);
    } catch {
      setRuntimeInfo(null);
      setModelProfiles([]);
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

    const delays = [500, 1000, 2000, 4000, 8000, 12000, 20000, 30000];

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

  async function handleSelectModelProfile(profileId: string): Promise<void> {
    try {
      setNotice(null);
      await runtimeClient.setDefaultModelProfile(profileId);
      await refreshRuntimeInfoNow();
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to switch model",
      );
    }
  }

  async function handleAppearanceThemeChange(
    theme: AwakenThemePreference,
  ): Promise<void> {
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
      setAppearanceMessage(
        error instanceof Error ? error.message : "Failed to save theme",
      );
    } finally {
      setAppearanceSaving(false);
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
    localStorage.removeItem("awaken:runId");
    prepareForNewTask();
    dispatch({ type: "run/reset" });
  }

  const enabledModelProfiles = modelProfiles.filter(
    (profile) => profile.enabled,
  );
  const selectedModelProfile =
    enabledModelProfiles.find(
      (profile) => profile.id === runtimeInfo?.modelProfile?.id,
    ) ??
    enabledModelProfiles.find((profile) => profile.isDefault) ??
    enabledModelProfiles[0];
  const defaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;
  const effectiveDefaultWorkspacePath = defaultProjectRemoved
    ? undefined
    : defaultWorkspacePath;
  // Without a user-selected workspace there is no implicit default project;
  // Projects shows a true empty state until the user opens/adds a project.
  const hasDefaultWorkspaceProject = Boolean(
    effectiveDefaultWorkspacePath?.trim(),
  );
  const activeThread = runtimeState.activeThreadId
    ? runtimeState.threads.find(
        (thread) => thread.threadId === runtimeState.activeThreadId,
      )
    : undefined;
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
        : projectsFromWorkspace.find(
            (project) => project.id === selectedProjectId,
          )?.path;
  const workspaceRootForRun = resolveWorkspaceRootForRun({
    workspacePath,
    workspaceTargetPath: workspaceTargetPathRef.current,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  });
  const isWorkspaceValid =
    workspaceRootForRun !== "" &&
    /^([A-Za-z]:[\\/]|\/)/.test(workspaceRootForRun);
  const {
    composerValue,
    setComposerValue,
    clearComposerValue,
    restoreComposerValue,
    composerFocusRequest,
    requestComposerFocus,
    composerBarState,
    composerRuntimePanels,
  } = useComposerLifecycle({
    desktopReady,
    desktopStarting,
    desktopNeedsRestart,
    isBusy,
    isWorkspaceValid,
    hasAttachments: composerAttachments.length > 0 || composerTokens.length > 0,
    inputAttachmentBlocked: false,
    runtimeErrorMessage: runtimeState.errorMessage,
    runtimeOperationIssue: runCreateIssue,
  });
  const composerInlineTextPrefix = useMemo(
    () =>
      composerTokens
        .map(
          (token) =>
            token.leadingText ?? composerTokenLeadingTexts[token.id] ?? "",
        )
        .join(""),
    [composerTokenLeadingTexts, composerTokens],
  );
  const trimmedEffectiveComposerValue =
    `${composerInlineTextPrefix}${composerValue}`.trim();
  const slashTrigger = slashTriggerFromComposerValue(composerValue);
  const slashPaletteOpen =
    Boolean(slashTrigger) && desktopReady && isWorkspaceValid && !isBusy;
  const filteredSlashActions = useMemo(
    () =>
      filterSlashActions(slashActions, slashTrigger?.query ?? "").slice(0, 24),
    [slashActions, slashTrigger?.query],
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
        ({
          id,
          type,
          label,
          description,
          icon,
          disabled,
          disabledReason,
          preview,
        }) => ({
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
    void actionsFromSlashProviders(slashProviders, { workspaceRootForRun })
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
        setSlashError(
          error instanceof Error ? error.message : "斜杠菜单加载失败",
        );
      })
      .finally(() => {
        if (!cancelled) {
          setSlashLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [slashPaletteOpen, slashProviders, workspaceRootForRun]);

  useEffect(() => {
    if (!slashPaletteOpen) {
      return;
    }
    const activeStillVisible = filteredSlashActions.some(
      (action) => action.id === slashActiveActionId,
    );
    if (!activeStillVisible) {
      setSlashActiveActionId(
        firstSelectableSlashAction(filteredSlashActions)?.id,
      );
    }
  }, [filteredSlashActions, slashActiveActionId, slashPaletteOpen]);

  const activeUserQuestion = runtimeState.userQuestions[0];
  const navItems: NavItem[] = [
    {
      id: "connectors",
      label: "连接器",
      icon: <Plug size={16} strokeWidth={1.9} />,
    },
    {
      id: "skills",
      label: "技能",
      icon: <Wrench size={16} strokeWidth={1.9} />,
    },
  ];
  const topbarStatusLabel = desktopNeedsRestart
    ? "Needs restart"
    : desktopStarting
      ? "Starting"
      : undefined;
  useEffect(() => {
    const defaultProjectAvailable =
      !defaultProjectRemoved && hasDefaultWorkspaceProject;
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
      return !defaultProjectAvailable ||
        next.includes(DEFAULT_WORKSPACE_PROJECT_ID)
        ? next
        : [DEFAULT_WORKSPACE_PROJECT_ID, ...next];
    });
  }, [defaultProjectRemoved, hasDefaultWorkspaceProject, projectsFromWorkspace]);

  useEffect(() => {
    if (
      pathForSelectedProject &&
      !isSameWorkspacePath(workspacePath, pathForSelectedProject)
    ) {
      workspaceTargetPathRef.current = pathForSelectedProject;
      setWorkspacePath(pathForSelectedProject);
    }
  }, [pathForSelectedProject, workspacePath]);

  const projectLabelById = new Map<string, string>([
    [
      DEFAULT_WORKSPACE_PROJECT_ID,
      defaultWorkspacePath
        ? workspaceLabelForPath(defaultWorkspacePath)
        : "awaken",
    ],
    ...projectsFromWorkspace.map(
      (project) => [project.id, project.label] as const,
    ),
  ]);
  const removedProjectIdSet = new Set(removedProjectIds);
  const workspaceLabelProjectId = activeThreadProjectId ?? selectedProjectId;
  const workspaceLabelPath =
    activeThreadWorkspacePath ?? pathForSelectedProject;
  const activeProjectLabel =
    (workspaceLabelProjectId
      ? projectLabelById.get(workspaceLabelProjectId)
      : undefined) ??
    (workspaceLabelPath
      ? workspaceLabelForPath(workspaceLabelPath)
      : "awaken");
  const contextMeta = contextUsageMeta(
    runtimeState.inspectorSummary.contextWindow,
  );
  const modelMetaItem: ComposerBarMetaItem =
    enabledModelProfiles.length > 0 && selectedModelProfile
      ? {
          id: "model",
          label: modelProfileLabel(selectedModelProfile),
          menuItems: enabledModelProfiles.map((profile) => ({
            id: profile.id,
            label: modelProfileLabel(profile),
            icon: modelProfileIcon(profile),
            selected: profile.id === selectedModelProfile.id,
            onSelect: () => {
              if (profile.id !== selectedModelProfile.id) {
                void handleSelectModelProfile(profile.id);
              }
            },
          })),
          menuFooterAction: {
            label: "添加模型",
            icon: <Settings size={16} strokeWidth={1.9} />,
            onClick: () => {
              setModelSettingsInitialModelSection("configure");
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
    ...(contextMeta ? [{ id: "context", ...contextMeta }] : []),
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
        <MessageCirclePlus size={13} strokeWidth={1.85} />
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

  function requestProjectDelete(
    projectId: string,
    action: ProjectDeleteAction,
  ): void {
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

  function projectMenuItems(projectId: string) {
    return [
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
      {
        id: "trash-project",
        label: "移到废纸篓",
        tone: "danger" as const,
        onSelect: () => {
          requestProjectDelete(projectId, "trash");
        },
      },
    ];
  }

  function threadItemsForProject(projectId: string): ProjectItem[] {
    return threadsForProject(projectId).map((thread) => {
      return {
        id: `thread:${thread.threadId}`,
        kind: "thread",
        label: thread.label,
        isLoading: shouldShowSidebarThreadSpinner(thread),
        menuItems: [
          {
            id: "rename-thread",
            label: "重命名",
            onSelect: () => {
              void handleRenameThread(thread.threadId);
            },
          },
          {
            id: "delete-thread",
            label: "删除会话",
            tone: "danger",
            onSelect: () => {
              void handleDeleteThread(thread.threadId);
            },
          },
        ],
      };
    });
  }

  const defaultProjectThreads = threadItemsForProject(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );
  const repositoryProjects: ProjectItem[] = [
    ...(removedProjectIdSet.has(DEFAULT_WORKSPACE_PROJECT_ID) ||
    !hasDefaultWorkspaceProject
      ? []
      : [
          {
            id: DEFAULT_WORKSPACE_PROJECT_ID,
            kind: "project" as const,
            label:
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            path: defaultWorkspacePath,
            action: projectAction(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            ),
            menuItems: projectMenuItems(DEFAULT_WORKSPACE_PROJECT_ID),
            children:
              defaultProjectThreads.length > 0
                ? defaultProjectThreads
                : undefined,
          },
        ]),
    ...projectsFromWorkspace
      .filter(
        (project) =>
          !isSameWorkspacePath(
            project.path,
            effectiveDefaultWorkspacePath,
          ),
      )
      .map((project) => {
        const threads = threadItemsForProject(project.id);
        return {
          id: project.id,
          kind: "project" as const,
          label: project.label,
          path: project.path,
          action: projectAction(project.id, project.label),
          menuItems: projectMenuItems(project.id),
          children: threads.length > 0 ? threads : undefined,
        };
      }),
  ];
  const activeProjectId =
    runtimeState.activeThreadId
      ? `thread:${runtimeState.activeThreadId}`
      : selectedProjectId;
  const inspectorPlaceholderItems = [
    {
      id: "right-sidebar-placeholder",
      label: "右侧栏待开发",
      disabled: true,
      tone: "neutral" as const,
    },
  ];
  const showCenterComposer = shouldUseCenterComposer({
    hasNarrative: visibleNarrativeItems.length > 0,
    hasRun: Boolean(runtimeState.runId),
    hasNotice: Boolean(notice),
    hasError: Boolean(runtimeState.errorMessage),
    needsRestart: desktopNeedsRestart,
  });
  const {
    activeWorkbenchTurnId,
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
  const showTurnMarkerRail = turnMarkerDisplay.mode !== "hidden";
  const activeExtensionSection: ExtensionSection | undefined =
    activeNavId === "connectors" || activeNavId === "skills"
      ? activeNavId
      : undefined;
  const inspectorPresentation = workbenchInspectorPresentation(
    activeExtensionSection,
    inspectorCollapsed,
  );

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
        const receivedAtMs = Date.now();
        if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
          terminalSeenRef.current = true;
          localStorage.removeItem("awaken:runId");
        }
        if (
          event.type === "RUN_STATUS_CHANGED" &&
          event.status === "cancelled"
        ) {
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
    for (const action of actions) {
      dispatch(action);
    }
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

    syncProjectSelectionForWorkspace(
      transcript.thread.workspace.workspaceRoot,
    );
    applyRuntimeActions(actionsForThreadTranscript(transcript));

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

  function handleSlashActionSelect(actionId: string): void {
    const action = filteredSlashActions.find((item) => item.id === actionId);
    if (!action || action.disabled) {
      return;
    }
    const token = composerTokenFromSlashAction(action);
    if (!token) {
      return;
    }
    const currentSlashTrigger = slashTriggerFromComposerValue(composerValue);
    const placement = composerTokenPlacementForSlashTrigger(currentSlashTrigger);
    const leadingText =
      placement === "after" && currentSlashTrigger
        ? composerValue.slice(0, currentSlashTrigger.start)
        : "";
    // The token itself is the durable owner of its leading text; the
    // composerTokenLeadingTexts map only mirrors it for composer editing.
    const tokenWithPlacement = leadingText ? { ...token, leadingText } : token;
    setComposerTokens((current) =>
      current.some((item) => item.id === token.id)
        ? current
        : [...current, tokenWithPlacement],
    );
    setComposerTokenPlacements((current) =>
      current[token.id] === placement ? current : { ...current, [token.id]: placement },
    );
    setComposerTokenLeadingTexts((current) => {
      if (!leadingText) {
        if (!(token.id in current)) {
          return current;
        }
        const next = { ...current };
        delete next[token.id];
        return next;
      }
      return current[token.id] === leadingText
        ? current
        : { ...current, [token.id]: leadingText };
    });
    setComposerValue((current) => {
      const trigger = slashTriggerFromComposerValue(current);
      if (!trigger) {
        return current;
      }
      return composerTokenPlacementForSlashTrigger(trigger) === "after"
        ? current.slice(trigger.end)
        : clearSlashTriggerFromComposerValue(current);
    });
    setSlashActiveActionId(undefined);
    requestComposerFocus();
  }

  function handleRemoveComposerToken(tokenId: string): void {
    const removedTokenIndex = composerTokens.findIndex(
      (item) => item.id === tokenId,
    );
    const removedToken = composerTokens[removedTokenIndex];
    const leadingText =
      removedToken?.leadingText ?? composerTokenLeadingTexts[tokenId] ?? "";
    const nextToken = composerTokens[removedTokenIndex + 1];
    if (leadingText && nextToken) {
      const nextLeadingText =
        nextToken.leadingText ?? composerTokenLeadingTexts[nextToken.id] ?? "";
      const mergedLeadingText = `${leadingText}${nextLeadingText}`;
      setComposerTokens((current) =>
        current
          .filter((token) => token.id !== tokenId)
          .map((token) =>
            token.id === nextToken.id
              ? { ...token, leadingText: mergedLeadingText }
              : token,
          ),
      );
      setComposerTokenLeadingTexts((current) => {
        const next = { ...current, [nextToken.id]: mergedLeadingText };
        delete next[tokenId];
        return next;
      });
    } else if (leadingText) {
      setComposerValue((current) => `${leadingText}${current}`);
      setComposerTokens((current) =>
        current.filter((token) => token.id !== tokenId),
      );
    } else {
      setComposerTokens((current) =>
        current.filter((token) => token.id !== tokenId),
      );
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

  function handleComposerStructuredInputChange(
    nextValue: ComposerBarStructuredInputValue,
  ): void {
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
      setComposerValue((current) => clearSlashTriggerFromComposerValue(current));
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
    const selectableActions = filteredSlashActions.filter(
      (action) => !action.disabled,
    );
    if (selectableActions.length === 0) {
      return;
    }
    const activeIndex = Math.max(
      0,
      selectableActions.findIndex((action) => action.id === slashActiveActionId),
    );
    if (event.key === "ArrowDown") {
      setSlashActiveActionId(
        selectableActions[(activeIndex + 1) % selectableActions.length]?.id,
      );
      return;
    }
    if (event.key === "ArrowUp") {
      setSlashActiveActionId(
        selectableActions[
          (activeIndex - 1 + selectableActions.length) %
            selectableActions.length
        ]?.id,
      );
      return;
    }
    handleSlashActionSelect(
      slashActiveActionId ?? selectableActions[0]?.id ?? "",
    );
  }

  async function handleSubmit(): Promise<void> {
    if (!desktopReady) {
      setNotice(
        desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting",
      );
      return;
    }
    const prompt = trimmedEffectiveComposerValue;
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
    const draftPromptTokens = promptNarrativeTokensForComposer(
      draftComposerTokens,
      draftComposerTokenLeadingTexts,
    );
    const runInputAttachments = draftAttachments.map(toRunInputAttachment);
    if (
      !prompt &&
      runInputAttachments.length === 0 &&
      draftComposerTokens.length === 0
    ) {
      setNotice("请输入内容或添加附件");
      return;
    }
    if (isBusy) {
      return;
    }
    if (!isWorkspaceValid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const workspaceRoot = resolveWorkspaceRootForRun({
      workspacePath,
      workspaceTargetPath: workspaceTargetPathRef.current,
      selectedProjectId,
      defaultWorkspacePath,
      activeThreadWorkspacePath,
      projectsFromWorkspace,
    });
    const validation = await runtimeClient.validateWorkspace(workspaceRoot);
    if (!validation.valid) {
      setNotice(validation.error);
      return;
    }
    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    setRunCreateIssue(null);
    prepareForRunStart();
    dispatch({
      type: "run/requested",
      prompt,
      promptAttachments: runInputAttachments,
      promptTokens: draftPromptTokens,
      startedAtMs: Date.now(),
    });
    clearComposerValue();
    setComposerAttachments([]);
    setComposerTokens([]);
    setComposerTokenPlacements({});
    setComposerTokenLeadingTexts({});

    try {
      const runProjectId = isSameWorkspacePath(
        workspaceRoot,
        effectiveDefaultWorkspacePath,
      )
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : projectsFromWorkspace.find((project) =>
            isSameWorkspacePath(project.path, workspaceRoot),
          )?.id;
      const isNewThread = !runtimeState.activeThreadId;
      const nextThreadLabel = isNewThread
        ? composerThreadLabel(
            prompt,
            draftAttachments,
            draftComposerTokens,
          )
        : "";
      const attachedFiles = runInputAttachments
        .filter((attachment) => attachment.kind === "file" && attachment.path)
        .map((attachment) => ({
          path: attachment.path ?? "",
          displayName: attachment.displayName,
        }));
      const startRunRequest: StartRunRequest = {
        input: prompt,
        attachments: runInputAttachments,
        composerTokens: draftComposerTokens,
        threadId: runtimeState.activeThreadId || undefined,
        workspace: {
          workspaceRoot,
          readableRoots: workspaceRoot ? [workspaceRoot] : [],
          writableRoots: workspaceRoot ? [workspaceRoot] : [],
          outputRoots: workspaceRoot ? [workspaceRoot] : [],
          attachedFiles: attachedFiles.length > 0 ? attachedFiles : undefined,
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      if (runProjectId === DEFAULT_WORKSPACE_PROJECT_ID) {
        setRemovedProjectIds((current) =>
          current.filter(
            (projectId) => projectId !== DEFAULT_WORKSPACE_PROJECT_ID,
          ),
        );
      }
      dispatch({ type: "run/created", run });
      dispatch({
        type: "run/threadAdded",
        threadId: run.threadId,
        label: nextThreadLabel,
        runId: run.runId,
        projectId: runProjectId,
        workspaceRoot: workspaceRoot,
      });
      if (isNewThread) {
        startThreadTitlePoll(run.threadId);
      }
      void refreshThreads();
      localStorage.setItem("awaken:runId", run.runId);
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
      );
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create run";
      setRunCreateIssue(desktopOperationIssueFromError(error));
      restoreComposerValue(draftComposerValue);
      setComposerAttachments(draftAttachments);
      setComposerTokens(draftComposerTokens);
      setComposerTokenPlacements(draftComposerTokenPlacements);
      setComposerTokenLeadingTexts(draftComposerTokenLeadingTexts);
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleComposerSubmit(): Promise<void> {
    if (isBusy) {
      await handleCancel();
      requestComposerFocus();
      return;
    }
    await handleSubmit();
    requestComposerFocus();
  }

  async function handleSelectConversation(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }
    if (threadId === runtimeState.activeThreadId) {
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

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    syncProjectSelectionForWorkspace(thread?.workspaceRoot);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      await replayThreadTranscript(threadId, loadSeq);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to load thread",
      );
    }
  }

  function threadsForProject(projectId: string): ThreadItem[] {
    return runtimeState.threads.filter(
      (thread) =>
        projectIdForThread(
          thread,
          effectiveDefaultWorkspacePath,
        ) === projectId,
    );
  }

  function syncProjectSelectionForWorkspace(
    nextWorkspacePath?: string,
  ): void {
    if (!nextWorkspacePath) {
      return;
    }

    const nextProjectId = isSameWorkspacePath(
      nextWorkspacePath,
      effectiveDefaultWorkspacePath,
    )
      ? DEFAULT_WORKSPACE_PROJECT_ID
      : projectsFromWorkspace.find((project) =>
          isSameWorkspacePath(project.path, nextWorkspacePath),
        )?.id;
    if (!nextProjectId) {
      return;
    }
    setSelectedProjectId(nextProjectId);
    workspaceTargetPathRef.current = nextWorkspacePath;
    setWorkspacePath(nextWorkspacePath);
    setExpandedProjectIds((current) =>
      current.includes(nextProjectId)
        ? current
        : [...current, nextProjectId],
    );
  }

  async function refreshAfterThreadMutation(
    removedThreadIds: string[] = [],
  ): Promise<void> {
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
          ? await runtimeClient.detachProject(
              pending.backendProjectId,
              idempotencyKey,
            )
          : pending.action === "delete_history"
            ? await runtimeClient.deleteProjectThreads(
                pending.backendProjectId,
                idempotencyKey,
              )
            : await runtimeClient.trashProject(
                pending.backendProjectId,
                idempotencyKey,
              );
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      const operation = result.operation;
      const operationState = projectOperationReconciliationState(operation);
      if (operationState === "failed") {
        await Promise.all([
          refreshProjects(),
          loadThreadsIntoState(),
        ]);
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
            label: pending.label,
            wasExpanded: expandedProjectIds.includes(pending.projectId),
          }),
        );
        resetActiveThreadContext();
        setExpandedProjectIds((current) =>
          current.filter((id) => id !== pending.projectId),
        );
        setPendingProjectDelete(null);
        setNotice(
          `正在将“${pending.label}”移到废纸篓，Awaken 会自动完成。`,
        );
        const refreshResults = await Promise.allSettled([
          refreshProjects(),
          loadThreadsIntoState(),
        ]);
        for (const refreshResult of refreshResults) {
          if (refreshResult.status === "rejected") {
            console.warn(
              "project_operation_initial_refresh_failed",
              refreshResult.reason,
            );
          }
        }
        return;
      }

      if (pending.action !== "delete_history") {
        resetActiveThreadContext();
      }
      await Promise.all([refreshProjects(), loadThreadsIntoState()]);
      setExpandedProjectIds((current) =>
        pending.action === "delete_history"
          ? current
          : current.filter((id) => id !== pending.projectId),
      );
      setPendingProjectDelete(null);
      setNotice(
        pending.action === "detach"
          ? `已从 Awaken 移除“${pending.label}”，磁盘文件和历史均已保留。`
          : pending.action === "delete_history"
            ? `已删除“${pending.label}”的 Awaken 历史，磁盘文件未改变。`
            : `已将“${pending.label}”移到废纸篓。`,
      );
    } catch (error) {
      console.error("project_delete_ui_failed", error);
      setNotice("项目操作暂时失败，请稍后重试。");
      await Promise.all([
        refreshProjects().catch(() => []),
        refreshThreads(),
      ]);
    } finally {
      setProjectDeleteSubmitting(false);
    }
  }

  async function handleRenameThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread) {
      setNotice("会话不存在");
      return;
    }

    setRenamingThreadId(threadId);
    setRenameDraft(thread.label);
    setRenameDialogOpen(true);
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

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread || nextLabel === thread.label) {
      setRenameDialogOpen(false);
      setRenamingThreadId(null);
      setRenameDraft("");
      return;
    }

    setRenameSubmitting(true);
    try {
      const updatedThread = await runtimeClient.renameThread(
        threadId,
        nextLabel,
      );
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

  async function handleDeleteThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread) {
      setNotice("会话不存在");
      return;
    }
    if (!window.confirm(`确认删除会话 "${thread.label}"？`)) {
      return;
    }

    try {
      const result = await runtimeClient.deleteThread(threadId);
      if (!result.ok) {
        setNotice(result.message);
        return;
      }
      await refreshAfterThreadMutation([threadId]);
      setNotice("已删除会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除会话失败");
      await refreshThreads();
    }
  }

  function handleSelectProject(id: string): void {
    setActiveNavId(undefined);
    if (id.startsWith("thread:")) {
      void handleSelectConversation(id.slice("thread:".length));
      return;
    }

    const projectPath =
      id === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === id)?.path;
    if (
      shouldResetActiveThreadForWorkspaceChange(
        activeThreadWorkspacePath,
        projectPath,
      )
    ) {
      resetActiveThreadContext();
    }
    setSelectedProjectId(id);
    if (projectPath) {
      workspaceTargetPathRef.current = projectPath;
      setWorkspacePath(projectPath);
    }
  }

  function handleOpenCreateProject(): void {
    setPendingProjectMode("local");
    setCreateProjectDialogOpen(true);
    setNotice(null);
  }

  async function handleUseExistingFolder(): Promise<void> {
    if (pickingWorkspace) {
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);

    try {
      const result = await runtimeClient.pickWorkspace(
        workspacePath || defaultWorkspacePath,
      );
      if (result.canceled) {
        if (result.error) {
          setNotice(result.error);
        }
        return;
      }

      const validation = await runtimeClient.validateWorkspace(result.path);
      if (!validation.valid) {
        setNotice(validation.error);
        return;
      }

      if (
        !defaultProjectRemoved &&
        isSameWorkspacePath(result.path, effectiveDefaultWorkspacePath)
      ) {
        setNotice("该项目已存在");
        workspaceTargetPathRef.current = result.path;
        setWorkspacePath(result.path);
        setSelectedProjectId(DEFAULT_WORKSPACE_PROJECT_ID);
        setExpandedProjectIds((current) =>
          current.includes(DEFAULT_WORKSPACE_PROJECT_ID)
            ? current
            : [DEFAULT_WORKSPACE_PROJECT_ID, ...current],
        );
        setCreateProjectDialogOpen(false);
        return;
      }

      const existingProject = projectsFromWorkspace.find((project) =>
        isSameWorkspacePath(project.path, result.path),
      );
      if (existingProject) {
        setNotice("该项目已存在");
        workspaceTargetPathRef.current = result.path;
        setWorkspacePath(result.path);
        setSelectedProjectId(existingProject.id);
        setExpandedProjectIds((current) =>
          current.includes(existingProject.id)
            ? current
            : [...current, existingProject.id],
        );
        setCreateProjectDialogOpen(false);
        return;
      }

      const upsertResult = await runtimeClient.upsertProject(
        result.path,
        workspaceLabelForPath(result.path),
      );
      if (!upsertResult.ok) {
        setNotice(
          upsertResult.code === "PROJECT_OPERATION_CONFLICT"
            ? "该项目正在移除中"
            : upsertResult.message,
        );
        return;
      }

      const project = upsertResult.project;
      const nextProjectId = isSameWorkspacePath(
        project.workspaceRoot,
        effectiveDefaultWorkspacePath,
      )
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : project.projectId;
      await refreshProjects();
      if (
        shouldResetActiveThreadForWorkspaceChange(
          activeThreadWorkspacePath,
          result.path,
        )
      ) {
        resetActiveThreadContext();
      }
      workspaceTargetPathRef.current = result.path;
      setWorkspacePath(result.path);
      setSelectedProjectId(nextProjectId);
      setExpandedProjectIds((current) =>
        current.includes(nextProjectId)
          ? current
          : [...current, nextProjectId],
      );
      setCreateProjectDialogOpen(false);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleNewProjectSession(projectId: string): void {
    handleSelectProject(projectId);
    setExpandedProjectIds((current) =>
      current.includes(projectId) ? current : [...current, projectId],
    );
    handleNewTask();
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
      const terminalStatuses: RuntimeRunStatus[] = [
        "finished",
        "error",
        "cancelled",
      ];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to cancel run";
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
      await runtimeClient.answerUserQuestion(
        runtimeState.runId,
        activeUserQuestion.id,
        trimmed,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to answer user question",
      );
    } finally {
      setAnsweringUserQuestionId(null);
    }
  }

  function handleNewTask(): void {
    setActiveNavId(undefined);
    clearComposerValue();
    setComposerAttachments([]);
    setComposerTokens([]);
    setComposerTokenPlacements({});
    setComposerTokenLeadingTexts({});
    resetActiveThreadContext();
  }

  function handleSelectNav(id: string): void {
    setActiveNavId(id);
  }

  function handleToggleInspectorCollapsed(): void {
    setInspectorCollapsed((collapsed) => {
      const nextCollapsed = !collapsed;
      if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
        inspectorCollapsedBeforeResponsiveRef.current = nextCollapsed;
      }
      return nextCollapsed;
    });
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
      const nextAttachments = result.files.map(
        composerDraftAttachmentFromPickedFile,
      );
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
    const oversizedFile = files.find(
      (file) => file.size > MAX_COMPOSER_IMAGE_BYTES,
    );
    if (oversizedFile) {
      setNotice(
        `单张图片不能超过 ${MAX_COMPOSER_IMAGE_BYTES / (1024 * 1024)} MiB`,
      );
      return;
    }

    try {
      const startIndex = composerAttachments.length;
      const nextAttachments = await Promise.all(
        files.map((file, index) =>
          readComposerDraftImageAttachment(file, startIndex + index),
        ),
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

  function handleSidebarResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
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
      document.body.classList.remove(
        "is-layout-resizing",
        "is-sidebar-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-sidebar-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleInspectorResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = inspectorWidth;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth - (moveEvent.clientX - startX),
        MIN_INSPECTOR_WIDTH,
        MAX_INSPECTOR_WIDTH,
      );
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
      document.body.classList.remove(
        "is-layout-resizing",
        "is-inspector-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-inspector-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleSidebarResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? LAYOUT_STEP : -LAYOUT_STEP;
    setSidebarWidth((width) =>
      clampNumber(width + delta, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH),
    );
  }

  function handleInspectorResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowLeft" ? LAYOUT_STEP : -LAYOUT_STEP;
    setInspectorWidth((width) =>
      clampNumber(width + delta, MIN_INSPECTOR_WIDTH, MAX_INSPECTOR_WIDTH),
    );
  }

  function renderNarrativeItem(item: RunNarrativeItem, key: string) {
    if (item.kind === "thinking") {
      return <ThoughtTrace key={key} text={item.text} status={item.status} />;
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
            elapsedMs={item.elapsedMs}
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
    if (item.kind === "compactBoundary") {
      return (
        <div
          key={key}
          className={`compact-boundary compact-boundary-${item.status}`}
          role="separator"
          aria-label={
            item.detail ? `${item.title}: ${item.detail}` : item.title
          }
        >
          <span className="compact-boundary-line" aria-hidden="true" />
          <span className="compact-boundary-copy">
            <span className="compact-boundary-title">{item.title}</span>
            {item.detail ? (
              <span className="compact-boundary-detail">{item.detail}</span>
            ) : null}
          </span>
          <span className="compact-boundary-line" aria-hidden="true" />
        </div>
      );
    }
    if (item.kind === "exploring") {
      const actions = item.actions.map((action) => {
        const id = action.id.startsWith("explore:")
          ? action.id.slice("explore:".length)
          : rawToolId(action.id);
        const tool = runtimeState.toolResults.find(
          (record) => record.id === id,
        );
        return {
          ...action,
          detail: toolTraceDetail(tool, action.detail),
        };
      });
      return <ToolTraceGroup key={key} actions={actions} />;
    }
    if (item.kind === "tool") {
      const tool = runtimeState.toolResults.find(
        (record) => record.id === rawToolId(item.id),
      );
      return (
        <ToolTraceLine
          key={key}
          title={item.title}
          detail={toolTraceDetail(tool, item.detail)}
          status={item.status}
        />
      );
    }
    if (item.kind === "deliverables") {
      const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
      return (
        <DeliverableList
          key={key}
          terminalStatus={item.terminalStatus}
          items={item.files.map((file) => ({
            id: file.artifactId,
            title: file.title,
            extension: file.extension,
            fileType: file.fileType,
            opening: openingArtifactId === file.artifactId,
            disabled: !threadId,
            onOpen: threadId
              ? async () => {
                  setOpeningArtifactId(file.artifactId);
                  try {
                    const result = await runtimeClient.openArtifact({
                      threadId,
                      runId: file.runId,
                      artifactId: file.artifactId,
                    });
                    if (!result.ok) {
                      setNotice(result.message);
                    }
                  } finally {
                    setOpeningArtifactId((current) =>
                      current === file.artifactId ? null : current,
                    );
                  }
                }
              : undefined,
            onReveal: threadId
              ? async () => {
                  const result = await runtimeClient.revealArtifact({
                    threadId,
                    runId: file.runId,
                    artifactId: file.artifactId,
                  });
                  if (!result.ok) {
                    setNotice(result.message);
                  }
                }
              : undefined,
          }))}
        />
      );
    }
    if (item.kind === "fileChange") {
      return null;
    }
    return null;
  }

  function renderTurnBodySegment(
    turnId: string,
    segment: NarrativeTurnBodySegment,
  ) {
    if (segment.kind === "items") {
      return segment.items.map((item, itemIndex) =>
        renderNarrativeItem(
          item,
          `${turnId}:${segment.id}:${item.id}:${itemIndex}`,
        ),
      );
    }

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
              {segment.title}
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
            {segment.items.map((item, itemIndex) =>
              renderNarrativeItem(
                item,
                `${turnId}:${segment.id}:${item.id}:${itemIndex}`,
              ),
            )}
          </div>
        </Disclosure>
      </div>,
    ];
  }

  function toolTraceDetail(
    tool: (typeof runtimeState.toolResults)[number] | undefined,
    fallback?: string,
  ): string | undefined {
    if (!tool) {
      return fallback;
    }
    const output = firstContentLine(tool.commandOutput);
    const result = firstContentLine(tool.content);
    if (tool.status && tool.status !== "active") {
      return result || output || fallback;
    }
    return output || fallback || result;
  }

  const platform = runtimeClient.platform as PlatformKind;
  const composerBarAttachments = composerAttachmentViews(composerAttachments);
  const layoutStyle = {
    "--aw-sidebar-width": `${sidebarWidth}px`,
    "--aw-inspector-width": `${inspectorWidth}px`,
  } as CSSProperties;

  function renderUserQuestionPrompt() {
    if (!activeUserQuestion) {
      return null;
    }
    const answering = answeringUserQuestionId === activeUserQuestion.id;
    return (
      <UserQuestionPrompt
        question={activeUserQuestion}
        disabled={answering}
        onSubmit={(answer) => {
          void handleAnswerUserQuestion(answer);
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
            {panel.actions && panel.actions.length > 0 ? (
              <div className="composer-runtime-panel-actions">
                {panel.actions.includes("retry") ? (
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      void handleComposerSubmit();
                    }}
                  >
                    重试
                  </Button>
                ) : null}
                {panel.actions.includes("relaunch") ? (
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      setNotice("正在重新启动 Awaken…");
                      void runtimeClient.relaunchDesktop().catch(() => {
                        setNotice("Awaken 重新启动失败，请再试一次。");
                      });
                    }}
                  >
                    重新启动 Awaken
                  </Button>
                ) : null}
                {panel.actions.includes("reenter") ? (
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      setModelSettingsInitialModelSection("configured");
                      setModelSettingsOpen(true);
                    }}
                  >
                    在 Awaken 中恢复
                  </Button>
                ) : null}
              </div>
            ) : null}
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
        : "将项目移到废纸篓";
  const projectDeleteDialogBody = !pendingProjectDelete
    ? ""
    : pendingProjectDelete.action === "detach"
      ? `“${pendingProjectDelete.label}”会从侧栏移除。${pendingProjectDelete.threadCount} 条历史会话和磁盘文件都会保留。`
      : pendingProjectDelete.action === "delete_history"
        ? `将删除“${pendingProjectDelete.label}”的 ${pendingProjectDelete.threadCount} 条 Awaken 历史会话。项目和磁盘目录都会保留；运行中的任务会先自动停止。`
        : `“${pendingProjectDelete.label}”的磁盘目录将进入系统废纸篓。只有废纸篓操作成功后，Awaken 才会删除对应历史；失败时项目、历史和文件都会保持原样。`;
  const projectDeleteConfirmLabel =
    pendingProjectDelete?.action === "detach"
      ? "确认移除"
      : pendingProjectDelete?.action === "delete_history"
        ? "确认删除历史"
        : "移到废纸篓";

  return (
    <>
      <div
        className={`desktop-root desktop-root-electron ${inspectorPresentation.rootClassName}`}
        style={layoutStyle}
      >
        <AppShell
          platform={platform}
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
            inspectorPresentation.available
              ? {
                  role: "separator",
                  "aria-label": "Resize inspector",
                  "aria-orientation": "vertical",
                  "aria-valuemin": MIN_INSPECTOR_WIDTH,
                  "aria-valuemax": MAX_INSPECTOR_WIDTH,
                  "aria-valuenow": inspectorWidth,
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
              onToggleSidebar={() =>
                setSidebarCollapsed((collapsed) => !collapsed)
              }
              onNewTask={handleNewTask}
              right={
                <div className="workbench-chrome-status">
                  {topbarStatusLabel ? (
                    <span className="workbench-chrome-state">
                      {topbarStatusLabel}
                    </span>
                  ) : null}
                  {inspectorPresentation.available ? (
                    <button
                      type="button"
                      className="workbench-chrome-icon workbench-chrome-panel-toggle"
                      aria-label={
                        inspectorCollapsed
                          ? "Expand inspector"
                          : "Collapse inspector"
                      }
                      onClick={handleToggleInspectorCollapsed}
                    >
                      {inspectorCollapsed ? (
                        <PanelRightOpen size={14} strokeWidth={1.9} />
                      ) : (
                        <PanelRightClose size={14} strokeWidth={1.9} />
                      )}
                    </button>
                  ) : null}
                </div>
              }
            />
          }
          sidebar={
            <WorkbenchSidebarFrame>
              <Sidebar
                title="Awaken"
                showBrand={false}
                navItems={navItems}
                activeNavId={activeNavId}
                conversations={[]}
                projects={repositoryProjects}
                activeProjectId={activeProjectId}
                expandedIds={expandedProjectIds}
                defaultExpandedIds={[DEFAULT_WORKSPACE_PROJECT_ID]}
                profileName="Nolan"
                profileAvatarFallback="AW"
                newTaskLabel="New Agent"
                newTaskShortcut=""
                conversationSectionLabel=""
                projectSectionLabel="Projects"
                projectSectionActions={
                  <span className="sidebar-section-actions">
                    <SidebarSectionAction
                      label="Filter projects"
                      onClick={() =>
                        setNotice("Project filters are not available yet.")
                      }
                    >
                      <ListFilter size={13} strokeWidth={1.85} />
                    </SidebarSectionAction>
                    <SidebarSectionAction
                      label="Add folder"
                      onClick={handleOpenCreateProject}
                    >
                      <FolderPlus size={13} strokeWidth={1.85} />
                    </SidebarSectionAction>
                  </span>
                }
                showProjectCount={false}
                platform={platform}
                onSelectNav={handleSelectNav}
                onSelectProject={handleSelectProject}
                onExpandedChange={setExpandedProjectIds}
                onNewTask={handleNewTask}
                onProfileClick={() => setProfilePopoverOpen(true)}
                profileButtonRef={profileButtonRef}
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
              ) : (
                <WorkbenchShell
                  inspectorCollapsed={inspectorCollapsed}
                  stage={
                    <RunStage
                      scrollRef={
                        showCenterComposer
                          ? undefined
                          : setRunStageScrollElement
                      }
                      onScroll={
                        showCenterComposer ? undefined : handleRunStageScroll
                      }
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
                                    marker.id === activeWorkbenchTurnId
                                      ? "true"
                                      : undefined
                                  }
                                  aria-label={`跳转到第 ${marker.index + 1} 条消息`}
                                  title={`第 ${marker.index + 1} 条消息`}
                                  onClick={() =>
                                    handleTurnMarkerClick(marker.id)
                                  }
                                >
                                  <span
                                    className="turn-marker-bar"
                                    aria-hidden="true"
                                  />
                                </button>
                              ))}
                            </nav>
                          ) : null}
                          {returnToLatestVisible ? (
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
                                <ChevronDown
                                  size={16}
                                  strokeWidth={2.1}
                                  aria-hidden="true"
                                />
                              </button>
                            </div>
                          ) : null}
                        </>
                      }
                      composer={
                        showCenterComposer ? undefined : (
                          <ComposerDock ref={setComposerDockElement}>
                            <div className="composer-dock-stack">
                              {renderComposerRuntimePanels()}
                              {renderUserQuestionPrompt()}
                              <ComposerBar
                                value={composerValue}
                                onChange={setComposerValue}
                                onStructuredInputChange={
                                  handleComposerStructuredInputChange
                                }
                                onSubmit={() => {
                                  void handleComposerSubmit();
                                }}
                                onAddContext={handleAddContext}
                                placeholder=""
                                disabled={composerBarState.disabled}
                                isReady={composerBarState.isReady}
                                submitMode={composerBarState.submitMode}
                                submitDisabled={composerBarState.submitDisabled}
                                variant="dock"
                                metaItems={composerMetaItems}
                                focusRequest={composerFocusRequest}
                                attachments={composerBarAttachments}
                                tokens={composerBarTokens}
                                slashPalette={composerBarSlashPalette}
                                onInputKeyDown={handleComposerInputKeyDown}
                                onRemoveAttachment={
                                  handleRemoveComposerAttachment
                                }
                                onRemoveToken={handleRemoveComposerToken}
                                onPasteImages={(files) => {
                                  void handlePasteImages(files);
                                }}
                              />
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
                          <ComposerBar
                            value={composerValue}
                            onChange={setComposerValue}
                            onStructuredInputChange={
                              handleComposerStructuredInputChange
                            }
                            onSubmit={() => {
                              void handleComposerSubmit();
                            }}
                            onAddContext={handleAddContext}
                            placeholder=""
                            disabled={composerBarState.disabled}
                            isReady={composerBarState.isReady}
                            submitMode={composerBarState.submitMode}
                            submitDisabled={composerBarState.submitDisabled}
                            variant="center"
                            metaItems={composerMetaItems}
                            focusRequest={composerFocusRequest}
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
                            renderNarrativeItem(
                              item,
                              `lead:${item.id}:${index}`,
                            ),
                          )}
                          {narrativeSections.turns.map((turn, index) => {
                            const isLatestPrompt = turn.id === latestPromptId;
                            const isActiveTurn =
                              turn.id === activeWorkbenchTurnId;
                            const turnBodySegments =
                              projectNarrativeTurnBodySegments({
                                turnId: turn.id,
                                items: turn.items,
                                executionProcessOpen:
                                  executionProcessOpenByTurnId[turn.id] ===
                                  true,
                              });
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
                                  isLatestPrompt
                                    ? " execution-turn-section-latest"
                                    : ""
                                }${
                                  isActiveTurn
                                    ? " execution-turn-section-active"
                                    : ""
                                }`}
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
                                      tokens={promptTokenViews(
                                        turn.prompt.tokens,
                                      )}
                                      copyText={turn.prompt.text}
                                      attachments={promptAttachmentViews(
                                        turn.prompt.attachments,
                                      )}
                                      expanded={
                                        promptBubbleModeByTurnId[turn.id] ===
                                        "expanded"
                                      }
                                      onExpandedChange={(expanded) => {
                                        setPromptBubbleModeByTurnId(
                                          (current) => ({
                                            ...current,
                                            [turn.id]: expanded
                                              ? "expanded"
                                              : "collapsed",
                                          }),
                                        );
                                      }}
                                    />
                                  </div>
                                  <div className="execution-turn-body">
                                    {turnBodySegments.flatMap((segment) =>
                                      renderTurnBodySegment(turn.id, segment),
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
                    <LightInspectorRail
                      title="On Awaken"
                      items={inspectorPlaceholderItems}
                      activeId="right-sidebar-placeholder"
                    />
                  }
                />
              )}
            </div>
          }
        />
      </div>
      <Dialog
        open={createProjectDialogOpen}
        title="Create project"
        onOpenChange={setCreateProjectDialogOpen}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              loading={pickingWorkspace}
              onClick={() => {
                void handleUseExistingFolder();
              }}
            >
              Use existing folder
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={pickingWorkspace}
              onClick={() => {
                void handleUseExistingFolder();
              }}
            >
              Next
            </Button>
          </>
        }
      >
        <div className="create-project-dialog-body">
          <SelectableCard
            selected={pendingProjectMode === "local"}
            icon={<Monitor size={18} strokeWidth={1.9} />}
            title="Local"
            description="Use a folder on this Mac as an Awaken project."
            onSelect={() => setPendingProjectMode("local")}
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
      />
      <ProfileBalancePopover
        open={profilePopoverOpen}
        anchorRef={profileButtonRef}
        runtimeClient={runtimeClient}
        onOpenSettings={() => {
          setProfilePopoverOpen(false);
          setModelSettingsInitialModelSection("configured");
          setModelSettingsOpen(true);
        }}
        onClose={() => setProfilePopoverOpen(false)}
      />
    </>
  );
}
