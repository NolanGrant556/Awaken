import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AwakenArchivedChatsSettings,
  AwakenButton as Button,
  AwakenDialog as Dialog,
  type AwakenArchivedChatGroup,
  type AwakenArchivedChatRow,
  type AwakenSelectOption,
} from "@awaken/design-system";
import type { RuntimeProject, RuntimeThread } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

type ArchivedChatsSettingsPanelProps = {
  runtimeClient: RuntimeClient;
  /** 取消归档/删除完成后刷新 active 侧栏列表。 */
  onThreadsChanged: () => void;
};

const ALL_PROJECTS_FILTER = "all";
const CONVERSATION_GROUP_ID = "conversation";

export function unknownProjectGroupLabel(projectId: string): string {
  return `未知项目（${projectId.slice(-8)}）`;
}

function archivedChatRowFromThread(thread: RuntimeThread): AwakenArchivedChatRow {
  // Core 对缺 archived_at 的旧记录已用 updated_at 兜底；这里再防一次非法值。
  const iso = thread.archivedAt ?? thread.updatedAt;
  const date = new Date(iso);
  const valid = !Number.isNaN(date.getTime());
  return {
    threadId: thread.threadId,
    title: thread.label,
    archivedAtLabel: valid ? date.toLocaleDateString() : "",
    archivedAtTitle: valid ? date.toLocaleString() : undefined,
  };
}

/**
 * 契约 §3.3 分组：active 项目按 FileStore 项目顺序，removed 项目随后，
 * 空 projectId 用「对话」组，无法解析的非空 projectId 各自独立成组并按
 * projectId 稳定排序。空组不渲染；组内顺序保持 Core 返回顺序。
 */
export function groupArchivedThreadsByProject(
  threads: RuntimeThread[],
  projects: RuntimeProject[],
): AwakenArchivedChatGroup[] {
  const projectById = new Map(projects.map((project) => [project.projectId, project]));
  const buckets = new Map<string, RuntimeThread[]>();
  for (const thread of threads) {
    const key = thread.projectId ? thread.projectId : CONVERSATION_GROUP_ID;
    const bucket = buckets.get(key);
    if (bucket) {
      bucket.push(thread);
    } else {
      buckets.set(key, [thread]);
    }
  }

  const groups: AwakenArchivedChatGroup[] = [];
  const pushProjectGroup = (project: RuntimeProject) => {
    const rows = buckets.get(project.projectId);
    if (!rows?.length) {
      return;
    }
    groups.push({
      id: project.projectId,
      label: project.displayName || project.projectId,
      rows: rows.map(archivedChatRowFromThread),
    });
  };
  for (const project of projects.filter((item) => item.state === "active")) {
    pushProjectGroup(project);
  }
  for (const project of projects.filter((item) => item.state !== "active")) {
    pushProjectGroup(project);
  }
  const conversationRows = buckets.get(CONVERSATION_GROUP_ID);
  if (conversationRows?.length) {
    groups.push({
      id: CONVERSATION_GROUP_ID,
      label: "对话",
      rows: conversationRows.map(archivedChatRowFromThread),
    });
  }
  const unknownProjectIds = [...buckets.keys()]
    .filter((key) => key !== CONVERSATION_GROUP_ID && !projectById.has(key))
    .sort();
  for (const projectId of unknownProjectIds) {
    groups.push({
      id: projectId,
      label: unknownProjectGroupLabel(projectId),
      rows: buckets.get(projectId)!.map(archivedChatRowFromThread),
    });
  }
  return groups;
}

/** 本地受控搜索（Unicode 大小写不敏感子串）与项目筛选；只改可见集合。 */
export function filterArchivedChatGroups(
  groups: AwakenArchivedChatGroup[],
  query: string,
  projectFilter: string,
): AwakenArchivedChatGroup[] {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  return groups
    .filter((group) => projectFilter === ALL_PROJECTS_FILTER || group.id === projectFilter)
    .map((group) =>
      normalizedQuery
        ? {
            ...group,
            rows: group.rows.filter((row) =>
              row.title.toLocaleLowerCase().includes(normalizedQuery),
            ),
          }
        : group,
    )
    .filter((group) => group.rows.length > 0);
}

