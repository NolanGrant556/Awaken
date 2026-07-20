import "./AwakenMemorySettings.css";
import { useEffect, useRef, useState } from "react";
import { Brain, Check, ChevronDown, Search, Trash2 } from "lucide-react";

export type AwakenMemorySwitchItem = { id: string; title: string; description: string; checked: boolean };
export type AwakenMemoryFilterValue = string | number;
export type AwakenMemoryFilterItem = {
  id: string;
  label: string;
  value: AwakenMemoryFilterValue;
  options: Array<{ value: AwakenMemoryFilterValue; label: string }>;
  placeholder?: string;
  disabled?: boolean;
};
export type AwakenMemoryListItem = { id: string; name: string; description: string; meta: string };
export type AwakenMemoryDetail = { name: string; description: string; content: string; sourceThreadId: string; revision: string | number };

export type AwakenMemorySettingsProps = {
  switches: AwakenMemorySwitchItem[];
  filters: AwakenMemoryFilterItem[];
  query: string;
  memories: AwakenMemoryListItem[];
  selectedMemoryId?: string | null;
  detail?: AwakenMemoryDetail | null;
  busy?: boolean;
  canStartDream?: boolean;
  dreamRunning?: boolean;
  dreamStatusLabel?: string | null;
  message?: string | null;
  onToggleSwitch?: (id: string) => void;
  onQueryChange?: (query: string) => void;
  onFilterChange?: (id: string, value: AwakenMemoryFilterValue) => void;
  onStartDream?: () => void;
  onCancelDream?: () => void;
  onOpenMemory?: (id: string) => void;
  onDeleteMemory?: (id: string) => void;
};

function AwakenMemoryFilterSelect({ filter, onChange }: { filter: AwakenMemoryFilterItem; onChange?: (value: AwakenMemoryFilterValue) => void }) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selectedLabel = filter.options.find((option) => option.value === filter.value)?.label ?? filter.placeholder ?? "（待选择）";

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
      <button type="button" className="memory-filter-trigger" aria-label={filter.label} aria-haspopup="listbox" aria-expanded={open} disabled={filter.disabled} onClick={() => setOpen((current) => !current)}>
        <span>{selectedLabel}</span><ChevronDown size={15} aria-hidden="true" />
      </button>
      {open ? (
        <div className="memory-filter-menu" role="listbox" aria-label={filter.label}>
          {filter.options.map((option) => (
            <button type="button" className={`memory-filter-option${option.value === filter.value ? " memory-filter-option-selected" : ""}`} role="option" aria-selected={option.value === filter.value} key={option.value} onClick={() => { onChange?.(option.value); setOpen(false); }}>
              <span>{option.label}</span><Check size={14} aria-hidden="true" />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function AwakenMemorySettings({
  switches,
  filters,
  query,
  memories,
  selectedMemoryId,
  detail,
  busy = false,
  canStartDream = true,
  dreamRunning = false,
  dreamStatusLabel,
  message,
  onToggleSwitch,
  onQueryChange,
  onFilterChange,
  onStartDream,
  onCancelDream,
  onOpenMemory,
  onDeleteMemory,
}: AwakenMemorySettingsProps) {
  return (
    <section className="memory-settings" aria-label="记忆">
      <div className="memory-settings-switches">
        {switches.map((item) => (
          <div className="memory-settings-switch-row" key={item.id}>
            <div><strong>{item.title}</strong><p>{item.description}</p></div>
            <button type="button" className={`mcp-toggle-switch${item.checked ? " mcp-toggle-switch-on" : " mcp-toggle-switch-off"}`} aria-label={`${item.checked ? "关闭" : "开启"}${item.title}`} aria-pressed={item.checked} disabled={busy} onClick={() => onToggleSwitch?.(item.id)}><span className="mcp-toggle-switch-thumb" /></button>
          </div>
        ))}
      </div>

      <div className="memory-settings-toolbar">
        <div className="memory-search"><Search size={16} /><input aria-label="搜索记忆" placeholder="搜索记忆" value={query} onChange={(event) => onQueryChange?.(event.target.value)} /></div>
        {filters.map((filter) => <AwakenMemoryFilterSelect key={filter.id} filter={filter} onChange={(value) => onFilterChange?.(filter.id, value)} />)}
        <button type="button" className="memory-dream-button" disabled={busy || !canStartDream || dreamRunning} onClick={onStartDream}><Brain size={16} />立即整理</button>
        {dreamRunning ? <button type="button" onClick={onCancelDream}>取消</button> : null}
      </div>
      {dreamStatusLabel ? <div className="memory-dream-status" role="status">记忆整理：{dreamStatusLabel}</div> : null}
      {message ? <div className="model-settings-message" role="status">{message}</div> : null}

      <div className="memory-settings-content">
        <div className="memory-list" aria-label="记忆列表">
          {memories.map((memory) => (
            <article className={`memory-list-item${selectedMemoryId === memory.id ? " memory-list-item-selected" : ""}`} key={memory.id}>
              <button type="button" onClick={() => onOpenMemory?.(memory.id)}><strong>{memory.name}</strong><span>{memory.description}</span><small>{memory.meta}</small></button>
              <button type="button" className="memory-delete-button" aria-label={`删除 ${memory.name}`} onClick={() => onDeleteMemory?.(memory.id)}><Trash2 size={15} /></button>
            </article>
          ))}
          {!memories.length ? <div className="memory-empty">暂无符合条件的记忆</div> : null}
        </div>
        <div className="memory-detail" aria-label="记忆详情">
          {detail ? <><h2>{detail.name}</h2><p className="memory-detail-description">{detail.description}</p><pre className="aw-text-selectable">{detail.content}</pre><dl><dt>来源任务</dt><dd>{detail.sourceThreadId}</dd><dt>Revision</dt><dd>{detail.revision}</dd></dl></> : <div className="memory-empty">选择一条记忆查看正文</div>}
        </div>
      </div>
    </section>
  );
}
