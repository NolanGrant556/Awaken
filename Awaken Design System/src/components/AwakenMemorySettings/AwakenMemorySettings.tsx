import "./AwakenMemorySettings.css";
import { useEffect, useRef, useState } from "react";
import {
  BookOpenText,
  Brain,
  Check,
  ChevronDown,
  ChevronRight,
  Clock3,
  Folder,
  History,
  MessageSquare,
  RefreshCw,
  Search,
  SlidersHorizontal,
  UserRound,
} from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";

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
export type AwakenMemoryTopicKind = "user" | "feedback" | "project" | "reference";
export type AwakenMemoryListItem = {
  id: string;
  name: string;
  description: string;
  scopeLabel: string;
  typeLabel: string;
  statusLabel: string;
  status: "active" | "superseded";
  updatedLabel: string;
  kind?: AwakenMemoryTopicKind;
  projectId?: string;
};
export type AwakenMemoryDetail = AwakenMemoryListItem & {
  content: string;
  sourceThreadId: string;
  revision: string | number;
};
export type AwakenMemoryActivityItem = {
  id: string;
  kind: "memory" | "dream";
  label: string;
  title: string;
  description: string;
  timeLabel: string;
  memoryId?: string;
  status?: "active" | "superseded" | "completed" | "running" | "failed" | "cancelled" | "interrupted" | "queued";
  details?: Array<{ label: string; value: string }>;
  stats?: Array<{ label: string; value: string | number }>;
  body?: string;
  memoryProjectId?: string;
};

export type AwakenMemorySettingsProps = {
  switches: AwakenMemorySwitchItem[];
  filters: AwakenMemoryFilterItem[];
  query: string;
  memories: AwakenMemoryListItem[];
  activities?: AwakenMemoryActivityItem[];
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
  onOpenMemory?: (id: string, projectId?: string) => void;
  onCloseMemory?: () => void;
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
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopImmediatePropagation();
        setOpen(false);
      }
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

function MemoryTopicIcon({ kind = "reference" }: { kind?: AwakenMemoryTopicKind }) {
  const iconProps = { size: 18, strokeWidth: 1.8, "aria-hidden": true } as const;
  if (kind === "user") return <UserRound {...iconProps} />;
  if (kind === "feedback") return <MessageSquare {...iconProps} />;
  if (kind === "project") return <Folder {...iconProps} />;
  return <BookOpenText {...iconProps} />;
}

