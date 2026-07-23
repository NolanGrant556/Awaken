import "./AwakenMemorySettings.css";
import { useState } from "react";
import {
  ArrowLeft,
  BookOpenText,
  Brain,
  ChevronRight,
  Clock3,
  Folder,
  History,
  MessageSquare,
  RefreshCw,
  UserRound,
} from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";
import { AwakenProjectSkillFolder } from "../AwakenProjectSkillFolder";

export type AwakenMemorySwitchItem = { id: string; title: string; description: string; checked: boolean; disabled?: boolean };
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
export type AwakenMemoryLibrarySummary = {
  userTopicCount: number;
  userTopicCountCapped?: boolean;
  userUpdatedLabel?: string;
  projectCount: number;
  projectTopicCount: number;
  projectTopicCountCapped?: boolean;
  projectUpdatedLabel?: string;
};
export type AwakenMemoryProjectSummary = {
  id: string;
  name: string;
  topicCount: number;
  topicCountCapped?: boolean;
  updatedLabel?: string;
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
  memories: AwakenMemoryListItem[];
  librarySummary?: AwakenMemoryLibrarySummary;
  memoryProjects?: AwakenMemoryProjectSummary[];
  libraryLoading?: boolean;
  activities?: AwakenMemoryActivityItem[];
  selectedMemoryId?: string | null;
  detail?: AwakenMemoryDetail | null;
  busy?: boolean;
  busySwitchId?: string | null;
  dreamBusy?: boolean;
  canStartDream?: boolean;
  dreamRunning?: boolean;
  dreamStatusLabel?: string | null;
  message?: string | null;
  onToggleSwitch?: (id: string) => void;
  onOpenLibraryScope?: (scope: "user" | "project", projectId?: string) => void;
  onStartDream?: () => void;
  onCancelDream?: () => void;
  onOpenMemory?: (id: string, projectId?: string) => void;
  onCloseMemory?: () => void;
  onDeleteMemory?: (id: string) => void;
};

function MemoryTopicIcon({ kind = "reference" }: { kind?: AwakenMemoryTopicKind }) {
  const iconProps = { size: 18, strokeWidth: 1.8, "aria-hidden": true } as const;
  if (kind === "user") return <UserRound {...iconProps} />;
  if (kind === "feedback") return <MessageSquare {...iconProps} />;
  if (kind === "project") return <Folder {...iconProps} />;
  return <BookOpenText {...iconProps} />;
}

const MEMORY_ACTIVITY_TITLE_LIMIT = 32;
const MEMORY_ACTIVITY_INITIAL_VISIBLE = 6;
const MEMORY_ACTIVITY_VISIBLE_STEP = 6;
const MEMORY_ACTIVITY_MAX_VISIBLE = 18;
const EMPTY_LIBRARY_SUMMARY: AwakenMemoryLibrarySummary = {
  userTopicCount: 0,
  projectCount: 0,
  projectTopicCount: 0,
};

type MemoryLibraryView =
  | { kind: "user" }
  | { kind: "projects" }
  | { kind: "project"; projectId: string; projectName: string }
  | null;

type MemoryLibraryDetailContext = {
  memoryId: string;
  returnView: Exclude<MemoryLibraryView, null>;
};

function truncateActivityTitle(title: string): string {
  const characters = Array.from(title);
  if (characters.length <= MEMORY_ACTIVITY_TITLE_LIMIT) return title;
  return `${characters.slice(0, MEMORY_ACTIVITY_TITLE_LIMIT).join("")}.....`;
}

function topicCountLabel(count: number, capped = false): string {
  if (capped) return `至少 ${count} 个主题`;
  return `${count} 个主题`;
}

function MemoryDetailContent({ detail }: { detail: AwakenMemoryDetail }) {
  return (
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
  );
}

