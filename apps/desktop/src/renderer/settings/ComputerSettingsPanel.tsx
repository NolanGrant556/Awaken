import { useCallback, useEffect, useRef, useState } from "react";
import { AwakenComputerSettings } from "@awaken/design-system";
import type { AwakenComputerSettingsValue } from "@awaken/design-system";
import type {
  ComputerHostStatus,
  ComputerPermission,
  ComputerSettingsDocument,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

function firstMissingPermission(
  status: ComputerHostStatus,
): ComputerPermission | null {
  if (!status.tcc_screen_recording) return "screen_recording";
  if (!status.tcc_accessibility) return "accessibility";
  return null;
}

function permissionLabel(permission: ComputerPermission): string {
  return permission === "screen_recording" ? "屏幕录制" : "辅助功能";
}

function toFormValue(
  document: ComputerSettingsDocument,
): AwakenComputerSettingsValue {
  const value = document.value;
  return {
    enabled: value.enabled,
    allow_background_window_actions: value.allow_background_window_actions,
  };
}

function toSaveRequest(
  document: ComputerSettingsDocument,
  draft: AwakenComputerSettingsValue,
) {
  return {
    expected_revision: document.revision,
    enabled: draft.enabled,
    allow_background_window_actions: draft.allow_background_window_actions,
  };
}

export function ComputerSettingsPanel({
  runtimeClient,
}: {
  runtimeClient: RuntimeClient;
}) {
  const [status, setStatus] = useState<ComputerHostStatus | null>(null);
  const [document, setDocument] = useState<ComputerSettingsDocument | null>(
    null,
  );
  const [draft, setDraft] = useState<AwakenComputerSettingsValue | null>(null);
  const [loading, setLoading] = useState(false);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsBusy, setSettingsBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [messageTone, setMessageTone] = useState<"info" | "error">("info");

  const saveTimerRef = useRef<number | null>(null);
  const setupInFlightRef = useRef(false);
  const lastOpenedPermissionRef = useRef<ComputerPermission | null>(null);

  useEffect(() => {
    return () => {
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current);
      }
    };
  }, []);

  const loadAll = useCallback(async () => {
    setLoading(true);
    setSettingsLoading(true);
    setMessage(null);
    setMessageTone("info");
    try {
      const [nextStatus, nextDocument] = await Promise.all([
        runtimeClient.getComputerHostStatus(),
        runtimeClient.getComputerSettings(),
      ]);
      setStatus(nextStatus);
      setDocument(nextDocument);
      setDraft(toFormValue(nextDocument));
    } catch (error: unknown) {
      setMessageTone("error");
      setMessage(
        error instanceof Error
          ? error.message
          : "无法读取计算机操作配置，请检查 Core 是否已启动。",
      );
    } finally {
      setLoading(false);
      setSettingsLoading(false);
    }
  }, [runtimeClient]);

  useEffect(() => {
    let live = true;
    void loadAll().then(() => {
      if (!live) return;
    });
    return () => {
      live = false;
    };
  }, [loadAll]);

  // Core save logic without any UI busy state; callers manage their own.
  const persistSettings = useCallback(
    async (nextDraft: AwakenComputerSettingsValue) => {
      const currentDocument = document;
      if (!currentDocument) {
        throw new Error("设置尚未加载");
      }
      const request = toSaveRequest(currentDocument, nextDraft);
      try {
        const saved = await runtimeClient.saveComputerSettings(request);
        setDocument(saved);
        setDraft(toFormValue(saved));
        return saved;
      } catch (error: unknown) {
        if (
          error instanceof Error &&
          /revision|conflict|版本/i.test(error.message)
        ) {
          const refreshed = await runtimeClient.getComputerSettings();
          setDocument(refreshed);
          setDraft(toFormValue(refreshed));
          throw new Error("设置已被其他客户端修改，已刷新，请重新操作。");
        }
        throw error;
      }
    },
    [document, runtimeClient],
  );

  const saveSettings = useCallback(
    async (nextDraft: AwakenComputerSettingsValue) => {
      setSettingsBusy(true);
      setMessage(null);
      setMessageTone("info");
      try {
        await persistSettings(nextDraft);
      } catch (error: unknown) {
        setMessageTone("error");
        setMessage(
          error instanceof Error
            ? error.message
            : "保存设置失败，请检查 Core 是否已启动。",
        );
      } finally {
        setSettingsBusy(false);
      }
    },
    [persistSettings],
  );

  const requestPermissionsAndOpenNext = useCallback(
    async (preferredPermission?: ComputerPermission) => {
      setMessageTone("info");
      setMessage("正在请求屏幕录制与辅助功能权限…");
      const requestedStatus = await runtimeClient.requestComputerPermissions();
      setStatus(requestedStatus);

      const preferredStillMissing =
        preferredPermission === "screen_recording"
          ? !requestedStatus.tcc_screen_recording
          : preferredPermission === "accessibility"
            ? !requestedStatus.tcc_accessibility
            : false;
      const permission = preferredStillMissing
        ? preferredPermission!
        : firstMissingPermission(requestedStatus);
      if (!permission && requestedStatus.available && requestedStatus.healthy) {
        lastOpenedPermissionRef.current = null;
        setMessage(null);
        return;
      }
      if (!permission) {
        setMessageTone("error");
        setMessage(requestedStatus.message ?? "CUA Host 不可用。");
        return;
      }

      lastOpenedPermissionRef.current = permission;
      setMessage(
        `请在 macOS 系统设置中为 Computer Host 开启${permissionLabel(permission)}权限。`,
      );
      await runtimeClient.openComputerPermissionSettings(permission);
    },
    [runtimeClient],
  );

  const setupEnvironment = useCallback(
    async (
      enabledDraft: AwakenComputerSettingsValue,
      options: { persist?: boolean } = {},
    ) => {
      if (setupInFlightRef.current) return;
      setupInFlightRef.current = true;
      setSettingsBusy(true);
      setMessage(null);
      setMessageTone("info");

      try {
        // 1. Persist a newly enabled flag first so Core side is aligned. An
        // already-enabled setup recovery does not create a redundant revision.
        if (options.persist !== false) {
          setMessage("正在启用计算机操作…");
          await persistSettings(enabledDraft);
        }

        // 2. Prepare Node runtime and CUA Host if missing.
        setMessage("正在检查 CUA 运行时…");
        const setup = await runtimeClient.prepareComputerRuntime();
        if (!setup.node_ready || !setup.cua_host_ready) {
          setMessage(
            setup.message ??
              "CUA 运行时准备未完成，请检查网络或手动运行 node scripts/ensure-desktop-runtime.mjs。",
          );
          return;
        }

        // 3. Refresh status so we know whether Host-owned permissions are
        // still missing.
        setMessage("正在检查权限状态…");
        const nextStatus = await runtimeClient.getComputerHostStatus();
        setStatus(nextStatus);

        // 4. Ask the real CUA Host for both TCC permissions. If macOS still
        // requires a manual toggle, open the first missing System Settings pane.
        if (firstMissingPermission(nextStatus)) {
          await requestPermissionsAndOpenNext();
          return;
        }

        // 5. Final status refresh.
        const finalStatus = await runtimeClient.getComputerHostStatus();
        setStatus(finalStatus);

        if (
          finalStatus.available &&
          finalStatus.tcc_screen_recording &&
          finalStatus.tcc_accessibility
        ) {
          setMessage(null);
        } else if (
          !finalStatus.tcc_screen_recording ||
          !finalStatus.tcc_accessibility
        ) {
          setMessage(
            "运行时准备完成，但系统权限尚未授予。请在系统设置中授权后点击刷新。",
          );
        } else {
          setMessage(finalStatus.message ?? "CUA Host 不可用。");
        }
      } catch (error: unknown) {
        setMessageTone("error");
        setMessage(
          error instanceof Error
            ? error.message
            : "自动配置失败，请检查 Core 与 Desktop 是否正常。",
        );
      } finally {
        setSettingsBusy(false);
        setupInFlightRef.current = false;
      }
    },
    [runtimeClient, persistSettings, requestPermissionsAndOpenNext],
  );

  const continuePermissionSetup = useCallback(
    async (preferredPermission?: ComputerPermission) => {
      if (setupInFlightRef.current) return;
      setupInFlightRef.current = true;
      setSettingsBusy(true);
      try {
        await requestPermissionsAndOpenNext(preferredPermission);
      } catch (error: unknown) {
        setMessageTone("error");
        setMessage(
          error instanceof Error ? error.message : "权限请求失败，请稍后重试。",
        );
      } finally {
        setSettingsBusy(false);
        setupInFlightRef.current = false;
      }
    },
    [requestPermissionsAndOpenNext],
  );

  // Returning from System Settings refreshes Host-owned TCC state. When the
  // first permission was granted, continue to the second pane automatically;
  // returning without progress leaves the user in Awaken with a retry action.
  useEffect(() => {
    const handleFocus = () => {
      if (
        lastOpenedPermissionRef.current === null ||
        !document?.value.enabled ||
        setupInFlightRef.current ||
        settingsLoading
      ) {
        return;
      }
      void (async () => {
        setupInFlightRef.current = true;
        setSettingsBusy(true);
        try {
          setMessageTone("info");
          setMessage("正在重新检测系统权限…");
          const nextStatus = await runtimeClient.getComputerHostStatus();
          setStatus(nextStatus);
          const missing = firstMissingPermission(nextStatus);
          if (!missing && nextStatus.available && nextStatus.healthy) {
            lastOpenedPermissionRef.current = null;
            setMessage(null);
          } else if (missing && missing !== lastOpenedPermissionRef.current) {
            await requestPermissionsAndOpenNext(missing);
          } else if (missing) {
            setMessage(
              `${permissionLabel(missing)}权限仍未开启，可点击“去授权”继续。`,
            );
          } else {
            setMessageTone("error");
            setMessage(nextStatus.message ?? "CUA Host 不可用。");
          }
        } catch (error: unknown) {
          setMessageTone("error");
          setMessage(
            error instanceof Error ? error.message : "重新检测系统权限失败。",
          );
        } finally {
          setSettingsBusy(false);
          setupInFlightRef.current = false;
        }
      })();
    };
    window.addEventListener("focus", handleFocus);
    return () => window.removeEventListener("focus", handleFocus);
  }, [document, requestPermissionsAndOpenNext, runtimeClient, settingsLoading]);

  const handleSettingsChange = useCallback(
    (next: AwakenComputerSettingsValue) => {
      setDraft(next);
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current);
      }

      const wasEnabled = document?.value.enabled ?? false;
      if (next.enabled && !wasEnabled) {
        // User just turned the master switch on: run auto-setup immediately.
        void setupEnvironment(next);
      } else {
        // Other changes are saved with a small debounce.
        saveTimerRef.current = window.setTimeout(() => {
          saveTimerRef.current = null;
          void saveSettings(next);
        }, 300);
      }
    },
    [document, saveSettings, setupEnvironment],
  );

  return (
    <AwakenComputerSettings
      status={status}
      settings={draft}
      loading={loading}
      settingsLoading={settingsLoading}
      settingsBusy={settingsBusy}
      settingsError={message}
      settingsMessageTone={messageTone}
      onRefresh={loadAll}
      onSettingsChange={handleSettingsChange}
      onRequestPermissions={() => {
        void continuePermissionSetup();
      }}
      onOpenPermissionSettings={(permission) => {
        void continuePermissionSetup(permission);
      }}
      onSetupRuntime={() => {
        if (draft) {
          void setupEnvironment(draft, { persist: false });
        }
      }}
    />
  );
}
