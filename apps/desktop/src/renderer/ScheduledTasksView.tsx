// 定时任务产品页面（契约 §4.1/§4.4）：列表 + 创建/编辑 + 模板闭环 + 启停/立即执行/删除。
// 所有 Core payload 组装与错误映射都委托 ./adapter，本组件只做状态编排；
// 切换本页面不触碰当前项目、活动 thread 或进行中的 run。
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { Plus, RefreshCw } from "lucide-react";
import {
  AwakenButton,
  AwakenDialog,
  AwakenScheduledTaskEditorDialog,
  AwakenScheduledTaskHistoryDialog,
  AwakenScheduledTaskList,
  AwakenScheduledTaskTemplateDialog,
  createEmptyAwakenScheduledTaskDraft,
  type ComposerBarStructuredInputValue,
  type AwakenScheduledTaskDraft,
  type AwakenScheduledTaskDraftField,
  type AwakenScheduledTaskExecutionItem,
  type AwakenScheduledTaskListItem,
  type AwakenScheduledTaskProfileOption,
  type AwakenScheduledTaskProjectOption,
} from "@awaken/design-system";
import type {
  ComposerToken,
  RuntimeProject,
  ScheduledTask,
} from "../shared/runtime";
import {
  actionsFromSlashProviders,
  clearSlashTriggerFromComposerValue,
  composerTokenFromSlashAction,
  filterSlashActions,
  firstSelectableSlashAction,
  skillSlashProvider,
  slashTriggerFromComposerValue,
  type SlashAction,
} from "./composerSlash";
import type { RuntimeClient } from "./runtime/client";
import {
  buildScheduledTaskTemplateCategories,
  buildTemplateDraft,
} from "./scheduled-tasks/templates";
import {
  SCHEDULED_TASK_KIND_LABELS,
  SCHEDULED_TASK_STATE_LABELS,
  TASK_ALREADY_RUNNING_MESSAGE,
  VERSION_CONFLICT_MESSAGE,
  draftToCreateInput,
  draftToPatchInput,
  formatExecutionItem,
  formatLastRunText,
  formatNextRunText,
  formatScheduleText,
  isTaskAlreadyRunningError,
  isVersionConflictError,
  mapScheduledTaskSubmitError,
  scheduledTaskErrorMessage,
  taskToDraft,
} from "./scheduled-tasks/adapter";

export type ScheduledTasksViewProps = {
  runtimeClient: RuntimeClient;
  desktopReady: boolean;
  onOpenThread: (threadId: string) => void;
};

type EditorState = {
  open: boolean;
  mode: "create" | "edit";
  draft: AwakenScheduledTaskDraft;
  editingTask: ScheduledTask | null;
  busy: boolean;
  errorMessage: string | null;
  fieldErrors: Partial<Record<AwakenScheduledTaskDraftField, string>>;
  composerTokens: ComposerToken[];
};

type HistoryState = {
  open: boolean;
  taskName: string;
  items: AwakenScheduledTaskExecutionItem[];
  loading: boolean;
};

type ConfirmState = {
  kind: "run" | "delete";
  task: ScheduledTask;
  busy: boolean;
};

const FALLBACK_TIMEZONES = [
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Singapore",
  "Europe/London",
  "Europe/Berlin",
  "America/New_York",
  "America/Los_Angeles",
  "UTC",
];

function supportedTimezones(): string[] {
  try {
    const values = (
      Intl as typeof Intl & {
        supportedValuesOf?: (key: "timeZone") => string[];
      }
    ).supportedValuesOf?.("timeZone");
    if (Array.isArray(values) && values.length > 0) {
      return values;
    }
  } catch {
    // fall through to the static table
  }
  return FALLBACK_TIMEZONES;
}

function projectNameForTask(
  task: ScheduledTask,
  projects: RuntimeProject[],
): string {
  const project = projects.find((candidate) => candidate.projectId === task.projectId);
  if (!project) {
    return task.projectId;
  }
  if (project.state !== "active") {
    return "已移除项目";
  }
  return project.displayName || task.projectId;
}

function cloneComposerTokens(tokens?: ComposerToken[]): ComposerToken[] {
  return (tokens ?? []).map((token) => ({
    ...token,
    payload: token.payload ? { ...token.payload } : undefined,
  }));
}