export function AwakenMemorySettings({
  switches,
  memories,
  librarySummary = EMPTY_LIBRARY_SUMMARY,
  memoryProjects = [],
  libraryLoading = false,
  activities = [],
  selectedMemoryId,
  detail,
  busy = false,
  busySwitchId = null,
  dreamBusy = false,
  canStartDream = true,
  dreamRunning = false,
  dreamStatusLabel,
  message,
  onToggleSwitch,
  onOpenLibraryScope,
  onStartDream,
  onCancelDream,
  onOpenMemory,
  onCloseMemory,
  onDeleteMemory,
}: AwakenMemorySettingsProps) {
  const [libraryView, setLibraryView] = useState<MemoryLibraryView>(null);
  const [libraryDetailContext, setLibraryDetailContext] = useState<MemoryLibraryDetailContext | null>(null);
  const [activeActivity, setActiveActivity] = useState<AwakenMemoryActivityItem | null>(null);
  const [dismissedDetailId, setDismissedDetailId] = useState<string | null>(null);
  const [visibleActivityCount, setVisibleActivityCount] = useState(MEMORY_ACTIVITY_INITIAL_VISIBLE);
  const detailOpen = Boolean(detail && detail.id !== dismissedDetailId);
  const maximumVisibleActivities = Math.min(activities.length, MEMORY_ACTIVITY_MAX_VISIBLE);
  const visibleActivities = activities.slice(0, Math.min(visibleActivityCount, maximumVisibleActivities));
  const canExpandActivities = visibleActivityCount < maximumVisibleActivities;
  const nextActivityBatchSize = Math.min(MEMORY_ACTIVITY_VISIBLE_STEP, maximumVisibleActivities - visibleActivityCount);

  function openMemory(memoryId: string, projectId?: string, returnView: Exclude<MemoryLibraryView, null> | null = null) {
    setDismissedDetailId(null);
    setLibraryDetailContext(returnView ? { memoryId, returnView } : null);
    onOpenMemory?.(memoryId, projectId);
  }

  function openLibraryScope(scope: "user" | "project", projectId?: string, projectName?: string) {
    setLibraryView(scope === "user"
      ? { kind: "user" }
      : { kind: "project", projectId: projectId ?? "", projectName: projectName ?? "项目记忆" });
    onOpenLibraryScope?.(scope, projectId);
  }

  function closeLibrary() {
    if (libraryDetailContext) {
      setDismissedDetailId(libraryDetailContext.memoryId);
      setLibraryDetailContext(null);
      onCloseMemory?.();
    }
    setLibraryView(null);
  }

  function openLibraryMemory(memoryId: string, projectId?: string) {
    if (libraryView?.kind !== "project" && libraryView?.kind !== "user") return;
    const returnView: Exclude<MemoryLibraryView, null> = libraryView.kind === "project"
      ? { kind: "projects" }
      : { kind: "user" };
    openMemory(memoryId, projectId, returnView);
  }

  const libraryDetailOpen = Boolean(
    libraryDetailContext
      && detailOpen
      && detail?.id === libraryDetailContext.memoryId,
  );
  const libraryDialogTitle = libraryDetailOpen && detail
    ? detail.name
    : libraryView?.kind === "project"
    ? libraryView.projectName
    : libraryView?.kind === "user"
      ? "用户记忆"
      : "项目记忆";
  const libraryShowsTopics = libraryView?.kind === "user" || libraryView?.kind === "project";

  function closeMemoryDetail() {
    setDismissedDetailId(detail?.id ?? libraryDetailContext?.memoryId ?? null);
    if (libraryDetailContext) setLibraryView(libraryDetailContext.returnView);
    setLibraryDetailContext(null);
    onCloseMemory?.();
  }

  return (
    <section className="memory-settings" aria-label="记忆">
      <div className="memory-settings-switches">
        {switches.map((item) => (
          <div className="memory-settings-switch-row" key={item.id}>
            <div><strong>{item.title}</strong><p>{item.description}</p></div>
            <button type="button" className={`mcp-toggle-switch${item.checked ? " mcp-toggle-switch-on" : " mcp-toggle-switch-off"}`} aria-label={`${item.checked ? "关闭" : "开启"}${item.title}`} aria-pressed={item.checked} aria-busy={busySwitchId === item.id || undefined} disabled={busy || item.disabled || busySwitchId === item.id} onClick={() => onToggleSwitch?.(item.id)}><span className="mcp-toggle-switch-thumb" /></button>
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
                  <button type="button" className="memory-dream-button" disabled={dreamBusy || !canStartDream || dreamRunning} onClick={onStartDream}><Brain size={15} aria-hidden="true" />更新记忆</button>
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
            </div>
          </header>

          <div className="memory-library-overview">
            <button type="button" className="memory-library-entry" aria-haspopup="dialog" onClick={() => openLibraryScope("user")}>
              <span className="memory-library-entry-icon"><UserRound size={19} strokeWidth={1.8} aria-hidden="true" /></span>
              <span className="memory-library-entry-copy">
                <strong>用户记忆</strong>
                <span>用户资料、协作偏好与个人参考入口</span>
                <small>{topicCountLabel(librarySummary.userTopicCount, librarySummary.userTopicCountCapped)}{librarySummary.userUpdatedLabel ? ` · 最近更新 ${librarySummary.userUpdatedLabel}` : ""}</small>
              </span>
              <span className="memory-library-entry-action">查看<ChevronRight size={17} aria-hidden="true" /></span>
            </button>
            <button type="button" className="memory-library-entry" aria-haspopup="dialog" onClick={() => setLibraryView({ kind: "projects" })}>
              <span className="memory-library-entry-icon"><Folder size={19} strokeWidth={1.8} aria-hidden="true" /></span>
              <span className="memory-library-entry-copy">
                <strong>项目记忆</strong>
                <span>按工作区保存的项目背景、决策与参考入口</span>
                <small>{librarySummary.projectCount} 个项目 · {topicCountLabel(librarySummary.projectTopicCount, librarySummary.projectTopicCountCapped)}{librarySummary.projectUpdatedLabel ? ` · 最近更新 ${librarySummary.projectUpdatedLabel}` : ""}</small>
              </span>
              <span className="memory-library-entry-action">查看<ChevronRight size={17} aria-hidden="true" /></span>
            </button>
          </div>
        </section>
      </section>

      <AwakenDialog
        open={Boolean(libraryView)}
        title={libraryDialogTitle}
        size="large"
        className="memory-library-dialog"
        bodyClassName={libraryDetailOpen ? "memory-detail-dialog-body" : "memory-library-dialog-body"}
        onOpenChange={(open) => {
          if (!open) {
            if (libraryDetailOpen) closeMemoryDetail();
            else closeLibrary();
          }
        }}
        footer={libraryDetailOpen && detail ? (
          <div className="memory-detail-dialog-actions">
            <button type="button" className="memory-detail-delete" onClick={() => onDeleteMemory?.(detail.id)}>删除记忆</button>
            <AwakenButton onClick={closeMemoryDetail}>关闭</AwakenButton>
          </div>
        ) : <AwakenButton onClick={closeLibrary}>关闭</AwakenButton>}
      >
        {libraryDetailOpen && detail ? <MemoryDetailContent detail={detail} /> : null}

        {!libraryDetailOpen && libraryView?.kind === "projects" ? (
          <div className="memory-project-browser">
            <div className="memory-project-folder-grid" aria-label="已有记忆的项目">
              {memoryProjects.map((project) => (
                <AwakenProjectSkillFolder
                  key={project.id}
                  projectName={project.name}
                  countLabel={topicCountLabel(project.topicCount, project.topicCountCapped)}
                  size="compact"
                  aria-label={`查看${project.name}的项目记忆`}
                  onOpen={() => openLibraryScope("project", project.id, project.name)}
                />
              ))}
              {!memoryProjects.length ? <div className="memory-empty"><Folder size={22} aria-hidden="true" /><span>暂无产生记忆的项目</span></div> : null}
            </div>
          </div>
        ) : null}

        {!libraryDetailOpen && libraryShowsTopics ? (
          <div className="memory-topic-browser">
            {libraryView.kind === "project" ? (
              <button type="button" className="memory-library-back" onClick={() => setLibraryView({ kind: "projects" })}><ArrowLeft size={15} aria-hidden="true" />返回项目列表</button>
            ) : null}
            <div className="memory-topic-list" aria-label="记忆列表">
              {!libraryLoading ? memories.map((memory) => (
                <article className={`memory-topic-row${selectedMemoryId === memory.id ? " memory-topic-row-selected" : ""}`} key={memory.id}>
                  <button type="button" aria-haspopup="dialog" onClick={() => openLibraryMemory(memory.id, memory.projectId)}>
                    <span className="memory-topic-icon"><MemoryTopicIcon kind={memory.kind} /></span>
                    <span className="memory-topic-copy">
                      <strong>{memory.name}</strong>
                      <span>{memory.description}</span>
                      <small><span>{memory.typeLabel}</span><i aria-hidden="true" /><span className={memory.status === "superseded" ? "memory-status-superseded" : "memory-status-active"}>{memory.statusLabel}</span><i aria-hidden="true" /><span>{memory.updatedLabel}</span></small>
                    </span>
                    <span className="memory-topic-action">查看<ChevronRight size={17} aria-hidden="true" /></span>
                  </button>
                </article>
              )) : null}
              {libraryLoading ? <div className="memory-empty"><RefreshCw size={22} className="memory-loading-icon" aria-hidden="true" /><span>正在读取记忆</span></div> : null}
              {!libraryLoading && !memories.length ? <div className="memory-empty"><BookOpenText size={22} aria-hidden="true" /><span>暂无记忆</span></div> : null}
            </div>
          </div>
        ) : null}
      </AwakenDialog>

      <AwakenDialog
        open={detailOpen && !libraryDetailContext}
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
        {detail ? <MemoryDetailContent detail={detail} /> : null}
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
