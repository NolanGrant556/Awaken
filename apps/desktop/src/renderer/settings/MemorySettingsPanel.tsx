import { useCallback, useEffect, useRef, useState } from "react";
import { Brain, Check, ChevronDown, Search, Trash2 } from "lucide-react";
import type {
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

type FilterValue = string | number;

type FilterOption<T extends FilterValue> = {
  value: T;
  label: string;
};

function MemoryFilterSelect<T extends FilterValue>({
  label,
  value,
  options,
  onChange,
  placeholder = "（待选择）",
  disabled = false,
}: {
  label: string;
  value: T;
  options: Array<FilterOption<T>>;
  onChange: (value: T) => void;
  placeholder?: string;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selectedLabel = options.find((option) => option.value === value)?.label ?? placeholder;

  useEffect(() => {
    if (!open) return;
    const closeOnOutsidePress = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutsidePress);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePress);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  return (
    <div className="memory-filter" ref={rootRef}>
      <button
        type="button"
        className="memory-filter-trigger"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
      >
        <span>{selectedLabel}</span>
        <ChevronDown size={15} aria-hidden="true" />
      </button>
      {open ? (
        <div className="memory-filter-menu" role="listbox" aria-label={label}>
          {options.map((option) => (
            <button
              type="button"
              className={`memory-filter-option${option.value === value ? " memory-filter-option-selected" : ""}`}
              role="option"
              aria-selected={option.value === value}
              key={option.value}
              onClick={() => {
                onChange(option.value);
                setOpen(false);
              }}
            >
              <span>{option.label}</span>
              <Check size={14} aria-hidden="true" />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function memoryDreamStatusLabel(status: string): string {
  return ({
    queued: "等待整理",
    running: "正在整理",
    completed: "整理完成",
    failed: "整理失败",
    cancelled: "已取消",
  } as Record<string, string>)[status] ?? "状态未知";
}

export function MemorySettingsPanel({ runtimeClient, currentProjectId }: { runtimeClient: RuntimeClient; currentProjectId?: string }) {
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
    <section className="memory-settings" aria-label="记忆">
      <div className="memory-settings-switches">
        {switchCopy.map((item) => (
          <div className="memory-settings-switch-row" key={item.key}>
            <div><strong>{item.title}</strong><p>{item.description}</p></div>
            <button type="button" className={`mcp-toggle-switch${settings?.[item.key] ? " mcp-toggle-switch-on" : " mcp-toggle-switch-off"}`} aria-label={`${settings?.[item.key] ? "关闭" : "开启"}${item.title}`} aria-pressed={settings?.[item.key] ?? false} disabled={!settings || busy} onClick={() => void toggle(item.key)}><span className="mcp-toggle-switch-thumb" /></button>
          </div>
        ))}
      </div>

      <div className="memory-settings-toolbar">
        <div className="memory-search"><Search size={16} /><input aria-label="搜索记忆" placeholder="搜索记忆" value={query} onChange={(event) => changeFilter(setQuery, event.target.value)} /></div>
        <MemoryFilterSelect label="记忆范围" value={scope} options={[{ value: "user", label: "用户记忆" }, { value: "project", label: "项目记忆" }]} onChange={(next) => changeFilter(setScope, next as MemoryScope)} />
        {scope === "project" ? <MemoryFilterSelect label="项目" value={projectId} options={projects.map((project) => ({ value: project.projectId, label: project.displayName }))} placeholder="（项目待选择）" disabled={!projects.length} onChange={(next) => changeFilter(setProjectId, next)} /> : null}
        <MemoryFilterSelect label="记忆类型" value={type} options={[{ value: "", label: "全部类型" }, { value: "user", label: "用户" }, { value: "feedback", label: "反馈" }, { value: "project", label: "项目" }, { value: "reference", label: "参考" }]} onChange={(next) => changeFilter(setType, next as MemoryType | "")} />
        <MemoryFilterSelect label="记忆状态" value={status} options={[{ value: "", label: "全部状态" }, { value: "active", label: "有效" }, { value: "superseded", label: "已替代" }]} onChange={(next) => changeFilter(setStatus, next as typeof status)} />
        <MemoryFilterSelect label="更新时间" value={updatedWithinDays} options={[{ value: 0, label: "全部时间" }, { value: 7, label: "最近 7 天" }, { value: 30, label: "最近 30 天" }]} onChange={(next) => changeFilter(setUpdatedWithinDays, next)} />
        <button type="button" className="memory-dream-button" disabled={busy || (scope === "project" && !projectId) || dreamStatus === "queued" || dreamStatus === "running"} onClick={() => void startDream()}><Brain size={16} />立即整理</button>
        {dreamTaskId && (dreamStatus === "queued" || dreamStatus === "running") ? <button type="button" onClick={() => void runtimeClient.cancelMemoryDream(dreamTaskId)}>取消</button> : null}
      </div>
      {dreamStatus ? <div className="memory-dream-status" role="status">记忆整理：{memoryDreamStatusLabel(dreamStatus)}</div> : null}
      {message ? <div className="model-settings-message" role="status">{message}</div> : null}

      <div className="memory-settings-content">
        <div className="memory-list" aria-label="记忆列表">
          {memories.map((memory) => <article className={`memory-list-item${selected?.memoryId === memory.memoryId ? " memory-list-item-selected" : ""}`} key={memory.memoryId}><button type="button" onClick={() => void openMemory(memory)}><strong>{memory.name}</strong><span>{memory.description}</span><small>{memory.scope} · {memory.type} · {new Date(memory.updatedAt).toLocaleString()}</small></button><button type="button" className="memory-delete-button" aria-label={`删除 ${memory.name}`} onClick={() => void removeMemory(memory)}><Trash2 size={15} /></button></article>)}
          {!memories.length ? <div className="memory-empty">暂无符合条件的记忆</div> : null}
        </div>
        <div className="memory-detail" aria-label="记忆详情">
          {selected ? <><h2>{selected.name}</h2><p className="memory-detail-description">{selected.description}</p><pre>{selected.content}</pre><dl><dt>来源任务</dt><dd>{selected.sourceThreadId}</dd><dt>Revision</dt><dd>{selected.revision}</dd></dl></> : <div className="memory-empty">选择一条记忆查看正文</div>}
        </div>
      </div>
    </section>
  );
}
