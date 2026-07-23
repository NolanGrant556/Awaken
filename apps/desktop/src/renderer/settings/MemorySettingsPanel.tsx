import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AwakenMemoryLightFlowLayout,
  AwakenMemorySettings,
  type AwakenMemoryActivityItem,
  type AwakenMemoryLibrarySummary,
  type AwakenMemoryProjectSummary,
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
type MemoryLibraryPresentation = {
  summary: AwakenMemoryLibrarySummary;
  projects: AwakenMemoryProjectSummary[];
};

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

export function buildMemoryLibraryPresentation(
  memories: RecentMemoryEntry[],
  projects: RuntimeProject[],
  now = Date.now(),
): MemoryLibraryPresentation {
  const userMemories = memories.filter((entry) => !entry.projectId);
  const projectMemories = new Map<string, RecentMemoryEntry[]>();
  memories.forEach((entry) => {
    if (!entry.projectId) return;
    const existing = projectMemories.get(entry.projectId) ?? [];
    existing.push(entry);
    projectMemories.set(entry.projectId, existing);
  });

  const latestUpdatedLabel = (entries: RecentMemoryEntry[]): string | undefined => {
    const latest = entries.reduce<number>((timestamp, entry) => {
      const candidate = new Date(entry.memory.updatedAt).getTime();
      return Number.isFinite(candidate) ? Math.max(timestamp, candidate) : timestamp;
    }, 0);
    return latest > 0 ? memoryRelativeTime(new Date(latest).toISOString(), now) : undefined;
  };

  const memoryProjects = projects.flatMap<AwakenMemoryProjectSummary>((project) => {
    const entries = projectMemories.get(project.projectId) ?? [];
    if (!entries.length) return [];
    return [{
      id: project.projectId,
      name: project.displayName,
      topicCount: entries.length,
      topicCountCapped: entries.length >= 20,
      updatedLabel: latestUpdatedLabel(entries),
    }];
  }).sort((left, right) => {
    const leftEntries = projectMemories.get(left.id) ?? [];
    const rightEntries = projectMemories.get(right.id) ?? [];
    const latest = (entries: RecentMemoryEntry[]) => entries.reduce((value, entry) => Math.max(value, new Date(entry.memory.updatedAt).getTime() || 0), 0);
    return latest(rightEntries) - latest(leftEntries);
  });
  const projectTopicCount = memoryProjects.reduce((total, project) => total + project.topicCount, 0);
  const allProjectMemories = memories.filter((entry) => Boolean(entry.projectId));

  return {
    summary: {
      userTopicCount: userMemories.length,
      userTopicCountCapped: userMemories.length >= 20,
      userUpdatedLabel: latestUpdatedLabel(userMemories),
      projectCount: memoryProjects.length,
      projectTopicCount,
      projectTopicCountCapped: memoryProjects.some((project) => project.topicCountCapped),
      projectUpdatedLabel: latestUpdatedLabel(allProjectMemories),
    },
    projects: memoryProjects,
  };
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
  const [memories, setMemories] = useState<RuntimeMemory[]>([]);
  const [libraryLoading, setLibraryLoading] = useState(true);
  const [libraryReloadKey, setLibraryReloadKey] = useState(0);
  const [activityMemories, setActivityMemories] = useState<RecentMemoryEntry[]>([]);
  const [activityDreamTasks, setActivityDreamTasks] = useState<MemoryDreamTask[]>([]);
  const [selected, setSelected] = useState<RuntimeMemory | null>(null);
  const [selectedMemoryProjectId, setSelectedMemoryProjectId] = useState<string | undefined>();
  const [busySwitchId, setBusySwitchId] = useState<keyof MemorySettings | null>(null);
  const [dreamBusy, setDreamBusy] = useState(false);
  const [dreamTaskId, setDreamTaskId] = useState<string | null>(null);
  const [dreamStatus, setDreamStatus] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const loadRequestId = useRef(0);
  const activityLoadRequestId = useRef(0);

  const loadMemories = useCallback(async () => {
    const requestId = ++loadRequestId.current;
    setLibraryLoading(true);
    if (scope === "project" && !projectId) {
      setMemories([]);
      setSelected(null);
      setLibraryLoading(false);
      return;
    }
    try {
      const items = await runtimeClient.listMemories({ scope, projectId: scope === "project" ? projectId : undefined, limit: 20 });
      if (requestId !== loadRequestId.current) return;
      setMemories(items);
      setSelected((current) => current && !items.some((item) => item.memoryId === current.memoryId) ? null : current);
    } catch (error) {
      if (requestId !== loadRequestId.current) return;
      setMessage(error instanceof Error ? error.message : "无法读取记忆。");
    } finally {
      if (requestId === loadRequestId.current) setLibraryLoading(false);
    }
  }, [libraryReloadKey, projectId, runtimeClient, scope]);

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

  function openLibraryScope(nextScope: MemoryScope, nextProjectId?: string) {
    loadRequestId.current += 1;
    setSelected(null);
    setSelectedMemoryProjectId(undefined);
    setMemories([]);
    setLibraryLoading(true);
    setScope(nextScope);
    setProjectId(nextScope === "project" ? nextProjectId ?? "" : "");
    setLibraryReloadKey((current) => current + 1);
  }

  async function toggle(key: keyof MemorySettings) {
    if (!settings || busySwitchId) return;
    if (key !== "enabled" && !settings.enabled) return;
    setBusySwitchId(key);
    try {
      const document = await runtimeClient.updateMemorySettings(key === "enabled"
        ? settings.enabled ? {
            expectedRevision: revision,
            enabled: false,
            automaticRecall: false,
            proactiveMemory: false,
            autoDream: false,
          } : {
            expectedRevision: revision,
            enabled: true,
            automaticRecall: true,
            proactiveMemory: true,
            autoDream: true,
          }
        : { expectedRevision: revision, [key]: !settings[key] });
      setSettings(document.value);
      setRevision(document.revision);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "更新记忆设置失败。");
    } finally { setBusySwitchId(null); }
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
    if (!settings?.enabled) return;
    setDreamBusy(true);
    try {
      const task = await runtimeClient.startMemoryDream(scope === "project" ? projectId : undefined);
      setDreamTaskId(task.taskId);
      setDreamStatus(task.status);
      setActivityDreamTasks((current) => [task, ...current.filter((item) => item.taskId !== task.taskId)]);
    } catch (error) { setMessage(error instanceof Error ? error.message : "启动 Dream 失败。"); }
    finally { setDreamBusy(false); }
  }

  const listItems = memories.map((memory) => toMemoryListItem(memory, projectId));
  const libraryPresentation = useMemo(
    () => buildMemoryLibraryPresentation(activityMemories, projects),
    [activityMemories, projects],
  );

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
          checked: item.key === "enabled"
            ? settings?.enabled ?? false
            : Boolean(settings?.enabled && settings[item.key]),
          disabled: item.key !== "enabled" && settings?.enabled === false,
        }))}
        memories={listItems}
        librarySummary={libraryPresentation.summary}
        memoryProjects={libraryPresentation.projects}
        libraryLoading={libraryLoading}
        activities={activities}
        selectedMemoryId={selected?.memoryId}
        detail={selected && selectedListItem ? { ...selectedListItem, content: selected.content ?? "", sourceThreadId: selected.sourceThreadId, revision: selected.revision } : null}
        busy={!settings}
        busySwitchId={busySwitchId}
        dreamBusy={dreamBusy}
        canStartDream={Boolean(settings?.enabled) && (scope !== "project" || Boolean(projectId))}
        dreamRunning={Boolean(dreamTaskId && (dreamStatus === "queued" || dreamStatus === "running"))}
        dreamStatusLabel={dreamStatus ? memoryDreamStatusLabel(dreamStatus) : null}
        message={message}
        onToggleSwitch={(key) => void toggle(key as keyof MemorySettings)}
        onOpenLibraryScope={(nextScope, nextProjectId) => openLibraryScope(nextScope, nextProjectId)}
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
