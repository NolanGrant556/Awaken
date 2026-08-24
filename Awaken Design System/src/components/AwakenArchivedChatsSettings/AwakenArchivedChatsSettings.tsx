import "./AwakenArchivedChatsSettings.css";
import type { ReactNode } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenIconButton } from "../AwakenIconButton";
import { AwakenSelect, type AwakenSelectOption } from "../AwakenSelect";

// 行与组数据全部是人类可读字符串，由调用方（Desktop 归档面板）完成分组、排序、
// 搜索过滤与日期格式化；组件不读取 Runtime、Electron、FileStore 或项目 store。
export type AwakenArchivedChatRow = {
  threadId: string;
  title: string;
  /** 归档日期显示串（Desktop 当前 locale）。 */
  archivedAtLabel: string;
  /** 辅助技术用的完整归档日期时间。 */
  archivedAtTitle?: string;
};

export type AwakenArchivedChatGroup = {
  id: string;
  /** 组标题：项目名、「对话」或「未知项目（ID 后 8 位）」。 */
  label: string;
  rows: AwakenArchivedChatRow[];
};

export type AwakenArchivedChatsSettingsProps = {
  /** 已按当前搜索与项目筛选过滤后的可见分组；空组不应传入。 */
  groups: AwakenArchivedChatGroup[];
  query: string;
  onQueryChange?: (query: string) => void;
  /** 项目筛选项，第一项应为「所有项目」。 */
  projectFilterOptions: readonly AwakenSelectOption[];
  projectFilterValue: string;
  onProjectFilterChange?: (value: string) => void;
  /**
   * 当前搜索/项目筛选是否处于生效状态。groups 为空时用它区分
   * 「还没有已归档的聊天」（空库）与「没有匹配的已归档聊天」（被条件排除）。
   */
  filtersActive?: boolean;
  /** 正在进行取消归档/删除的 thread id，对应行的操作按钮禁用。 */
  busyThreadIds?: readonly string[];
  loading?: boolean;
  /** 载入失败文案；非空时渲染可重试错误态，不伪装成空库。 */
  errorMessage?: string | null;
  onUnarchive?: (threadId: string) => void;
  /** 只暴露删除意图；确认 Dialog 由 Desktop 侧负责。 */
  onDelete?: (threadId: string) => void;
  onRetry?: () => void;
  onClearFilters?: () => void;
};

export function AwakenArchivedChatsSettings({
  groups,
  query,
  onQueryChange,
  projectFilterOptions,
  projectFilterValue,
  onProjectFilterChange,
  filtersActive = false,
  busyThreadIds = [],
  loading = false,
  errorMessage = null,
  onUnarchive,
  onDelete,
  onRetry,
  onClearFilters,
}: AwakenArchivedChatsSettingsProps) {
  const busy = new Set(busyThreadIds);

  let body: ReactNode;
  if (loading) {
    body = (
      <div className="aw-archived-chats__status" role="status">
        正在加载已归档的聊天…
      </div>
    );
  } else if (errorMessage) {
    body = (
      <div className="aw-archived-chats__status" role="alert">
        <p className="aw-archived-chats__status-text">{errorMessage}</p>
        <AwakenButton variant="default" size="sm" onClick={onRetry}>
          重试
        </AwakenButton>
      </div>
    );
  } else if (groups.length === 0) {
    body = filtersActive ? (
      <div className="aw-archived-chats__status">
        <p className="aw-archived-chats__status-text">没有匹配的已归档聊天</p>
        <AwakenButton variant="default" size="sm" onClick={onClearFilters}>
          清除搜索和筛选条件
        </AwakenButton>
      </div>
    ) : (
      <div className="aw-archived-chats__status">
        <p className="aw-archived-chats__status-text">还没有已归档的聊天</p>
      </div>
    );
  } else {
    body = (
      <div className="aw-archived-chats__groups">
        {groups.map((group) => (
          <section
            key={group.id}
            className="aw-archived-chats__group"
            aria-label={group.label}
          >
            <h3 className="aw-archived-chats__group-title">{group.label}</h3>
            <ul className="aw-archived-chats__list">
              {group.rows.map((row) => {
                const rowBusy = busy.has(row.threadId);
                return (
                  <li className="aw-archived-chats__row" key={row.threadId}>
                    <div className="aw-archived-chats__row-main">
                      <span className="aw-archived-chats__row-title" title={row.title}>
                        {row.title}
                      </span>
                      <span
                        className="aw-archived-chats__row-date"
                        title={row.archivedAtTitle}
                        aria-label={
                          row.archivedAtTitle ? `归档时间 ${row.archivedAtTitle}` : undefined
                        }
                      >
                        {row.archivedAtLabel}
                      </span>
                    </div>
                    <div className="aw-archived-chats__row-actions">
                      <AwakenButton
                        variant="default"
                        size="sm"
                        disabled={rowBusy}
                        onClick={() => onUnarchive?.(row.threadId)}
                      >
                        取消归档
                      </AwakenButton>
                      <AwakenIconButton
                        label="删除会话"
                        size="sm"
                        disabled={rowBusy}
                        style={{ color: "var(--aw-status-danger)" }}
                        onClick={() => onDelete?.(row.threadId)}
                      >
                        <AwakenIcon concept="delete" size={16} />
                      </AwakenIconButton>
                    </div>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
      </div>
    );
  }

  return (
    <section className="aw-archived-chats" aria-label="已归档的聊天">
      <div className="aw-archived-chats__toolbar">
        <label className="aw-archived-chats__search">
          <AwakenIcon concept="search" size={14} />
          <input
            type="search"
            aria-label="搜索已归档聊天"
            placeholder="搜索已归档聊天"
            value={query}
            onChange={(event) => onQueryChange?.(event.target.value)}
          />
        </label>
        <AwakenSelect
          className="aw-archived-chats__project-filter"
          value={projectFilterValue}
          options={projectFilterOptions}
          ariaLabel="项目筛选"
          onValueChange={(value) => onProjectFilterChange?.(value)}
        />
      </div>
      {body}
    </section>
  );
}
