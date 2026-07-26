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
  AwakenContextUsagePanel as ContextUsagePanel,
  AwakenDisclosure as Disclosure,
  AwakenDialog as Dialog,
  AwakenDeliverableList as DeliverableList,
  AwakenExecutionFeed as ExecutionFeed,
  AwakenInspectorRail as InspectorRail,
  AwakenProjectFoldersEditor as ProjectFoldersEditor,
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
  type AwakenContextUsagePanelProps,
  type NavItem,
  type PlatformKind,
  type ProjectFolderRow,
  type ProjectHoverCard,
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
  MemoryDreamTask,
  RuntimeInfo,
  RuntimeProject,
  RunInputAttachment,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  RuntimeThread,
  StartRunRequest,
  UpsertProjectResult,
} from "../shared/runtime";
import type { AguiEvent } from "../shared/agui";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  type ThreadItem,
  type RuntimeAction,
  type UserQuestionItem,
} from "./runtime/reducer";
import type {
  ACPDisplayStatus,
  PromptNarrativeToken,
  RunNarrativeItem,
} from "./run-narrative";
import type { ToolDisplayState } from "../shared/toolStatus";
import { projectPetActivity } from "./pets/pet-activity-projector";
import {
  projectNarrativeTurnBodySegments,
  contextUsagePanelBottomOffset,
  projectVisibleNarrativeItems,
  shouldShowAssistantAnswerFooter,
  shouldUseCenterComposer,
  turnMarkerRailHeight,
  turnMarkerRailDisplay,
  type NarrativeTurnBodySegment,
} from "./workbenchPresentation";
import { selectAllInFocusedEditor } from "./desktop-selection";
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
import { ExternalActionGate } from "./ExternalActionGate";
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
  /** 仅 trash 用：废纸篓是文件夹粒度，这里是本次要移走的那个目录。 */
  folderPath?: string;
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
        .filter((token) => remainingTokenIds.has(token.id) && token.leadingText)
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
  if (thread.runStatus === "running" || thread.runStatus === "cancelling") {
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

  const selectedProject =
    selectedProjectId === undefined ||
    selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
      ? undefined
      : projectsFromWorkspace.find(
          (project) => project.id === selectedProjectId,
        );

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
      [...args.attachedPaths, ...accepted].some((taken) =>
        isSameWorkspacePath(taken, candidate),
      )
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
        project.folders.some((folder) =>
          isSameWorkspacePath(folder.path, args.folderPath),
        ),
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
        candidate.folders.some((folder) =>
          isSameWorkspacePath(folder.path, root),
        ),
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
      ?.folders.some((folder) =>
        isSameWorkspacePath(folder.path, workspaceRoot),
      ) === true;
  // 没有根就只能靠选中项来定身份，那正是零文件夹项目的情形。
  const projectId = !workspaceRoot
    ? selectedProjectId
    : selectedHoldsRoot
      ? selectedProjectId
      : isSameWorkspacePath(workspaceRoot, effectiveDefaultWorkspacePath)
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : workspaceProjectForFolderPath(projectsFromWorkspace, workspaceRoot)
            ?.id;
  if (!projectId) {
    return {};
  }
  if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
    return {
      projectId,
      projectName: defaultWorkspacePath
        ? workspaceLabelForPath(defaultWorkspacePath)
        : "awaken",
    };
  }
  return {
    projectId,
    projectName: projectsFromWorkspace.find(
      (project) => project.id === projectId,
    )?.label,
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
  const backendById = new Map(
    projects.map((project) => [project.id, project.backendProjectId]),
  );
  const backendIds = ids.map((id) =>
    id === DEFAULT_WORKSPACE_PROJECT_ID
      ? defaultBackendProjectId
      : backendById.get(id),
  );
  return backendIds.every((id): id is string => Boolean(id))
    ? backendIds
    : undefined;
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

type RuntimeContextWindow = ReturnType<
  typeof createInitialRuntimeState
>["inspectorSummary"]["contextWindow"];

type RuntimeModelUsage = NonNullable<
  NonNullable<RuntimeContextWindow>["latestModelUsage"]
>;

type RuntimeModelIdentity = {
  id?: string;
  providerId?: string;
  credentialRevision?: number;
  modelId?: string;
  protocol?: string;
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

function providerCurrentContextTokens(
  usage: RuntimeModelUsage | undefined,
): number | undefined {
  if (
    !usage ||
    usage.purpose !== "agent_turn" ||
    usage.status !== "succeeded" ||
    !usage.usageComplete
  ) {
    return undefined;
  }

  const input = normalizedTokenCount(usage.inputTokens);
  const output = normalizedTokenCount(usage.outputTokens);
  const total = normalizedTokenCount(usage.totalTokens);
  if (
    total !== undefined &&
    (input === undefined || total >= input) &&
    (output === undefined || total >= output)
  ) {
    return total;
  }
  if (input !== undefined) {
    return output !== undefined ? safeTokenSum(input, output) : input;
  }

  const cached = normalizedTokenCount(usage.cachedInputTokens);
  const uncached = normalizedTokenCount(usage.uncachedInputTokens);
  if (cached === undefined || uncached === undefined) {
    return undefined;
  }
  const cacheWrite = normalizedTokenCount(usage.cacheWriteTokens) ?? 0;
  return safeTokenSum(cached, uncached, cacheWrite, output ?? 0);
}

function usageMatchesSnapshot(
  usage: RuntimeModelUsage | undefined,
  snapshot: NonNullable<RuntimeContextWindow>["snapshot"],
  currentModel?: RuntimeModelIdentity,
): boolean {
  if (!usage || !snapshot) {
    return true;
  }
  if (
    currentModel &&
    ((usage.profileId &&
      currentModel.id &&
      usage.profileId !== currentModel.id) ||
      (usage.providerId &&
        currentModel.providerId &&
        usage.providerId !== currentModel.providerId) ||
      (usage.credentialRevision !== undefined &&
        currentModel.credentialRevision !== undefined &&
        usage.credentialRevision !== currentModel.credentialRevision) ||
      (usage.modelId &&
        currentModel.modelId &&
        usage.modelId !== currentModel.modelId) ||
      (usage.protocolFamily &&
        currentModel.protocol &&
        usage.protocolFamily !== currentModel.protocol))
  ) {
    return false;
  }
  if (usage.modelId && snapshot.modelId && usage.modelId !== snapshot.modelId) {
    return false;
  }
  if (
    usage.contextEpochId &&
    snapshot.contextEpochId &&
    usage.contextEpochId !== snapshot.contextEpochId &&
    snapshot.contextEpochBoundaryReason !== undefined &&
    snapshot.contextEpochBoundaryReason !== "tool_schema_changed"
  ) {
    return false;
  }
  return !(
    usage.protocolFamily &&
    snapshot.protocolFamily &&
    usage.protocolFamily !== snapshot.protocolFamily
  );
}

function currentContextTokens(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): number | undefined {
  const usage = contextWindow?.latestModelUsage;
  const providerActual = usageMatchesSnapshot(
    usage,
    contextWindow?.snapshot,
    currentModel,
  )
    ? providerCurrentContextTokens(usage)
    : undefined;
  return (
    providerActual ??
    normalizedTokenCount(contextWindow?.actualTokens) ??
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
      ? Math.max(
          0,
          contextWindow.modelWindow - (contextWindow.outputReserve ?? 0),
        )
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

function proportionallyAllocateTokens(
  values: number[],
  target: number,
): number[] {
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
    .sort(
      (left, right) =>
        right.fraction - left.fraction || left.index - right.index,
    );
  for (let index = 0; remainder > 0; index += 1, remainder -= 1) {
    allocated[byFraction[index % byFraction.length].index] += 1;
  }
  return allocated;
}

function reconcileContributionTokens(
  values: number[],
  currentTokens: number,
): number[] {
  const sourceTotal = values.reduce((sum, value) => sum + value, 0);
  const delta = currentTokens - sourceTotal;
  if (delta === 0) {
    return values;
  }

  const conversationIndex = CONTEXT_PANEL_CONTRIBUTIONS.findIndex(
    (contribution) => contribution.id === "conversation",
  );
  const reconciled = values.slice();
  if (delta > 0) {
    reconciled[conversationIndex] += delta;
    return reconciled;
  }
  if (reconciled[conversationIndex] + delta >= 0) {
    reconciled[conversationIndex] += delta;
    return reconciled;
  }
  return proportionallyAllocateTokens(values, currentTokens);
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
  const outputReserveTokens =
    snapshot?.outputReserveTokens ?? contextWindow.outputReserve ?? 0;
  const modelWindowTokens =
    snapshot?.modelWindowTokens ?? contextWindow.modelWindow;
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
    ((snapshot?.schemaVersion === 4 &&
      snapshot.estimatorId === "awaken_leaf_v4") ||
      (snapshot?.schemaVersion === 3 &&
        snapshot.estimatorId === "awaken_leaf_v3")) &&
    rawContributions.every((contribution) =>
      supportedKinds.has(contribution.kind),
    );
  const tokensByKind = new Map<string, number>();
  if (hasRetainedContributionContract) {
    for (const contribution of rawContributions) {
      tokensByKind.set(
        contribution.kind,
        (tokensByKind.get(contribution.kind) ?? 0) +
          contribution.estimatedTokens,
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
  const reconciledTokens = hasCompleteBreakdown
    ? reconcileContributionTokens(
        CONTEXT_PANEL_CONTRIBUTIONS.map(
          (contribution) => tokensByKind.get(contribution.id) ?? 0,
        ),
        currentInputTokens,
      )
    : [];
  return {
    estimatedInputTokens: currentInputTokens,
    effectiveInputWindowTokens,
    modelWindowTokens,
    outputReserveTokens,
    autoCompactThresholdTokens:
      snapshot?.autoCompactThresholdTokens ??
      contextWindow.autoCompactThreshold ??
      0,
    contributions: hasCompleteBreakdown
      ? CONTEXT_PANEL_CONTRIBUTIONS.map((contribution, index) => ({
          ...contribution,
          estimatedTokens: reconciledTokens[index],
        }))
      : [],
    latestModelUsage: contextWindow.latestModelUsage
      ? {
          status: contextWindow.latestModelUsage.status,
          cacheMode: contextWindow.latestModelUsage.cacheMode,
          cacheKeyApplied: contextWindow.latestModelUsage.cacheKeyApplied,
          cachedInputTokens: contextWindow.latestModelUsage.cachedInputTokens,
          uncachedInputTokens:
            contextWindow.latestModelUsage.uncachedInputTokens,
          cacheWriteTokens: contextWindow.latestModelUsage.cacheWriteTokens,
          inputTokens: contextWindow.latestModelUsage.inputTokens,
          outputTokens: contextWindow.latestModelUsage.outputTokens,
          reasoningTokens: contextWindow.latestModelUsage.reasoningTokens,
          totalTokens: contextWindow.latestModelUsage.totalTokens,
          calculatedCostUsd: contextWindow.latestModelUsage.calculatedCostUsd,
          currency: contextWindow.latestModelUsage.currency,
          invalidationReason: contextWindow.latestModelUsage.invalidationReason,
          purpose: contextWindow.latestModelUsage.purpose,
          errorCode: contextWindow.latestModelUsage.errorCode,
        }
      : null,
  };
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
const DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY =
  "awaken:memory:dreamReceiptThreadAssignments";
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

function dreamTaskReceiptEvent(task: MemoryDreamTask): AguiEvent | undefined {
  if (
    !["completed", "failed", "cancelled", "interrupted"].includes(task.status)
  ) {
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
  const activeThreadIdRef = useRef(runtimeState.activeThreadId);
  activeThreadIdRef.current = runtimeState.activeThreadId;
  const dreamReceiptThreadAssignmentsRef =
    useRef<DreamReceiptThreadAssignments>(
      parseDreamReceiptThreadAssignments(
        readStoredString(DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY),
      ),
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
  const [resolvingExternalInteractionId, setResolvingExternalInteractionId] =
    useState<string | null>(null);
  const [openingArtifactId, setOpeningArtifactId] = useState<string | null>(
    null,
  );
  const [answeringUserQuestionId, setAnsweringUserQuestionId] = useState<
    string | null
  >(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [modelProfiles, setModelProfiles] = useState<
    RuntimeModelProfileSummary[]
  >([]);
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [
    modelSettingsInitialModelSection,
    setModelSettingsInitialModelSection,
  ] = useState<"configure" | "configured">("configure");
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
  const [appearanceMessage, setAppearanceMessage] = useState<string | null>(
    null,
  );
  const [profilePopoverOpen, setProfilePopoverOpen] = useState(false);
  const profileButtonRef = useRef<HTMLDivElement>(null);
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
  const [pendingProjectMode, setPendingProjectMode] =
    useState<"local">("local");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  const [draftProjectFolders, setDraftProjectFolders] = useState<string[]>([]);
  const [draftPrimaryFolder, setDraftPrimaryFolder] = useState<string>("");
  const [draftProjectName, setDraftProjectName] = useState<string>("");
  const [createProjectSubmitting, setCreateProjectSubmitting] = useState(false);
  const [editingProjectId, setEditingProjectId] = useState<string | null>(null);
  const [projectFolderBusyPath, setProjectFolderBusyPath] = useState<
    string | undefined
  >(undefined);
  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<
    WorkspaceProject[]
  >([]);
  const [selectedProjectId, setSelectedProjectId] = useState<
    string | undefined
  >(undefined);
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
        writeStoredValue(
          DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY,
          JSON.stringify(assignments),
        );
        const event = dreamTaskReceiptEvent(task);
        if (event)
          dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
      }),
    [runtimeClient, selectedProjectId],
  );
  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>([]);
  const [projectOrderIds, setProjectOrderIds] = useState<string[]>([]);
  const [projectReorderSubmitting, setProjectReorderSubmitting] =
    useState(false);
  const projectReorderSubmittingRef = useRef(false);
  const [removedProjectIds, setRemovedProjectIds] = useState<string[]>([]);
  const [defaultBackendProjectId, setDefaultBackendProjectId] = useState<
    string | undefined
  >(undefined);
  const [defaultProjectFolders, setDefaultProjectFolders] = useState<
    WorkspaceProjectFolder[]
  >([]);
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
  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem
        ? [runtimeState.narrative.activeItem]
        : []),
    ];
    return projectVisibleNarrativeItems({ items: narrativeItems, isBusy });
  }, [isBusy, runtimeState.narrative]);
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
    const configuredDefaultWorkspacePath =
      runtimeInfo?.defaultWorkspace ?? workspacePath;
    const defaultProject = projects.find((project) =>
      isSameWorkspacePath(
        project.workspaceRoot,
        configuredDefaultWorkspacePath,
      ),
    );
    setDefaultBackendProjectId(defaultProject?.projectId);
    const nextDefaultFolders =
      defaultProject?.folders.map<WorkspaceProjectFolder>((folder) => ({
        path: folder.path,
        role: folder.role,
        isPrimary: folder.isPrimary,
      })) ?? [];
    setDefaultProjectFolders((current) =>
      sameWorkspaceProjectFolders(current, nextDefaultFolders)
        ? current
        : nextDefaultFolders,
    );
    setRemovedProjectIds(
      defaultProject && defaultProject.state !== "active"
        ? [DEFAULT_WORKSPACE_PROJECT_ID]
        : [],
    );

    const nextProjectOrderIds = projects
      .filter((project) => project.state === "active")
      .map((project) =>
        isSameWorkspacePath(
          project.workspaceRoot,
          configuredDefaultWorkspacePath,
        )
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
          !isSameWorkspacePath(
            project.workspaceRoot,
            configuredDefaultWorkspacePath,
          ),
      )
      .map<WorkspaceProject>((project) => ({
        id: project.projectId,
        backendProjectId: project.projectId,
        label:
          project.displayName || workspaceLabelForPath(project.workspaceRoot),
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
            return canReconcileTrackedTrashOperation(
              state,
              project,
              tracked.folderPath,
            );
          });

          if (reconciled.length > 0) {
            applyRuntimeProjects(projects);
            const reconciledIds = new Set(
              reconciled.map(({ tracked }) => tracked.operationId),
            );
            const failed = reconciled.filter(({ state }) => state === "failed");
            if (failed.length > 0) {
              setExpandedProjectIds((current) => {
                const next = [...current];
                for (const { tracked } of failed) {
                  if (
                    tracked.wasExpanded &&
                    !next.includes(tracked.projectId)
                  ) {
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
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to resolve external interaction",
      );
    } finally {
      setResolvingExternalInteractionId(null);
    }
  }

  async function resolveActiveACPPermission(
    optionId: string,
    cancelled = false,
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
      await runtimeClient.resolveACPPermission(
        runtimeState.runId,
        activeExternalInteraction.id,
        optionId,
        cancelled,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to resolve ACP permission",
      );
    } finally {
      setResolvingExternalInteractionId(null);
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
  // 项目工作区 shows a true empty state until the user opens/adds a project.
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
  const activeExternalInteraction = runtimeState.externalInteractions[0];
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
  }, [
    defaultProjectRemoved,
    hasDefaultWorkspaceProject,
    projectsFromWorkspace,
  ]);

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
    (workspaceLabelPath ? workspaceLabelForPath(workspaceLabelPath) : "awaken");
  const contextMeta = contextUsageMeta(
    runtimeState.inspectorSummary.contextWindow,
    selectedModelProfile,
  );
  const contextPanelData = contextUsagePanelData(
    runtimeState.inspectorSummary.contextWindow,
    selectedModelProfile,
  );
  const contextUsagePanelId = "awaken-context-usage-panel";

  function closeContextUsagePanel(returnFocus: boolean): void {
    setContextUsagePanelOpen(false);
    if (returnFocus && typeof window !== "undefined") {
      window.requestAnimationFrame(() =>
        contextUsageTriggerRef.current?.focus(),
      );
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
    ...(contextMeta
      ? [
          {
            id: "context",
            ...contextMeta,
            onClick: () => setContextUsagePanelOpen((current) => !current),
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
  const unorderedRepositoryProjects: ProjectItem[] = [
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
            hoverCard: projectHoverCard(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
              defaultProjectThreads.length,
            ),
            children:
              defaultProjectThreads.length > 0
                ? defaultProjectThreads
                : undefined,
          },
        ]),
    ...projectsFromWorkspace
      .filter(
        (project) =>
          !isSameWorkspacePath(project.path, effectiveDefaultWorkspacePath),
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
          hoverCard: projectHoverCard(
            project.id,
            project.label,
            threads.length,
          ),
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
  const activeProjectId = runtimeState.activeThreadId
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

    syncProjectSelectionForWorkspace(transcript.thread.workspace.workspaceRoot);
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
          if (event)
            dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
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
    const placement =
      composerTokenPlacementForSlashTrigger(currentSlashTrigger);
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
      current[token.id] === placement
        ? current
        : { ...current, [token.id]: placement },
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
      setComposerValue((current) =>
        clearSlashTriggerFromComposerValue(current),
      );
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
      selectableActions.findIndex(
        (action) => action.id === slashActiveActionId,
      ),
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

  async function handleSubmitWithPrompt(options?: {
    prompt?: string;
    forceNewThread?: boolean;
  }): Promise<void> {
    if (!desktopReady) {
      setNotice(
        desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting",
      );
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
      selectedProjectId,
      defaultWorkspacePath,
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
      promptAttachments: forceNewThread ? [] : runInputAttachments,
      promptTokens: forceNewThread ? [] : draftPromptTokens,
      startedAtMs: Date.now(),
    });
    clearComposerValue();
    setComposerAttachments([]);
    setComposerTokens([]);
    setComposerTokenPlacements({});
    setComposerTokenLeadingTexts({});

    try {
      const { projectId: runProjectId, projectName: runProjectName } =
        resolveRunProjectForRun({
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
        threadId: forceNewThread
          ? undefined
          : runtimeState.activeThreadId || undefined,
        workspace: {
          // projectId is what lets Core place a folder-less run in that
          // project's own session workspace rather than guess an identity from
          // the path. With no folder there is no root to declare either.
          projectId: runProjectId
            ? backendProjectIdFor(runProjectId)
            : undefined,
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
    await handleSubmitWithPrompt();
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
        projectIdForThread(thread, effectiveDefaultWorkspacePath) === projectId,
    );
  }

  function syncProjectSelectionForWorkspace(nextWorkspacePath?: string): void {
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
      current.includes(nextProjectId) ? current : [...current, nextProjectId],
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
        if (
          isSameWorkspacePath(activeThreadWorkspacePath, pending.folderPath)
        ) {
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
            console.warn(
              "project_operation_initial_refresh_failed",
              refreshResult.reason,
            );
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
        pending.action === "detach"
          ? current.filter((id) => id !== pending.projectId)
          : current,
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
        error instanceof Error
          ? error.message
          : "项目顺序暂时未保存，已恢复原顺序，请稍后重试。",
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
    return (
      projectsFromWorkspace.find((project) => project.id === projectId)
        ?.folders ?? []
    );
  }

  /** 目标项目之外，所有已经占用了某个目录的项目。 */
  /** 选一个或多个目录，逐个通过 Core 校验后再判定哪些能加入。 */
  async function pickProjectFolders(
    excludedPaths: string[],
  ): Promise<{ paths: string[]; skipped: string[] }> {
    const result = await runtimeClient.pickProjectFolders(
      workspacePath || defaultWorkspacePath,
    );
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
        skipped.push(
          `“${workspaceLabelForPath(candidate)}”：${validation.error}`,
        );
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
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleRemoveDraftProjectFolder(path: string): void {
    setDraftProjectFolders((current) => {
      const next = current.filter(
        (candidate) => !isSameWorkspacePath(candidate, path),
      );
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
        (draftPrimaryFolder
          ? workspaceLabelForPath(draftPrimaryFolder)
          : undefined);
      const result = await runtimeClient.createProject(folders, displayName);
      if (!result.ok) {
        setNotice(
          result.code === "PROJECT_OPERATION_CONFLICT"
            ? "该项目正在移除中"
            : result.message,
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
        shouldResetActiveThreadForWorkspaceChange(
          activeThreadWorkspacePath,
          project.workspaceRoot,
        )
      ) {
        resetActiveThreadContext();
      }
      // A folder-less project has no path to select. Leaving the target and the
      // workspace path untouched would keep pointing at the previous project, so
      // both are cleared and the selection carries the identity instead.
      workspaceTargetPathRef.current = project.workspaceRoot;
      setWorkspacePath(project.workspaceRoot);
      setSelectedProjectId(nextProjectId);
      setExpandedProjectIds((current) =>
        current.includes(nextProjectId) ? current : [...current, nextProjectId],
      );
      setCreateProjectDialogOpen(false);
      setDraftProjectFolders([]);
      setDraftPrimaryFolder("");
      setDraftProjectName("");
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to create project",
      );
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
      setNotice(
        error instanceof Error ? error.message : "项目文件夹操作暂时失败。",
      );
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
        const result = await runtimeClient.attachProjectFolder(
          backendProjectId,
          path,
        );
        if (result.ok) {
          attached.push(label);
        } else {
          failures.push(`“${label}”：${result.message}`);
        }
      } catch (error) {
        failures.push(
          `“${label}”：${error instanceof Error ? error.message : "加入失败"}`,
        );
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
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  async function handleSetPrimaryProjectFolder(
    projectId: string,
    path: string,
  ): Promise<void> {
    await runProjectFolderMutation(
      projectId,
      path,
      (id) => runtimeClient.setPrimaryProjectFolder(id, path),
      `已把“${workspaceLabelForPath(path)}”设为主文件夹。`,
    );
  }

  async function handleDetachProjectFolder(
    projectId: string,
    path: string,
  ): Promise<void> {
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
    if (item.kind === "planning" || item.kind === "thinking") {
      return (
        <ThoughtTrace
          key={key}
          text={item.kind === "planning" ? "" : item.text}
          status={item.kind === "planning" ? "active" : item.status}
          theme={appearanceTheme}
        />
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
    if (item.kind === "memoryReceipt") {
      return (
        <ToolTraceLine
          key={key}
          title={item.title}
          detail={item.detail}
          status={item.status}
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

  const platform = runtimeClient.platform as PlatformKind;
  const composerBarAttachments = composerAttachmentViews(composerAttachments);
  const layoutStyle = {
    "--aw-sidebar-width": `${sidebarWidth}px`,
    "--aw-inspector-width": `${inspectorWidth}px`,
  } as CSSProperties;

  function renderExternalActionGate() {
    if (activeExternalInteraction) {
      const resolving =
        resolvingExternalInteractionId === activeExternalInteraction.id;
      const isACPPermission =
        activeExternalInteraction.kind === "acp-permission";
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
                  label:
                    projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ??
                    "awaken",
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
                navItems={navItems}
                activeNavId={activeNavId}
                projects={repositoryProjects}
                activeProjectId={activeProjectId}
                expandedIds={expandedProjectIds}
                defaultExpandedIds={[DEFAULT_WORKSPACE_PROJECT_ID]}
                profileName="Nolan"
                profileAvatarFallback="AW"
                newTaskLabel="New Agent"
                projectSectionLabel="项目工作区"
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
                onSelectNav={handleSelectNav}
                onSelectProject={handleSelectProject}
                onReorderProjects={(ids) => {
                  void handleReorderProjects(ids);
                }}
                projectReorderDisabled={
                  !desktopReady || projectReorderSubmitting
                }
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
                          {contextUsagePanelOpen && contextPanelData ? (
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
                        </>
                      }
                      composer={
                        showCenterComposer ? undefined : (
                          <ComposerDock ref={setComposerDockElement}>
                            <div className="composer-dock-stack">
                              {renderComposerRuntimePanels()}
                              {renderUserQuestionPrompt()}
                              {renderExternalActionGate()}
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
                          {renderExternalActionGate()}
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
                    <InspectorRail
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
                icon={<Monitor size={15} strokeWidth={1.9} />}
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
            folders={
              editingProjectId ? projectFolderRows(editingProjectId) : []
            }
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