function MemoryScopeTabs({ filter, onChange }: { filter?: AwakenMemoryFilterItem; onChange?: (value: AwakenMemoryFilterValue) => void }) {
  if (!filter) return null;
  return (
    <div className="memory-scope-tabs" role="group" aria-label={filter.label}>
      {filter.options.map((option) => (
        <button
          type="button"
          className={option.value === filter.value ? "memory-scope-tab memory-scope-tab-active" : "memory-scope-tab"}
          aria-pressed={option.value === filter.value}
          key={option.value}
          onClick={() => onChange?.(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function isFilterActive(filter: AwakenMemoryFilterItem): boolean {
  return filter.value !== "" && filter.value !== 0;
}

const MEMORY_ACTIVITY_TITLE_LIMIT = 32;
const MEMORY_ACTIVITY_INITIAL_VISIBLE = 6;
const MEMORY_ACTIVITY_VISIBLE_STEP = 6;
const MEMORY_ACTIVITY_MAX_VISIBLE = 18;

function truncateActivityTitle(title: string): string {
  const characters = Array.from(title);
  if (characters.length <= MEMORY_ACTIVITY_TITLE_LIMIT) return title;
  return `${characters.slice(0, MEMORY_ACTIVITY_TITLE_LIMIT).join("")}.....`;
}

export function AwakenMemorySettings({
  switches,
  filters,
  query,
  memories,
  activities = [],
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
  onCloseMemory,
  onDeleteMemory,
}: AwakenMemorySettingsProps) {
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [activeActivity, setActiveActivity] = useState<AwakenMemoryActivityItem | null>(null);
  const [dismissedDetailId, setDismissedDetailId] = useState<string | null>(null);
  const [visibleActivityCount, setVisibleActivityCount] = useState(MEMORY_ACTIVITY_INITIAL_VISIBLE);
  const filterPanelRef = useRef<HTMLDivElement>(null);

  const scopeFilter = filters.find((filter) => filter.id === "scope");
  const projectFilter = filters.find((filter) => filter.id === "project");
  const secondaryFilters = filters.filter((filter) => !["scope", "project"].includes(filter.id));
  const activeFilterCount = secondaryFilters.filter(isFilterActive).length;
  const detailOpen = Boolean(detail && detail.id !== dismissedDetailId);
  const maximumVisibleActivities = Math.min(activities.length, MEMORY_ACTIVITY_MAX_VISIBLE);
  const visibleActivities = activities.slice(0, Math.min(visibleActivityCount, maximumVisibleActivities));
  const canExpandActivities = visibleActivityCount < maximumVisibleActivities;
  const nextActivityBatchSize = Math.min(MEMORY_ACTIVITY_VISIBLE_STEP, maximumVisibleActivities - visibleActivityCount);

  useEffect(() => {
    if (!filtersOpen) return;
    const closeOnOutsidePress = (event: PointerEvent) => {
      if (!filterPanelRef.current?.contains(event.target as Node)) setFiltersOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        setFiltersOpen(false);
      }
    };
    document.addEventListener("pointerdown", closeOnOutsidePress);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePress);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [filtersOpen]);

  function openMemory(memoryId: string, projectId?: string) {
    setDismissedDetailId(null);
    onOpenMemory?.(memoryId, projectId);
  }

  function closeMemoryDetail() {
    setDismissedDetailId(detail?.id ?? null);
    onCloseMemory?.();
  }

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

      <section className="memory-manager" aria-label="记忆管理">
        {message ? <div className="model-settings-message" role="status">{message}</div> : null}

        <section className="memory-manager-section memory-activity-section" aria-labelledby="memory-activity-title">
          <header className="memory-manager-section-header">
            <div>
              <h3 id="memory-activity-title">最近动态</h3>
            </div>
          </header>

          {dreamStatusLabel ? <div className="memory-dream-status" role="status"><RefreshCw size={14} aria-hidden="true" />记忆整理：{dreamStatusLabel}</div> : null}

          <div className="memory-activity-view">
            <section className="memory-activity-panel" aria-label="最近更新与整理">
              <header>
                <div><History size={17} aria-hidden="true" /><strong>最近更新与整理</strong></div>
                <div className="memory-manager-actions">
                  <button type="button" className="memory-dream-button" disabled={busy || !canStartDream || dreamRunning} onClick={onStartDream}><Brain size={15} aria-hidden="true" />更新记忆</button>
                  {dreamRunning ? <button type="button" className="memory-cancel-button" onClick={onCancelDream}>取消</button> : null}
                </div>
              </header>
              <div className="memory-activity-list">
                {visibleActivities.map((activity) => (
                  <button type="button" className="memory-activity-row" key={activity.id} aria-haspopup="dialog" aria-label={`${activity.label} ${activity.title} ${activity.timeLabel}`} onClick={() => setActiveActivity(activity)}>
                    <span className={`memory-activity-marker memory-activity-marker-${activity.kind}`}>{activity.kind === "dream" ? <RefreshCw size={15} aria-hidden="true" /> : <Brain size={15} aria-hidden="true" />}</span>
                    <span className="memory-activity-copy"><small>{activity.label}</small><strong>{truncateActivityTitle(activity.title)}</strong></span>
                    <time>{activity.timeLabel}</time>
                    <ChevronRight size={16} aria-hidden="true" />
                  </button>
                ))}
                {!activities.length ? <div className="memory-empty"><Clock3 size={22} aria-hidden="true" /><span>暂无最近动态</span></div> : null}
                {activities.length > MEMORY_ACTIVITY_INITIAL_VISIBLE ? (
                  <div className="memory-activity-list-controls">
                    <button
                      type="button"
                      className="memory-activity-list-toggle"
                      aria-expanded={visibleActivityCount > MEMORY_ACTIVITY_INITIAL_VISIBLE}
                      onClick={() => {
                        if (canExpandActivities) {
                          setVisibleActivityCount((current) => Math.min(
                            current + MEMORY_ACTIVITY_VISIBLE_STEP,
                            MEMORY_ACTIVITY_MAX_VISIBLE,
                            activities.length,
                          ));
                          return;
                        }
                        setVisibleActivityCount(MEMORY_ACTIVITY_INITIAL_VISIBLE);
                      }}
                    >
                      {canExpandActivities ? `显示更多 · ${nextActivityBatchSize} 条` : "收起"}
                    </button>
                  </div>
                ) : null}
              </div>
            </section>
          </div>
        </section>

        <section className="memory-manager-section memory-library-section" aria-labelledby="memory-library-title">
          <header className="memory-manager-section-header">
            <div>
              <h3 id="memory-library-title">记忆库</h3>
              <p>搜索、筛选并查看跨任务保存的主题记忆。</p>
            </div>
          </header>

          <div className="memory-library">
            <div className="memory-search-row">
              <label className="memory-search"><Search size={17} aria-hidden="true" /><input aria-label="搜索记忆" placeholder="搜索记忆" value={query} onChange={(event) => onQueryChange?.(event.target.value)} /></label>
              <div className="memory-filter-panel-root" ref={filterPanelRef}>
                <button type="button" className={`memory-filter-panel-trigger${filtersOpen ? " memory-filter-panel-trigger-open" : ""}`} aria-expanded={filtersOpen} aria-haspopup="dialog" onClick={() => setFiltersOpen((current) => !current)}>
                  <SlidersHorizontal size={16} aria-hidden="true" />筛选{activeFilterCount ? ` ${activeFilterCount}` : ""}
                </button>
                {filtersOpen ? (
                  <div className="memory-filter-panel" role="dialog" aria-label="筛选记忆">
                    {secondaryFilters.map((filter) => (
                      <div className="memory-filter-panel-row" key={filter.id}>
                        <span>{filter.label}</span>
                        <AwakenMemoryFilterSelect filter={filter} onChange={(value) => onFilterChange?.(filter.id, value)} />
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            </div>

            <div className="memory-library-scope-row">
              <MemoryScopeTabs filter={scopeFilter} onChange={(value) => scopeFilter && onFilterChange?.(scopeFilter.id, value)} />
              {projectFilter ? <AwakenMemoryFilterSelect filter={projectFilter} onChange={(value) => onFilterChange?.(projectFilter.id, value)} /> : null}
            </div>

            <div className="memory-topic-list" aria-label="记忆列表">
              {memories.map((memory) => (
                <article className={`memory-topic-row${selectedMemoryId === memory.id ? " memory-topic-row-selected" : ""}`} key={memory.id}>
                  <button type="button" aria-haspopup="dialog" onClick={() => openMemory(memory.id, memory.projectId)}>
                    <span className="memory-topic-icon"><MemoryTopicIcon kind={memory.kind} /></span>
                    <span className="memory-topic-copy">
                      <strong>{memory.name}</strong>
                      <span>{memory.description}</span>
                      <small><span>{memory.scopeLabel}</span><i aria-hidden="true" /><span>{memory.typeLabel}</span><i aria-hidden="true" /><span className={memory.status === "superseded" ? "memory-status-superseded" : "memory-status-active"}>{memory.statusLabel}</span><i aria-hidden="true" /><span>{memory.updatedLabel}</span></small>
                    </span>
                    <span className="memory-topic-action">查看<ChevronRight size={17} aria-hidden="true" /></span>
                  </button>
                </article>
              ))}
              {!memories.length ? <div className="memory-empty"><BookOpenText size={22} aria-hidden="true" /><span>暂无符合条件的记忆</span></div> : null}
            </div>
          </div>
        </section>
      </section>

      <AwakenDialog
        open={detailOpen}
        title={detail?.name ?? "记忆详情"}
        size="large"
        className="memory-detail-dialog"
        bodyClassName="memory-detail-dialog-body"
        onOpenChange={(open) => { if (!open) closeMemoryDetail(); }}
        footer={detail ? (
          <div className="memory-detail-dialog-actions">
            <button type="button" className="memory-detail-delete" onClick={() => onDeleteMemory?.(detail.id)}>删除记忆</button>
            <AwakenButton onClick={closeMemoryDetail}>关闭</AwakenButton>
          </div>
        ) : undefined}
      >
        {detail ? (
          <div className="memory-detail-dialog-content">
            <p className="memory-detail-description">{detail.description}</p>
            <div className="memory-detail-tags" aria-label={`${detail.scopeLabel}，${detail.typeLabel}，${detail.statusLabel}，${detail.updatedLabel}`}>
              <span>{detail.scopeLabel}</span><i aria-hidden="true" />
              <span>{detail.typeLabel}</span><i aria-hidden="true" />
              <span className={detail.status === "superseded" ? "memory-detail-tag-superseded" : "memory-detail-tag-active"}>{detail.statusLabel}</span><i aria-hidden="true" />
              <span>{detail.updatedLabel}</span>
            </div>
            <div className="memory-detail-document aw-text-selectable">
              <AwakenMarkdownRenderer markdown={detail.content} fallback="这条记忆没有正文。" renderImages={false} renderMermaid={false} allowedLinkProtocols={["http", "https"]} />
            </div>
            <dl className="memory-detail-meta"><dt>来源任务</dt><dd>{detail.sourceThreadId}</dd><dt>Revision</dt><dd>{detail.revision}</dd></dl>
          </div>
        ) : null}
      </AwakenDialog>

      <AwakenDialog
        open={Boolean(activeActivity)}
        title="动态详情"
        className="memory-activity-dialog"
        onOpenChange={(open) => { if (!open) setActiveActivity(null); }}
        footer={activeActivity ? (
          <div className="memory-activity-dialog-actions">
            {activeActivity.memoryId ? <AwakenButton variant="primary" onClick={() => { const memoryId = activeActivity.memoryId; const projectId = activeActivity.memoryProjectId; setActiveActivity(null); if (memoryId) openMemory(memoryId, projectId); }}>查看完整记忆</AwakenButton> : null}
            <AwakenButton onClick={() => setActiveActivity(null)}>关闭</AwakenButton>
          </div>
        ) : undefined}
      >
        {activeActivity ? (
          <div className="memory-activity-dialog-content">
            <div className="memory-activity-dialog-heading"><span>{activeActivity.label}</span><time>{activeActivity.timeLabel}</time></div>
            <h3>{activeActivity.title}</h3>
            <p>{activeActivity.description}</p>
            {activeActivity.body ? <pre className="aw-text-selectable">{activeActivity.body}</pre> : null}
            {activeActivity.details?.length ? <dl>{activeActivity.details.map((item) => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl> : null}
          </div>
        ) : null}
      </AwakenDialog>
    </section>
  );
}
