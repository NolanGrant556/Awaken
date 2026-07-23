import { useCallback, useEffect, useRef, useState } from "react";
import {
  AwakenMemoryLightFlowLayout,
  AwakenMemorySettings,
  type AwakenMemoryFilterValue,
} from "@awaken/design-system";
import type {
  AwakenTheme,
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
  } as Record<string, string>)[status] ?? "状态未知";
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
  const [selected, setSelected] = useState<RuntimeMemory | null>(null);
  const [busy, setBusy] = useState(false);
  const [dreamTaskId, setDreamTaskId] = useState<string | null>(null);
  const [dreamStatus, setDreamStatus] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const loadRequestId = useRef(0);

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
    setMemories([]);
    void loadMemories();
  }, [loadMemories]);

  useEffect(() => {
    if (!dreamTaskId) return;
    const timer = window.setInterval(() => {
      void runtimeClient.getMemoryDream(dreamTaskId).then((task) => {
        setDreamStatus(task.status);
        if (["completed", "failed", "cancelled"].includes(task.status)) {
          window.clearInterval(timer);
          void loadMemories();
        }
      }).catch((error: unknown) => {
        window.clearInterval(timer);
        setMessage(error instanceof Error ? error.message : "无法读取 Dream 状态。");
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [dreamTaskId, loadMemories, runtimeClient]);

  function changeFilter<T>(setter: (value: T) => void, value: T) {
    loadRequestId.current += 1;
    setSelected(null);
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

  async function openMemory(memory: RuntimeMemory) {
    const requestId = loadRequestId.current;
    try {
      const detail = await runtimeClient.getMemory(memory.memoryId, memory.scope === "project" ? projectId : undefined);
      if (requestId === loadRequestId.current) setSelected(detail);
    } catch (error) {
      if (requestId === loadRequestId.current) setMessage(error instanceof Error ? error.message : "无法读取记忆正文。");
    }
  }

  async function removeMemory(memory: RuntimeMemory) {
    if (!window.confirm(`确定删除记忆“${memory.name}”吗？此操作不可撤销。`)) return;
    try {
      await runtimeClient.deleteMemory(memory.memoryId, memory.revision, memory.scope === "project" ? projectId : undefined);
      if (selected?.memoryId === memory.memoryId) setSelected(null);
      await loadMemories();
    } catch (error) { setMessage(error instanceof Error ? error.message : "删除记忆失败。"); }
  }

  async function startDream() {
    setBusy(true);
    try {
      const task = await runtimeClient.startMemoryDream(scope === "project" ? projectId : undefined);
      setDreamTaskId(task.taskId);
      setDreamStatus(task.status);
    } catch (error) { setMessage(error instanceof Error ? error.message : "启动 Dream 失败。"); }
    finally { setBusy(false); }
  }

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
        memories={memories.map((memory) => ({ id: memory.memoryId, name: memory.name, description: memory.description, meta: `${memory.scope} · ${memory.type} · ${new Date(memory.updatedAt).toLocaleString()}` }))}
        selectedMemoryId={selected?.memoryId}
        detail={selected ? { name: selected.name, description: selected.description, content: selected.content ?? "", sourceThreadId: selected.sourceThreadId, revision: selected.revision } : null}
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
        onOpenMemory={(memoryId) => { const memory = memories.find((item) => item.memoryId === memoryId); if (memory) void openMemory(memory); }}
        onDeleteMemory={(memoryId) => { const memory = memories.find((item) => item.memoryId === memoryId); if (memory) void removeMemory(memory); }}
      />
    </AwakenMemoryLightFlowLayout>
  );
}