function skillRootsForProject(project?: RuntimeProject): string[] {
  if (!project) {
    return [];
  }
  const folderRoots = project.folders
    .map((folder) => folder.path.trim())
    .filter(Boolean);
  if (folderRoots.length > 0) {
    return folderRoots;
  }
  const workspaceRoot = project.workspaceRoot.trim();
  return workspaceRoot ? [workspaceRoot] : [];
}

function ConfirmActionDialog({
  state,
  onCancel,
  onConfirm,
}: {
  state: ConfirmState;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const isRun = state.kind === "run";
  return (
    <AwakenDialog
      open
      className="scheduled-tasks-view__confirm-dialog"
      title={isRun ? "立即执行" : "删除定时任务"}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={state.busy} onClick={onCancel}>
            取消
          </AwakenButton>
          <AwakenButton variant="primary" size="md" loading={state.busy} onClick={onConfirm}>
            {isRun ? "确认执行" : "确认删除"}
          </AwakenButton>
        </>
      }
    >
      <p className="scheduled-tasks-view__confirm-text">
        {isRun
          ? `确定要立即执行“${state.task.name}”吗？将在后台运行一次，不影响原有计划。`
          : `确定要删除“${state.task.name}”吗？此操作不可撤销。`}
      </p>
    </AwakenDialog>
  );
}

