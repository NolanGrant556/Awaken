import { type ProjectItem } from "@awaken/design-system";
import { useEffect, useRef, useState } from "react";
import { PendingThreadDelete } from "./projectRunScope";
import { projectIdForThread } from "./projectWorkspace";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction, RuntimeViewState } from "./runtime/reducer";
import { type ThreadItem } from "./runtime/reducer";
import {
  shouldShowSidebarThreadSpinner,
  sidebarThreadMenuItems,
} from "./sidebarThreadPresentation";
import { composerDraftOwnerKey } from "./useThreadComposerDraft";
import { useThreadDirectory } from "./useThreadDirectory";
type Options = {
  dispatch: (action: RuntimeAction) => void;
  runtimeClient: RuntimeClient;
  desktopReady: boolean;
  runtimeState: Pick<RuntimeViewState, "threads" | "activeThreadId">;
  directory: ReturnType<typeof useThreadDirectory>;
  effectiveDefaultWorkspacePath?: string;
  setConversationSectionExpanded: (expanded: boolean) => void;
  clearThreadComposerDraft: (ownerKey?: string) => void;
  setNotice: (message: string | null) => void;
  resetActiveThreadContext: () => void;
  onActiveThreadArchived: () => void;
  threadsForProject: (projectId: string) => ThreadItem[];
};
export function useThreadNavigation(options: Options) {
  const {
    dispatch,
    runtimeClient,
    desktopReady,
    runtimeState,
    effectiveDefaultWorkspacePath,
    setConversationSectionExpanded,
    clearThreadComposerDraft,
    setNotice,
    resetActiveThreadContext,
    onActiveThreadArchived,
    threadsForProject,
  } = options;
  const { refreshThreads, loadThreadsIntoState, stopThreadTitlePoll } =
    options.directory;

  const [renameDialogOpen, setRenameDialogOpen] = useState(false);

  const [renamingThreadId, setRenamingThreadId] = useState<string | null>(null);

  const [renameDraft, setRenameDraft] = useState("");

  const [renameSubmitting, setRenameSubmitting] = useState(false);

  const renameInputRef = useRef<HTMLInputElement>(null);

  const [conversationBatchMode, setConversationBatchMode] = useState(false);

  const [selectedConversationThreadIds, setSelectedConversationThreadIds] =
    useState<string[]>([]);

  const [pendingThreadDelete, setPendingThreadDelete] =
    useState<PendingThreadDelete | null>(null);

  const [threadDeleteSubmitting, setThreadDeleteSubmitting] = useState(false);

  useEffect(() => {
    if (!renameDialogOpen) {
      return;
    }
    const input = renameInputRef.current;
    if (!input) {
      return;
    }
    const timeoutId = setTimeout(() => {
      input.focus();
      input.select();
    }, 0);
    return () => clearTimeout(timeoutId);
  }, [renameDialogOpen]);

  function threadItems(threads: ThreadItem[]): ProjectItem[] {
    return threads.map((thread) => {
      return {
        id: `thread:${thread.threadId}`,
        kind: "thread",
        label: thread.label,
        isLoading: shouldShowSidebarThreadSpinner(thread),
        menuItems: sidebarThreadMenuItems(thread, {
          onRename: () => {
            void handleRenameThread(thread.threadId);
          },
          onArchive: () => {
            void handleArchiveThread(thread.threadId);
          },
          onDelete: () => {
            requestThreadDelete([thread.threadId], "single");
          },
        }),
      };
    });
  }

  function threadItemsForProject(projectId: string): ProjectItem[] {
    return threadItems(threadsForProject(projectId));
  }

  const conversationThreads = runtimeState.threads.filter(
    (thread) =>
      projectIdForThread(thread, effectiveDefaultWorkspacePath) === undefined,
  );

  const eligibleConversationThreadIds = conversationThreads
    .filter((thread) => !shouldShowSidebarThreadSpinner(thread))
    .map((thread) => thread.threadId);

  const eligibleConversationThreadIdsKey =
    eligibleConversationThreadIds.join("\u0000");

  const eligibleConversationThreadIdSet = new Set(
    eligibleConversationThreadIds,
  );

  useEffect(() => {
    if (!conversationBatchMode) {
      return;
    }
    setSelectedConversationThreadIds((current) => {
      const eligibleIds = new Set(eligibleConversationThreadIds);
      const next = current.filter((threadId) => eligibleIds.has(threadId));
      return next.length === current.length ? current : next;
    });
  }, [conversationBatchMode, eligibleConversationThreadIdsKey]);

  const selectedConversationThreadIdSet = new Set(
    selectedConversationThreadIds.filter((threadId) =>
      eligibleConversationThreadIdSet.has(threadId),
    ),
  );

  const selectedConversationCount = selectedConversationThreadIdSet.size;

  const allEligibleConversationsSelected =
    eligibleConversationThreadIds.length > 0 &&
    selectedConversationCount === eligibleConversationThreadIds.length;

  const conversationItems = threadItems(conversationThreads).map(
    (item, index) => {
      if (!conversationBatchMode) {
        return item;
      }
      const thread = conversationThreads[index]!;
      const selectionDisabled = shouldShowSidebarThreadSpinner(thread);
      return {
        ...item,
        selection: {
          checked: selectedConversationThreadIdSet.has(thread.threadId),
          disabled: selectionDisabled,
          disabledReason: selectionDisabled
            ? "正在运行，停止后可删除"
            : undefined,
          onChange: (checked: boolean) => {
            setSelectedConversationThreadIds((current) => {
              if (checked) {
                return current.includes(thread.threadId)
                  ? current
                  : [...current, thread.threadId];
              }
              return current.filter((threadId) => threadId !== thread.threadId);
            });
          },
        },
      };
    },
  );

  async function refreshAfterThreadMutation(
    removedThreadIds: string[] = [],
  ): Promise<void> {
    if (removedThreadIds.includes(runtimeState.activeThreadId ?? "")) {
      resetActiveThreadContext();
    }

    await loadThreadsIntoState();
  }

  async function handleRenameThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread) {
      setNotice("会话不存在");
      return;
    }

    setRenamingThreadId(threadId);
    setRenameDraft(thread.label);
    setRenameDialogOpen(true);
  }

  async function handleArchiveThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    setNotice(null);
    try {
      const result = await runtimeClient.setThreadArchived(threadId, true);
      if (!result.ok) {
        // 失败保留原行，不做无法核实的乐观删除。
        setNotice(result.message);
        return;
      }

      if (threadId === runtimeState.activeThreadId) {
        onActiveThreadArchived();
      }
      clearThreadComposerDraft(composerDraftOwnerKey(threadId));
      // active authority 列表不再包含该 thread；若它是当前 thread，
      // refreshAfterThreadMutation 会经 reducer 回到“新建任务”空态，项目选择保持不变。
      await refreshAfterThreadMutation([threadId]);
      setNotice("已归档会话");
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "归档会话失败，请稍后重试",
      );
    }
  }

  async function handleConfirmRename(): Promise<void> {
    const threadId = renamingThreadId;
    if (!threadId) {
      return;
    }

    const nextLabel = renameDraft.trim();
    if (!nextLabel) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread || nextLabel === thread.label) {
      setRenameDialogOpen(false);
      setRenamingThreadId(null);
      setRenameDraft("");
      return;
    }

    setRenameSubmitting(true);
    try {
      const updatedThread = await runtimeClient.renameThread(
        threadId,
        nextLabel,
      );
      dispatch({ type: "run/threadRenamed", thread: updatedThread });
      setNotice("已重命名会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "重命名会话失败");
    } finally {
      setRenameSubmitting(false);
      setRenameDialogOpen(false);
      setRenamingThreadId(null);
      setRenameDraft("");
    }
  }

  function handleCancelRename(): void {
    setRenameDialogOpen(false);
    setRenamingThreadId(null);
    setRenameDraft("");
  }

  function enterConversationBatchMode(): void {
    setConversationSectionExpanded(true);
    setSelectedConversationThreadIds([]);
    setConversationBatchMode(true);
    setNotice(null);
  }

  function exitConversationBatchMode(): void {
    setConversationBatchMode(false);
    setSelectedConversationThreadIds([]);
  }

  function requestThreadDelete(
    threadIds: string[],
    source: PendingThreadDelete["source"],
  ): void {
    if (!desktopReady) {
      return;
    }
    const existingThreadIds = new Set(
      runtimeState.threads.map((thread) => thread.threadId),
    );
    const requestedThreadIds = threadIds.filter((threadId) =>
      existingThreadIds.has(threadId),
    );
    if (requestedThreadIds.length === 0) {
      setNotice("没有可删除的会话");
      return;
    }
    setPendingThreadDelete({ threadIds: requestedThreadIds, source });
    setNotice(null);
  }

  async function handleConfirmThreadDelete(): Promise<void> {
    const pending = pendingThreadDelete;
    if (!pending || threadDeleteSubmitting || !desktopReady) {
      return;
    }

    setThreadDeleteSubmitting(true);
    try {
      const result = await runtimeClient.deleteThreads(pending.threadIds);
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      await refreshAfterThreadMutation(result.deletedThreadIds);
      const failedThreadIds = result.failures.map(
        (failure) => failure.threadId,
      );
      if (pending.source === "batch") {
        setSelectedConversationThreadIds(failedThreadIds);
        if (failedThreadIds.length === 0) {
          setConversationBatchMode(false);
        }
      }

      if (result.failures.length === 0) {
        setNotice(
          result.deletedThreadIds.length === 1
            ? "已删除会话"
            : `已删除 ${result.deletedThreadIds.length} 个对话`,
        );
      } else {
        const busyCount = result.failures.filter(
          (failure) => failure.code === "THREAD_BUSY",
        ).length;
        const failureSummary =
          busyCount === result.failures.length
            ? `${busyCount} 个会话正在运行，停止后可继续删除`
            : `${result.failures.length} 个会话未能删除，请稍后重试`;
        setNotice(
          result.deletedThreadIds.length > 0
            ? `已删除 ${result.deletedThreadIds.length} 个对话；${failureSummary}`
            : failureSummary,
        );
      }
      setPendingThreadDelete(null);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除会话失败");
      await refreshThreads();
    } finally {
      setThreadDeleteSubmitting(false);
    }
  }

  const pendingThreadDeleteThreads = pendingThreadDelete
    ? pendingThreadDelete.threadIds.flatMap((threadId) => {
        const thread = runtimeState.threads.find(
          (item) => item.threadId === threadId,
        );
        return thread ? [thread] : [];
      })
    : [];

  const pendingThreadDeleteCount = pendingThreadDeleteThreads.length;

  const threadDeleteDialogTitle =
    pendingThreadDelete?.source === "batch"
      ? `删除 ${pendingThreadDeleteCount} 个对话？`
      : pendingThreadDeleteThreads[0]
        ? `删除会话“${pendingThreadDeleteThreads[0].label}”？`
        : "删除会话？";

  const threadDeleteDialogCopy =
    pendingThreadDelete?.source === "batch"
      ? "将删除所选对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。"
      : "将删除这条对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。";
  return {
    renameDialogOpen,
    setRenameDialogOpen,
    renamingThreadId,
    setRenamingThreadId,
    renameDraft,
    setRenameDraft,
    renameSubmitting,
    setRenameSubmitting,
    renameInputRef,
    conversationBatchMode,
    setConversationBatchMode,
    selectedConversationThreadIds,
    setSelectedConversationThreadIds,
    pendingThreadDelete,
    setPendingThreadDelete,
    threadDeleteSubmitting,
    setThreadDeleteSubmitting,
    threadItems,
    threadItemsForProject,
    conversationThreads,
    eligibleConversationThreadIds,
    eligibleConversationThreadIdsKey,
    eligibleConversationThreadIdSet,
    selectedConversationThreadIdSet,
    selectedConversationCount,
    allEligibleConversationsSelected,
    conversationItems,
    refreshAfterThreadMutation,
    handleRenameThread,
    handleArchiveThread,
    handleConfirmRename,
    handleCancelRename,
    enterConversationBatchMode,
    exitConversationBatchMode,
    requestThreadDelete,
    handleConfirmThreadDelete,
    pendingThreadDeleteThreads,
    pendingThreadDeleteCount,
    threadDeleteDialogTitle,
    threadDeleteDialogCopy,
  };
}
