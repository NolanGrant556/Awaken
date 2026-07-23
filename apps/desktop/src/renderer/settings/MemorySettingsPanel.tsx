import { useCallback, useEffect, useRef, useState } from "react";
import {
  AwakenMemoryLightFlowLayout,
  AwakenMemorySettings,
  type AwakenMemoryActivityItem,
  type AwakenMemoryFilterValue,
} from "@awaken/design-system";
import type {
  AwakenTheme,
  MemoryDreamTask,
  MemoryScope,
  MemorySettings,
  MemoryType,
  RuntimeMemory,
  RuntimeProject,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

const switchCopy: Array<{ key: keyof MemorySettings; title: string; description: string }> = [
  { key: "enabled", title: "记忆", description: "控制长期记忆的召回和 Agent 写入能力。" },
  { key: "automaticRecall", title: "自动召回", description: "由当前主模型按任务相关性选择已有记忆。" },
  { key: "proactiveMemory", title: "主动记忆", description: "主模型在正常任务中自行判断是否值得长期保存。" },
  { key: "autoDream", title: "Auto Dream", description: "空闲并达到整理阈值时自动维护历史记忆。" },
];

export function memoryDreamStatusLabel(status: string): string {
  return ({
    queued: "等待整理",
    running: "正在整理",
    completed: "整理完成",
    failed: "整理失败",
    cancelled: "已取消",
    interrupted: "整理中断",
  } as Record<string, string>)[status] ?? "状态未知";
}

const memoryScopeLabels: Record<MemoryScope, string> = { user: "用户记忆", project: "项目记忆" };
const memoryTypeLabels: Record<MemoryType, string> = { user: "用户资料", feedback: "协作偏好", project: "项目背景", reference: "参考入口" };
const recentActivityLimit = 40;

type RecentMemoryEntry = {
  memory: RuntimeMemory;
  projectId?: string;
  projectName?: string;
};

type RecentActivitySnapshot = {
  memories: RecentMemoryEntry[];
  dreamTasks: MemoryDreamTask[];
  errors: string[];
};

type RecentActivityClient = Pick<RuntimeClient, "listMemories" | "listMemoryDreams">;
type RecentActivitySource = { label: string; projectId?: string; projectName?: string };

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function loadRecentActivitySnapshot(
  runtimeClient: RecentActivityClient,
  projects: RuntimeProject[],
): Promise<RecentActivitySnapshot> {
  const sources: RecentActivitySource[] = [
    { label: "用户记忆" },
    ...projects.map((project) => ({ label: project.displayName, projectId: project.projectId, projectName: project.displayName })),
  ];
  const [memoryResults, dreamResults] = await Promise.all([
    Promise.allSettled(sources.map(async (source) => ({
      source,
      items: await runtimeClient.listMemories({
        scope: source.projectId ? "project" : "user",
        projectId: source.projectId,
        limit: 20,
      }),
    }))),
    Promise.allSettled(sources.map(async (source) => ({
      source,
      items: await runtimeClient.listMemoryDreams(source.projectId, 20),
    }))),
  ]);

  const memories: RecentMemoryEntry[] = [];
  const dreamTasks = new Map<string, MemoryDreamTask>();
  const errors: string[] = [];

  memoryResults.forEach((result, index) => {
    if (result.status === "rejected") {
      errors.push(`${sources[index].label}读取失败：${errorMessage(result.reason)}`);
      return;
    }
    result.value.items.forEach((memory) => memories.push({
      memory,
      projectId: result.value.source.projectId,
      projectName: result.value.source.projectName,
    }));
  });
  dreamResults.forEach((result, index) => {
    if (result.status === "rejected") {
      errors.push(`${sources[index].label}整理记录读取失败：${errorMessage(result.reason)}`);
      return;
    }
    result.value.items.forEach((task) => dreamTasks.set(task.taskId, task));
  });

  return { memories, dreamTasks: [...dreamTasks.values()], errors };
}

export function memoryRelativeTime(value: string, now = Date.now()): string {
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return "时间未知";
  const elapsed = Math.max(0, now - timestamp);
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days} 天前`;
  return new Date(timestamp).toLocaleDateString();
}

function memoryStatusLabel(status: RuntimeMemory["status"]): string {
  return status === "active" ? "有效" : "已替代";
}

function dreamScopeLabel(scopes: MemoryScope[]): string {
  if (scopes.includes("user") && scopes.includes("project")) return "用户与项目记忆";
  return scopes[0] ? memoryScopeLabels[scopes[0]] : "记忆范围未知";
}

function dreamReportTotals(task: MemoryDreamTask) {
  return Object.values(task.reports ?? {}).reduce((totals, report) => ({
    updated: totals.updated + (report?.updated ?? 0),
    superseded: totals.superseded + (report?.superseded ?? 0),
    conflicts: totals.conflicts + (report?.conflicts ?? 0),
    skipped: totals.skipped + (report?.skipped ?? 0),
  }), { updated: 0, superseded: 0, conflicts: 0, skipped: 0 });
}

function toMemoryListItem(memory: RuntimeMemory, projectId?: string) {
  return {
    id: memory.memoryId,
    name: memory.name,
    description: memory.description,
    scopeLabel: memoryScopeLabels[memory.scope],
    typeLabel: memoryTypeLabels[memory.type],
    statusLabel: memoryStatusLabel(memory.status),
    status: memory.status,
    updatedLabel: memoryRelativeTime(memory.updatedAt),
    kind: memory.type,
    projectId: memory.scope === "project" ? projectId : undefined,
  };
}

export function MemorySettingsPanel({
  runtimeClient,
  currentProjectId,
  theme = "light",
}: {
  runtimeClient: RuntimeClient;
  currentProjectId?: string;
  theme?: AwakenTheme;
}) {
  const [settings, setSettings] = useState<MemorySettings | null>(null);
  const [revision, setRevision] = useState(0);
  const [projects, setProjects] = useState<RuntimeProject[]>([]);
  const [scope, setScope] = useState<MemoryScope>("user");
  const [projectId, setProjectId] = useState("");
  const [type, setType] = useState<MemoryType | "">("");
  const [status, setStatus] = useState<"active" | "superseded" | "">("");
  const [updatedWithinDays, setUpdatedWithinDays] = useState(0);
  const [query, setQuery] = useState("");
  const [memories, setMemories] = useState<RuntimeMemory[]>([]);
  const [activityMemories, setActivityMemories] = useState<RecentMemoryEntry[]>([]);
  const [activityDreamTasks, setActivityDreamTasks] = useState<MemoryDreamTask[]>([]);
  const [selected, setSelected] = useState<RuntimeMemory | null>(null);
  const [selectedMemoryProjectId, setSelectedMemoryProjectId] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [dreamTaskId, setDreamTaskId] = useState<string | null>(null);
  const [dreamStatus, setDreamStatus] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const loadRequestId = useRef(0);
  const activityLoadRequestId = useRef(0);

  const loadMemories = useCallback(async () => {
    const requestId = ++loadRequestId.current;
    if (scope === "project" && !projectId) {
      setMemories([]);
      setSelected(null);
      return;
    }
    try {
      const items = await runtimeClient.listMemories({ query, scope, projectId: scope === "project" ? projectId : undefined, type: type || undefined, status: status || undefined, limit: 20 });
      if (requestId !== loadRequestId.current) return;
      const cutoff = updatedWithinDays > 0 ? Date.now() - updatedWithinDays * 24 * 60 * 60 * 1000 : 0;
      const visible = cutoff ? items.filter((item) => new Date(item.updatedAt).getTime() >= cutoff) : items;
      setMemories(visible);
      setSelected((current) => current && !visible.some((item) => item.memoryId === current.memoryId) ? null : current);
    } catch (error) {
      if (requestId !== loadRequestId.current) return;
      setMessage(error instanceof Error ? error.message : "无法读取记忆。");
    }
  }, [projectId, query, runtimeClient, scope, status, type, updatedWithinDays]);

  const loadRecentActivity = useCallback(async () => {
    const requestId = ++activityLoadRequestId.current;
    const snapshot = await loadRecentActivitySnapshot(runtimeClient, projects);
    if (requestId !== activityLoadRequestId.current) return;
    setActivityMemories(snapshot.memories);
    setActivityDreamTasks(snapshot.dreamTasks);
    if (snapshot.errors.length) setMessage(`部分最近动态无法读取：${snapshot.errors.join("；")}`);
  }, [projects, runtimeClient]);

  useEffect(() => {
    let live = true;
    void Promise.all([runtimeClient.getMemorySettings(), runtimeClient.listProjects()])
      .then(([document, nextProjects]) => {
        if (!live) return;
        setSettings(document.value);
        setRevision(document.revision);
        setProjects(nextProjects);
        setProjectId(nextProjects.some((project) => project.projectId === currentProjectId) ? currentProjectId ?? "" : "");
      })
      .catch((error: unknown) => live && setMessage(error instanceof Error ? error.message : "无法读取记忆设置。"));
    return () => { live = false; };
  }, [currentProjectId, runtimeClient]);

  useEffect(() => {
    setSelected(null);
    setSelectedMemoryProjectId(undefined);
    setMemories([]);
    void loadMemories();
  }, [loadMemories]);

  useEffect(() => {
    void loadRecentActivity();
  }, [loadRecentActivity]);

  useEffect(() => {
    if (!dreamTaskId) return;
    const timer = window.setInterval(() => {
      void runtimeClient.getMemoryDream(dreamTaskId).then((task) => {
        setDreamStatus(task.status);
        setActivityDreamTasks((current) => [task, ...current.filter((item) => item.taskId !== task.taskId)]);
        if (["completed", "failed", "cancelled", "interrupted"].includes(task.status)) {
          window.clearInterval(timer);
          void Promise.all([loadMemories(), loadRecentActivity()]);
        }
      }).catch((error: unknown) => {
        window.clearInterval(timer);
        setMessage(error instanceof Error ? error.message : "无法读取 Dream 状态。");
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [dreamTaskId, loadMemories, loadRecentActivity, runtimeClient]);

  function changeFilter<T>(setter: (value: T) => void, value: T) {
    loadRequestId.current += 1;
    setSelected(null);
    setSelectedMemoryProjectId(undefined);
    setMemories([]);
    setter(value);
  }

  async function toggle(key: keyof MemorySettings) {
    if (!settings) return;
    setBusy(true);
    try {
      const document = await runtimeClient.updateMemorySettings({ expectedRevision: revision, [key]: !settings[key] });
      setSettings(document.value);
      setRevision(document.revision);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "更新记忆设置失败。");
    } finally { setBusy(false); }
  }

  async function openMemory(memory: RuntimeMemory, memoryProjectId?: string) {
    const requestId = loadRequestId.current;
    try {
      const targetProjectId = memory.scope === "project" ? memoryProjectId : undefined;
      const detail = await runtimeClient.getMemory(memory.memoryId, targetProjectId);
      if (requestId === loadRequestId.current) {
        setSelected(detail);
        setSelectedMemoryProjectId(targetProjectId);
      }
    } catch (error) {
      if (requestId === loadRequestId.current) setMessage(error instanceof Error ? error.message : "无法读取记忆正文。");
    }
  }

  async function removeMemory(memory: RuntimeMemory, memoryProjectId?: string) {
    if (!window.confirm(`确定删除记忆“${memory.name}”吗？此操作不可撤销。`)) return;
    try {
      await runtimeClient.deleteMemory(memory.memoryId, memory.revision, memory.scope === "project" ? memoryProjectId : undefined);
      if (selected?.memoryId === memory.memoryId) {
        setSelected(null);
        setSelectedMemoryProjectId(undefined);
      }
      await Promise.all([loadMemories(), loadRecentActivity()]);
    } catch (error) { setMessage(error instanceof Error ? error.message : "删除记忆失败。"); }
  }

  async function startDream() {
    setBusy(true);
    try {
      const task = await runtimeClient.startMemoryDream(scope === "project" ? projectId : undefined);
      setDreamTaskId(task.taskId);
      setDreamStatus(task.status);
      setActivityDreamTasks((current) => [task, ...current.filter((item) => item.taskId !== task.taskId)]);
    } catch (error) { setMessage(error instanceof Error ? error.message : "启动 Dream 失败。"); }
    finally { setBusy(false); }
  }

  const listItems = memories.map((memory) => toMemoryListItem(memory, projectId));

  const projectNames = new Map(projects.map((project) => [project.projectId, project.displayName]));
  const activities: AwakenMemoryActivityItem[] = [
    ...activityMemories.map(({ memory, projectId: memoryProjectId, projectName }) => ({
      timestamp: new Date(memory.updatedAt).getTime(),
      item: {
        id: `memory:${memoryProjectId ?? "user"}:${memory.memoryId}`,
        kind: "memory" as const,
        label: memory.status === "superseded" ? "已替代" : memoryScopeLabels[memory.scope],
        title: memory.name,
        description: memory.description,
        timeLabel: memoryRelativeTime(memory.updatedAt),
        memoryId: memory.memoryId,
        memoryProjectId,
        status: memory.status,
        details: [
          { label: "范围", value: memoryScopeLabels[memory.scope] },
          ...(projectName ? [{ label: "项目", value: projectName }] : []),
          { label: "类型", value: memoryTypeLabels[memory.type] },
          { label: "状态", value: memoryStatusLabel(memory.status) },
          { label: "Revision", value: String(memory.revision) },
        ],
      },
    })),
    ...activityDreamTasks.map((task) => {
      const totals = dreamReportTotals(task);
      return {
        timestamp: new Date(task.finishedAt ?? task.startedAt ?? task.createdAt).getTime(),
        item: {
          id: `dream:${task.taskId}`,
          kind: "dream" as const,
          label: "记忆整理",
          title: memoryDreamStatusLabel(task.status),
          description: `更新 ${totals.updated} 条 · 已替代 ${totals.superseded} 条 · 冲突 ${totals.conflicts} 条 · 跳过 ${totals.skipped} 条`,
          timeLabel: memoryRelativeTime(task.finishedAt ?? task.startedAt ?? task.createdAt),
          status: task.status,
          stats: [
            { label: "更新", value: totals.updated },
            { label: "已替代", value: totals.superseded },
            { label: "冲突", value: totals.conflicts },
            { label: "跳过", value: totals.skipped },
          ],
          details: [
            { label: "范围", value: dreamScopeLabel(task.scopes) },
            ...(task.projectId ? [{ label: "项目", value: projectNames.get(task.projectId) ?? task.projectId }] : []),
            { label: "状态", value: memoryDreamStatusLabel(task.status) },
            ...(task.phase ? [{ label: "阶段", value: task.phase }] : []),
            { label: "更新", value: `${totals.updated} 条` },
            { label: "已替代", value: `${totals.superseded} 条` },
            { label: "冲突", value: `${totals.conflicts} 条` },
            { label: "跳过", value: `${totals.skipped} 条` },
          ],
          body: task.errorMessage,
        },
      };
    }),
  ].sort((a, b) => b.timestamp - a.timestamp).slice(0, recentActivityLimit).map((entry) => entry.item);

  const selectedListItem = selected ? toMemoryListItem(selected, selectedMemoryProjectId) : null;

  return (
    <AwakenMemoryLightFlowLayout
      className="desktop-memory-settings"
      theme={theme}
      titleAs="h1"
    >
      <AwakenMemorySettings
        switches={switchCopy.map((item) => ({
          id: item.key,
          title: item.title,
          description: item.description,
          checked: settings?.[item.key] ?? false,
        }))}
        filters={[
          { id: "scope", label: "记忆范围", value: scope, options: [{ value: "user", label: "用户记忆" }, { value: "project", label: "项目记忆" }] },
          ...(scope === "project" ? [{ id: "project", label: "项目", value: projectId, options: projects.map((project) => ({ value: project.projectId, label: project.displayName })), placeholder: "（项目待选择）", disabled: !projects.length }] : []),
          { id: "type", label: "记忆类型", value: type, options: [{ value: "", label: "全部类型" }, { value: "user", label: "用户" }, { value: "feedback", label: "反馈" }, { value: "project", label: "项目" }, { value: "reference", label: "参考" }] },
          { id: "status", label: "记忆状态", value: status, options: [{ value: "", label: "全部状态" }, { value: "active", label: "有效" }, { value: "superseded", label: "已替代" }] },
          { id: "updated", label: "更新时间", value: updatedWithinDays, options: [{ value: 0, label: "全部时间" }, { value: 7, label: "最近 7 天" }, { value: 30, label: "最近 30 天" }] },
        ]}
        query={query}
        memories={listItems}
        activities={activities}
        selectedMemoryId={selected?.memoryId}
        detail={selected && selectedListItem ? { ...selectedListItem, content: selected.content ?? "", sourceThreadId: selected.sourceThreadId, revision: selected.revision } : null}
        busy={!settings || busy}
        canStartDream={scope !== "project" || Boolean(projectId)}
        dreamRunning={Boolean(dreamTaskId && (dreamStatus === "queued" || dreamStatus === "running"))}
        dreamStatusLabel={dreamStatus ? memoryDreamStatusLabel(dreamStatus) : null}
        message={message}
        onToggleSwitch={(key) => void toggle(key as keyof MemorySettings)}
        onQueryChange={(value) => changeFilter(setQuery, value)}
        onFilterChange={(id: string, value: AwakenMemoryFilterValue) => {
          if (id === "scope") changeFilter(setScope, value as MemoryScope);
          else if (id === "project") changeFilter(setProjectId, String(value));
          else if (id === "type") changeFilter(setType, value as MemoryType | "");
          else if (id === "status") changeFilter(setStatus, value as typeof status);
          else if (id === "updated") changeFilter(setUpdatedWithinDays, Number(value));
        }}
        onStartDream={() => void startDream()}
        onCancelDream={() => { if (dreamTaskId) void runtimeClient.cancelMemoryDream(dreamTaskId); }}
        onOpenMemory={(memoryId, memoryProjectId) => {
          const activityMemory = activityMemories.find((entry) => entry.memory.memoryId === memoryId && entry.projectId === memoryProjectId);
          const libraryMemory = memories.find((item) => item.memoryId === memoryId);
          const memory = activityMemory?.memory ?? libraryMemory;
          if (memory) void openMemory(memory, activityMemory?.projectId ?? (memory.scope === "project" ? projectId : undefined));
        }}
        onCloseMemory={() => { setSelected(null); setSelectedMemoryProjectId(undefined); }}
        onDeleteMemory={(memoryId) => { if (selected?.memoryId === memoryId) void removeMemory(selected, selectedMemoryProjectId); }}
      />
    </AwakenMemoryLightFlowLayout>
  );
}