export function ScheduledTasksView({
  runtimeClient,
  desktopReady,
  onOpenThread,
}: ScheduledTasksViewProps) {
  const [tasks, setTasks] = useState<ScheduledTask[] | null>(null);
  const [projects, setProjects] = useState<RuntimeProject[]>([]);
  const [profiles, setProfiles] = useState<AwakenScheduledTaskProfileOption[]>([]);
  const [systemTimezone, setSystemTimezone] = useState("UTC");
  const [notice, setNotice] = useState<string | null>(null);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [history, setHistory] = useState<HistoryState | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [slashActions, setSlashActions] = useState<SlashAction[]>([]);
  const [slashLoading, setSlashLoading] = useState(false);
  const [slashError, setSlashError] = useState<string | null>(null);
  const [slashActiveActionId, setSlashActiveActionId] = useState<string | undefined>();
  const timezones = useMemo(supportedTimezones, []);
  const slashProviders = useMemo(
    () => [skillSlashProvider((projectRoots) => runtimeClient.listSkills(projectRoots))],
    [runtimeClient],
  );

  const refreshTasks = useCallback(async (): Promise<void> => {
    try {
      const list = await runtimeClient.listScheduledTasks();
      setTasks(list);
    } catch {
      // 静默刷新失败不覆盖现有列表；加载失败由首次加载路径提示。
    }
  }, [runtimeClient]);

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    let cancelled = false;
    void (async () => {
      const [taskList, projectList, profileList, timezone] = await Promise.all([
        runtimeClient.listScheduledTasks().catch(() => null),
        runtimeClient.listProjects(true).catch(() => [] as RuntimeProject[]),
        runtimeClient.listModelProfiles().catch(() => []),
        runtimeClient.getSystemTimezone().catch(() => "UTC"),
      ]);
      if (cancelled) {
        return;
      }
      if (taskList === null) {
        setNotice("定时任务加载失败，请稍后刷新重试。");
        setTasks([]);
      } else {
        setTasks(taskList);
      }
      setProjects(projectList);
      setProfiles(
        profileList
          .filter((profile) => profile.enabled && profile.profileStatus === "ready")
          .map((profile) => ({
            profileId: profile.id,
            name: profile.modelId || profile.displayName || profile.id,
          })),
      );
      setSystemTimezone(timezone || "UTC");
    })();
    return () => {
      cancelled = true;
    };
  }, [desktopReady, runtimeClient]);

  // 事件流只做静默刷新；页面卸载仅退订，绝不取消后台执行。
  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    return runtimeClient.subscribeScheduledTaskEvents(() => {
      void refreshTasks();
    });
  }, [desktopReady, runtimeClient, refreshTasks]);

  const items = useMemo<AwakenScheduledTaskListItem[]>(
    () =>
      (tasks ?? []).map((task) => ({
        taskId: task.taskId,
        name: task.name,
        kindLabel: SCHEDULED_TASK_KIND_LABELS[task.action.kind] ?? task.action.kind,
        projectName: projectNameForTask(task, projects),
        scheduleText: formatScheduleText(task),
        enabled: task.enabled,
        state: task.state,
        stateLabel: SCHEDULED_TASK_STATE_LABELS[task.state] ?? task.state,
        nextRunText: formatNextRunText(task),
        lastRunText: formatLastRunText(task),
        running: task.state === "running",
      })),
    [tasks, projects],
  );

  const projectOptions = useMemo<AwakenScheduledTaskProjectOption[]>(
    () =>
      projects
        .filter((project) => project.state === "active")
        .map((project) => ({
          projectId: project.projectId,
          name: project.displayName || project.projectId,
        })),
    [projects],
  );

  const editorProjectId = editor?.draft.projectId ?? "";
  const editorActionKind = editor?.draft.actionKind;
  const editorContent = editor?.draft.content ?? "";
  const editorProject = useMemo(
    () => projects.find((project) => project.projectId === editorProjectId),
    [editorProjectId, projects],
  );
  const editorSkillRoots = useMemo(
    () => skillRootsForProject(editorProject),
    [editorProject],
  );
  const editorSkillRootsKey = editorSkillRoots.join("\u0000");
  const slashTrigger =
    editorActionKind === "agent" ? slashTriggerFromComposerValue(editorContent) : null;
  const slashPaletteOpen = Boolean(editor?.open && slashTrigger && desktopReady);
  const filteredSlashActions = useMemo(
    () => filterSlashActions(slashActions, slashTrigger?.query ?? "").slice(0, 24),
    [slashActions, slashTrigger?.query],
  );
  const composerTokens = useMemo(
    () =>
      (editor?.composerTokens ?? []).map((token) => ({
        id: token.id,
        type: token.type,
        label: token.label,
        leadingText: token.leadingText,
        title:
          typeof token.payload?.skillFilePath === "string"
            ? token.payload.skillFilePath
            : token.label,
      })),
    [editor?.composerTokens],
  );
  const composerSlashActions = useMemo(
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

  useEffect(() => {
    if (!slashPaletteOpen) {
      setSlashLoading(false);
      setSlashError(null);
      setSlashActiveActionId(undefined);
      return;
    }
    if (!editorProject) {
      setSlashActions([]);
      setSlashLoading(false);
      setSlashError("请先选择运行项目");
      return;
    }

    let cancelled = false;
    setSlashActions([]);
    setSlashLoading(true);
    setSlashError(null);
    void actionsFromSlashProviders(slashProviders, {
      workspaceRootForRun: editorSkillRootsKey ? editorSkillRootsKey.split("\u0000") : [],
    })
      .then((actions) => {
        if (!cancelled) {
          setSlashActions(actions);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setSlashActions([]);
          setSlashError(error instanceof Error ? error.message : "斜杠菜单加载失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSlashLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [
    editorProject,
    editorSkillRootsKey,
    slashPaletteOpen,
    slashProviders,
  ]);

  useEffect(() => {
    if (!slashPaletteOpen) {
      return;
    }
    if (!filteredSlashActions.some((action) => action.id === slashActiveActionId)) {
      setSlashActiveActionId(firstSelectableSlashAction(filteredSlashActions)?.id);
    }
  }, [filteredSlashActions, slashActiveActionId, slashPaletteOpen]);

  function openCreateEditor(draft?: AwakenScheduledTaskDraft): void {
    setEditor({
      open: true,
      mode: "create",
      draft: draft ?? createEmptyAwakenScheduledTaskDraft(systemTimezone),
      editingTask: null,
      busy: false,
      errorMessage: null,
      fieldErrors: {},
      composerTokens: [],
    });
  }

  function openEditEditor(taskId: string): void {
    const task = (tasks ?? []).find((candidate) => candidate.taskId === taskId);
    if (!task) {
      return;
    }
    setEditor({
      open: true,
      mode: "edit",
      draft: taskToDraft(task),
      editingTask: task,
      busy: false,
      errorMessage: null,
      fieldErrors: {},
      composerTokens: cloneComposerTokens(task.action.composerTokens),
    });
  }

  // 模板选择只预填 draft，绝不直接创建（契约 §5）。
  function handleTemplateSelect(templateId: string): void {
    setTemplateOpen(false);
    openCreateEditor(buildTemplateDraft(templateId, systemTimezone, new Date()));
  }

  function handleSlashActionSelect(actionId: string): void {
    const action = filteredSlashActions.find((candidate) => candidate.id === actionId);
    if (!action || action.disabled) {
      return;
    }
    const token = composerTokenFromSlashAction(action);
    if (!token) {
      return;
    }
    setEditor((current) => {
      if (!current || current.draft.actionKind !== "agent") {
        return current;
      }
      const trigger = slashTriggerFromComposerValue(current.draft.content);
      if (!trigger) {
        return current;
      }
      if (current.composerTokens.some((candidate) => candidate.id === token.id)) {
        return {
          ...current,
          draft: {
            ...current.draft,
            content: clearSlashTriggerFromComposerValue(current.draft.content),
          },
        };
      }
      const leadingText = current.draft.content.slice(0, trigger.start);
      return {
        ...current,
        draft: {
          ...current.draft,
          content: current.draft.content.slice(trigger.end),
        },
        composerTokens: [
          ...current.composerTokens,
          leadingText ? { ...token, leadingText } : token,
        ],
      };
    });
    setSlashActiveActionId(undefined);
  }

  function handleComposerStructuredInputChange(
    nextValue: ComposerBarStructuredInputValue,
  ): void {
    setEditor((current) => {
      if (!current) {
        return current;
      }
      const currentTokens = new Map(
        current.composerTokens.map((token) => [token.id, token]),
      );
      const nextTokens = nextValue.tokens.flatMap((segment) => {
        const token = currentTokens.get(segment.id);
        if (!token) {
          return [];
        }
        const nextToken = { ...token };
        if (segment.leadingText) {
          nextToken.leadingText = segment.leadingText;
        } else {
          delete nextToken.leadingText;
        }
        return [nextToken];
      });
      return {
        ...current,
        draft: { ...current.draft, content: nextValue.value },
        composerTokens: nextTokens,
      };
    });
  }

  function handleRemoveComposerToken(tokenId: string): void {
    setEditor((current) => {
      if (!current) {
        return current;
      }
      const removedIndex = current.composerTokens.findIndex((token) => token.id === tokenId);
      if (removedIndex < 0) {
        return current;
      }
      const removed = current.composerTokens[removedIndex];
      const leadingText = removed?.leadingText ?? "";
      const nextTokens = current.composerTokens.filter((token) => token.id !== tokenId);
      const following = nextTokens[removedIndex];
      if (leadingText && following) {
        nextTokens[removedIndex] = {
          ...following,
          leadingText: `${leadingText}${following.leadingText ?? ""}`,
        };
        return { ...current, composerTokens: nextTokens };
      }
      return {
        ...current,
        draft: {
          ...current.draft,
          content: leadingText ? `${leadingText}${current.draft.content}` : current.draft.content,
        },
        composerTokens: nextTokens,
      };
    });
  }

  function handleComposerInputKeyDown(
    event: ReactKeyboardEvent<HTMLTextAreaElement | HTMLDivElement>,
  ): void {
    if (!slashPaletteOpen) {
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      setEditor((current) =>
        current
          ? {
              ...current,
              draft: {
                ...current.draft,
                content: clearSlashTriggerFromComposerValue(current.draft.content),
              },
            }
          : current,
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
    const selectableActions = filteredSlashActions.filter((action) => !action.disabled);
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
          (activeIndex - 1 + selectableActions.length) % selectableActions.length
        ]?.id,
      );
      return;
    }
    handleSlashActionSelect(slashActiveActionId ?? selectableActions[0]?.id ?? "");
  }

  async function handleEditorSubmit(draft: AwakenScheduledTaskDraft): Promise<void> {
    if (!editor || editor.busy) {
      return;
    }
    setEditor({ ...editor, busy: true, errorMessage: null, fieldErrors: {} });
    try {
      if (editor.mode === "create") {
        await runtimeClient.createScheduledTask(
          draftToCreateInput(draft, editor.composerTokens),
        );
      } else if (editor.editingTask) {
        await runtimeClient.patchScheduledTask(
          editor.editingTask.taskId,
          draftToPatchInput(draft, editor.editingTask.version, editor.composerTokens),
        );
      }
      setEditor(null);
      await refreshTasks();
    } catch (error) {
      const mapped = mapScheduledTaskSubmitError(error);
      setEditor((current) =>
        current
          ? {
              ...current,
              busy: false,
              errorMessage: mapped.errorMessage,
              fieldErrors: mapped.fieldErrors,
            }
          : current,
      );
      if (isVersionConflictError(error)) {
        await refreshTasks();
      }
    }
  }

  // 启停：乐观更新，失败时重新拉取真实状态回滚。
  async function handleToggleEnabled(taskId: string, enabled: boolean): Promise<void> {
    const task = (tasks ?? []).find((candidate) => candidate.taskId === taskId);
    if (!task) {
      return;
    }
    setTasks((current) =>
      (current ?? []).map((candidate) =>
        candidate.taskId === taskId ? { ...candidate, enabled } : candidate,
      ),
    );
    try {
      const updated = enabled
        ? await runtimeClient.resumeScheduledTask(taskId, task.version)
        : await runtimeClient.pauseScheduledTask(taskId, task.version);
      setTasks((current) =>
        (current ?? []).map((candidate) =>
          candidate.taskId === taskId ? updated : candidate,
        ),
      );
    } catch (error) {
      await refreshTasks();
      setNotice(
        isVersionConflictError(error)
          ? VERSION_CONFLICT_MESSAGE
          : scheduledTaskErrorMessage(error),
      );
    }
  }

  async function handleConfirmRun(): Promise<void> {
    if (!confirm || confirm.kind !== "run" || confirm.busy) {
      return;
    }
    const { task } = confirm;
    setConfirm({ ...confirm, busy: true });
    try {
      await runtimeClient.runScheduledTaskNow(task.taskId, window.crypto.randomUUID());
      setConfirm(null);
      await refreshTasks();
    } catch (error) {
      setConfirm(null);
      setNotice(
        isTaskAlreadyRunningError(error)
          ? TASK_ALREADY_RUNNING_MESSAGE
          : scheduledTaskErrorMessage(error),
      );
      await refreshTasks();
    }
  }

  async function handleConfirmDelete(): Promise<void> {
    if (!confirm || confirm.kind !== "delete" || confirm.busy) {
      return;
    }
    const { task } = confirm;
    if (task.state === "running") {
      setConfirm(null);
      setNotice("任务正在运行，请先取消执行或等待结束后再删除。");
      return;
    }
    setConfirm({ ...confirm, busy: true });
    try {
      await runtimeClient.deleteScheduledTask(
        task.taskId,
        task.version,
        window.crypto.randomUUID(),
      );
      setConfirm(null);
      await refreshTasks();
    } catch (error) {
      setConfirm(null);
      setNotice(
        isVersionConflictError(error)
          ? VERSION_CONFLICT_MESSAGE
          : scheduledTaskErrorMessage(error),
      );
      await refreshTasks();
    }
  }

  async function handleCancelRun(taskId: string): Promise<void> {
    try {
      await runtimeClient.cancelScheduledTaskExecution(taskId);
      await refreshTasks();
    } catch (error) {
      setNotice(scheduledTaskErrorMessage(error));
      await refreshTasks();
    }
  }

  async function openHistory(taskId: string): Promise<void> {
    const task = (tasks ?? []).find((candidate) => candidate.taskId === taskId);
    setHistory({
      open: true,
      taskName: task?.name ?? "定时任务",
      items: [],
      loading: true,
    });
    try {
      const executions = await runtimeClient.listScheduledTaskExecutions(taskId);
      const items = executions.map((execution) => formatExecutionItem(execution));
      setHistory((current) =>
        current ? { ...current, items, loading: false } : current,
      );
    } catch (error) {
      setHistory((current) =>
        current ? { ...current, items: [], loading: false } : current,
      );
      setNotice(scheduledTaskErrorMessage(error));
    }
  }

  return (
    <section className="extension-view scheduled-tasks-view" aria-label="定时任务">
      <div className="extension-view-inner">
        <header className="extension-header">
          <div>
            <h1>定时任务</h1>
            <p>按计划自动执行提醒或 Agent 任务；Awaken 退出期间不会运行。</p>
          </div>
          <div className="extension-header-actions">
            <button
              type="button"
              className="extension-icon-button"
              aria-label="刷新"
              disabled={tasks === null}
              onClick={() => {
                setNotice(null);
                void refreshTasks();
              }}
            >
              <RefreshCw size={16} strokeWidth={1.9} />
            </button>
            <AwakenButton
              variant="default"
              size="md"
              onClick={() => setTemplateOpen(true)}
            >
              从模板创建
            </AwakenButton>
            <AwakenButton
              variant="primary"
              size="md"
              onClick={() => openCreateEditor()}
            >
              <Plus size={16} strokeWidth={1.9} />
              创建任务
            </AwakenButton>
          </div>
        </header>

        {notice ? (
          <div className="scheduled-tasks-view__notice" role="alert">
            <span>{notice}</span>
            <button
              type="button"
              className="scheduled-tasks-view__notice-dismiss"
              aria-label="关闭提示"
              onClick={() => setNotice(null)}
            >
              ×
            </button>
          </div>
        ) : null}

        <AwakenScheduledTaskList
          items={items}
          loading={tasks === null}
          onCreate={() => openCreateEditor()}
          onCreateFromTemplate={() => setTemplateOpen(true)}
          onToggleEnabled={(taskId, enabled) => {
            void handleToggleEnabled(taskId, enabled);
          }}
          onRunNow={(taskId) => {
            const task = (tasks ?? []).find((candidate) => candidate.taskId === taskId);
            if (task) {
              setConfirm({ kind: "run", task, busy: false });
            }
          }}
          onCancelRun={(taskId) => {
            void handleCancelRun(taskId);
          }}
          onShowHistory={(taskId) => {
            void openHistory(taskId);
          }}
          onEdit={openEditEditor}
          onDelete={(taskId) => {
            const task = (tasks ?? []).find((candidate) => candidate.taskId === taskId);
            if (task) {
              setConfirm({ kind: "delete", task, busy: false });
            }
          }}
        />
      </div>

      <AwakenScheduledTaskTemplateDialog
        open={templateOpen}
        title="从模板创建"
        description="选择内置模板预填任务，保存前可以修改所有字段。"
        categories={buildScheduledTaskTemplateCategories()}
        onOpenChange={setTemplateOpen}
        onSelect={handleTemplateSelect}
      />

      {editor ? (
        <AwakenScheduledTaskEditorDialog
          open={editor.open}
          mode={editor.mode}
          value={editor.draft}
          projects={projectOptions}
          profiles={profiles}
          timezones={timezones}
          systemTimezone={systemTimezone}
          busy={editor.busy}
          errorMessage={editor.errorMessage}
          fieldErrors={editor.fieldErrors}
          composerTokens={composerTokens}
          slashPalette={{
            open: slashPaletteOpen,
            actions: composerSlashActions,
            activeActionId: slashActiveActionId,
            loading: slashLoading,
            error: slashError,
            emptyLabel: "没有匹配项",
            onSelectAction: handleSlashActionSelect,
            onActiveActionChange: setSlashActiveActionId,
          }}
          onOpenChange={(open) => {
            if (!open) {
              setEditor(null);
            }
          }}
          onValueChange={(draft) =>
            setEditor((current) => (current ? { ...current, draft } : current))
          }
          onComposerStructuredInputChange={handleComposerStructuredInputChange}
          onComposerInputKeyDown={handleComposerInputKeyDown}
          onRemoveComposerToken={handleRemoveComposerToken}
          onSubmit={(draft) => {
            void handleEditorSubmit(draft);
          }}
        />
      ) : null}

      {history ? (
        <AwakenScheduledTaskHistoryDialog
          open={history.open}
          title={`执行记录 · ${history.taskName}`}
          items={history.items}
          emptyDescription={
            history.loading
              ? "正在加载执行记录…"
              : "任务按计划或手动触发后，最近的执行结果会显示在这里。"
          }
          onOpenChange={(open) => {
            if (!open) {
              setHistory(null);
            }
          }}
          onOpenThread={(item) => {
            if (item.threadId) {
              onOpenThread(item.threadId);
            }
          }}
        />
      ) : null}

      {confirm ? (
        <ConfirmActionDialog
          state={confirm}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            if (confirm.kind === "run") {
              void handleConfirmRun();
            } else {
              void handleConfirmDelete();
            }
          }}
        />
      ) : null}
    </section>
  );
}
