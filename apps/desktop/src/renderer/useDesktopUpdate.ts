import { useEffect, useMemo, useState } from "react";
import type {
  DesktopUpdateErrorCode,
  DesktopUpdateSnapshot,
} from "../shared/desktop-update";
import {
  buildSidebarUpdateAction,
  desktopUpdateInstallBlockedMessage,
} from "./desktopUpdatePresentation";
import type { RuntimeClient } from "./runtime/client";

export function useDesktopUpdate(runtimeClient: RuntimeClient) {
  // Desktop 更新：先取当前快照再订阅变更；Main 是唯一 authority，
  // 推送先到时忽略迟到的 getState 返回，不写入 localStorage。
  const [desktopUpdateSnapshot, setDesktopUpdateSnapshot] =
    useState<DesktopUpdateSnapshot | null>(null);

  const [updateRestartDialogOpen, setUpdateRestartDialogOpen] = useState(false);

  const [updateInstallBlockedCode, setUpdateInstallBlockedCode] = useState<
    DesktopUpdateErrorCode | undefined
  >(undefined);

  const [updateInstallStarting, setUpdateInstallStarting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let pushReceived = false;
    const unsubscribe = runtimeClient.onDesktopUpdateStateChanged(
      (snapshot) => {
        pushReceived = true;
        if (!cancelled) {
          setDesktopUpdateSnapshot(snapshot);
        }
      },
    );
    runtimeClient
      .getDesktopUpdateState()
      .then((snapshot) => {
        if (!cancelled && !pushReceived) {
          setDesktopUpdateSnapshot(snapshot);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [runtimeClient]);

  const sidebarUpdateAction = useMemo(
    () =>
      buildSidebarUpdateAction(desktopUpdateSnapshot, {
        onDownload: () => {
          void runtimeClient
            .downloadDesktopUpdate()
            .then(setDesktopUpdateSnapshot)
            .catch(() => undefined);
        },
        onRestartRequested: () => {
          setUpdateInstallBlockedCode(undefined);
          setUpdateRestartDialogOpen(true);
        },
        onRetry: (retryFrom) => {
          const request =
            retryFrom === "download"
              ? runtimeClient.downloadDesktopUpdate()
              : runtimeClient.checkDesktopUpdate();
          void request.then(setDesktopUpdateSnapshot).catch(() => undefined);
        },
      }),
    [desktopUpdateSnapshot, runtimeClient],
  );

  const updateInstallBlockedNotice = desktopUpdateInstallBlockedMessage(
    updateInstallBlockedCode,
  );

  const updateInstallRetryBlocked =
    updateInstallBlockedCode === "active_runs" ||
    updateInstallBlockedCode === "shutdown_failed";

  const handleConfirmUpdateRestart = async () => {
    setUpdateInstallStarting(true);
    try {
      const snapshot = await runtimeClient.restartAndInstallDesktopUpdate();
      setDesktopUpdateSnapshot(snapshot);
      if (snapshot.phase === "downloaded") {
        // active_runs / shutdown_failed：留在确认框说明原因，不强制退出、不丢弃任务。
        setUpdateInstallBlockedCode(snapshot.errorCode);
        setUpdateInstallStarting(false);
      }
      // phase === "installing" 表示清理完成、安装器已接管，App 即将退出。
    } catch {
      setUpdateInstallBlockedCode("shutdown_failed");
      setUpdateInstallStarting(false);
    }
  };
  return {
    desktopUpdateSnapshot,
    updateRestartDialogOpen,
    setUpdateRestartDialogOpen,
    setUpdateInstallBlockedCode,
    updateInstallStarting,
    sidebarUpdateAction,
    updateInstallBlockedNotice,
    updateInstallRetryBlocked,
    handleConfirmUpdateRestart,
  };
}