export function ArchivedChatsSettingsPanel({
  runtimeClient,
  onThreadsChanged,
}: ArchivedChatsSettingsPanelProps) {
  const [threads, setThreads] = useState<RuntimeThread[] | null>(null);
  const [projects, setProjects] = useState<RuntimeProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [projectFilter, setProjectFilter] = useState(ALL_PROJECTS_FILTER);
  const [busyThreadIds, setBusyThreadIds] = useState<string[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<{ threadId: string; label: string } | null>(
    null,
  );
  const [deleteSubmitting, setDeleteSubmitting] = useState(false);
  // 切换设置页/卸载后到达的旧请求结果直接丢弃，不覆盖新页面状态。
  const loadSeqRef = useRef(0);

  const load = useCallback(async () => {
    const seq = ++loadSeqRef.current;
    setLoading(true);
    setLoadError(null);
    try {
      const [archivedThreads, allProjects] = await Promise.all([
        runtimeClient.listThreads("archived"),
        runtimeClient.listProjects(true),
      ]);
      if (seq !== loadSeqRef.current) {
        return;
      }
      setThreads(archivedThreads);
      setProjects(allProjects);
    } catch (error) {
      if (seq !== loadSeqRef.current) {
        return;
      }
      setLoadError(error instanceof Error ? error.message : "已归档聊天载入失败");
    } finally {
      if (seq === loadSeqRef.current) {
        setLoading(false);
      }
    }
  }, [runtimeClient]);

  useEffect(() => {
    void load();
    return () => {
      loadSeqRef.current += 1;
    };
  }, [load]);

  const allGroups = useMemo(
    () => groupArchivedThreadsByProject(threads ?? [], projects),
    [threads, projects],
  );
  const visibleGroups = useMemo(
    () => filterArchivedChatGroups(allGroups, query, projectFilter),
    [allGroups, query, projectFilter],
  );
  const projectFilterOptions = useMemo<AwakenSelectOption[]>(
    () => [
      { value: ALL_PROJECTS_FILTER, label: "所有项目" },
      ...allGroups.map((group) => ({ value: group.id, label: group.label })),
    ],
    [allGroups],
  );
  const filtersActive = query.trim() !== "" || projectFilter !== ALL_PROJECTS_FILTER;

  function setThreadBusy(threadId: string, busy: boolean): void {
    setBusyThreadIds((current) =>
      busy
        ? current.includes(threadId)
          ? current
          : [...current, threadId]
        : current.filter((id) => id !== threadId),
    );
  }

  async function handleUnarchive(threadId: string): Promise<void> {
    if (busyThreadIds.includes(threadId)) {
      return;
    }
    setThreadBusy(threadId, true);
    setNotice(null);
    try {
      const result = await runtimeClient.setThreadArchived(threadId, false);
      if (!result.ok) {
        setNotice(result.message);
        return;
      }
      const thread = threads?.find((item) => item.threadId === threadId);
      const project = thread?.projectId
        ? projects.find((item) => item.projectId === thread.projectId)
        : undefined;
      setThreads((current) => current?.filter((item) => item.threadId !== threadId) ?? current);
      onThreadsChanged();
      setNotice(
        project && project.state !== "active"
          ? "已取消归档；重新添加项目后可在侧栏查看"
          : "已取消归档",
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "取消归档失败，请稍后重试");
    } finally {
      setThreadBusy(threadId, false);
    }
  }

  async function handleConfirmDelete(): Promise<void> {
    const pending = pendingDelete;
    if (!pending || deleteSubmitting) {
      return;
    }
    setDeleteSubmitting(true);
    setNotice(null);
    try {
      const result = await runtimeClient.deleteThread(pending.threadId);
      if (result.ok) {
        setPendingDelete(null);
        setNotice("已删除会话");
        await load();
        onThreadsChanged();
        return;
      }
      if (result.code === "THREAD_NOT_FOUND") {
        setPendingDelete(null);
        await load();
        setNotice("该记录已不存在，列表已刷新。");
        return;
      }
      setNotice(result.message);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除会话失败，请稍后重试");
    } finally {
      setDeleteSubmitting(false);
    }
  }

  return (
    <>
      <AwakenArchivedChatsSettings
        groups={visibleGroups}
        query={query}
        onQueryChange={setQuery}
        projectFilterOptions={projectFilterOptions}
        projectFilterValue={projectFilter}
        onProjectFilterChange={setProjectFilter}
        filtersActive={filtersActive}
        busyThreadIds={busyThreadIds}
        loading={loading}
        errorMessage={loadError}
        onUnarchive={(threadId) => {
          void handleUnarchive(threadId);
        }}
        onDelete={(threadId) => {
          const row = allGroups
            .flatMap((group) => group.rows)
            .find((item) => item.threadId === threadId);
          setPendingDelete({ threadId, label: row?.title ?? "" });
          setNotice(null);
        }}
        onRetry={() => {
          void load();
        }}
        onClearFilters={() => {
          setQuery("");
          setProjectFilter(ALL_PROJECTS_FILTER);
        }}
      />
      {notice ? (
        <p className="aw-archived-chats__status-text" role="status">
          {notice}
        </p>
      ) : null}
      <Dialog
        open={Boolean(pendingDelete)}
        title={pendingDelete?.label ? `删除会话“${pendingDelete.label}”？` : "删除会话？"}
        onOpenChange={(open) => {
          if (!open && !deleteSubmitting) {
            setPendingDelete(null);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={deleteSubmitting}
              onClick={() => setPendingDelete(null)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={deleteSubmitting}
              onClick={() => {
                void handleConfirmDelete();
              }}
            >
              删除会话
            </Button>
          </>
        }
      >
        <p className="project-delete-dialog-copy">
          将删除这条对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。
        </p>
      </Dialog>
    </>
  );
}
