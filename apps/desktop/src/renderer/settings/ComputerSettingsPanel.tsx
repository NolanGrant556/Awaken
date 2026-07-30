import { useCallback, useEffect, useRef, useState } from "react";
import { AwakenComputerSettings } from "@awaken/design-system";
import type { AwakenComputerSettingsValue } from "@awaken/design-system";
import type { ComputerHostStatus, ComputerSettingsDocument } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

function toFormValue(document: ComputerSettingsDocument): AwakenComputerSettingsValue {
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
  const [document, setDocument] = useState<ComputerSettingsDocument | null>(null);
  const [draft, setDraft] = useState<AwakenComputerSettingsValue | null>(null);
  const [loading, setLoading] = useState(false);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsBusy, setSettingsBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const saveTimerRef = useRef<number | null>(null);
  const setupInFlightRef = useRef(false);
  const autoSetupAttemptedRef = useRef(false);

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
    try {
      const [nextStatus, nextDocument] = await Promise.all([
        runtimeClient.getComputerHostStatus(),
        runtimeClient.getComputerSettings(),
      ]);
      setStatus(nextStatus);
      setDocument(nextDocument);
      setDraft(toFormValue(nextDocument));
    } catch (error: unknown) {
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
      try {
        await persistSettings(nextDraft);
      } catch (error: unknown) {
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

  const setupEnvironment = useCallback(
    async (enabledDraft: AwakenComputerSettingsValue) => {
      if (setupInFlightRef.current) return;
      setupInFlightRef.current = true;
      setSettingsBusy(true);
      setMessage(null);

      try {
        // 1. Persist the enabled flag first so Core side is aligned.
        setMessage("正在启用计算机操作…");
        await persistSettings(enabledDraft);

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

        // 3. Refresh status so we know whether permissions are still missing.
        setMessage("正在检查权限状态…");
        const nextStatus = await runtimeClient.getComputerHostStatus();
        setStatus(nextStatus);

        // 4. Request screen recording permission if not granted.
        if (!nextStatus.tcc_screen_recording) {
          setMessage("正在请求屏幕录制权限…");
          await runtimeClient.requestComputerPermission("screen_recording");
        }

        // 5. Request accessibility permission if not granted (opens System Settings).
        if (!nextStatus.tcc_accessibility) {
          setMessage("正在打开辅助功能权限设置…");
          await runtimeClient.requestComputerPermission("accessibility");
        }

        // 6. Final status refresh.
        const finalStatus = await runtimeClient.getComputerHostStatus();
        setStatus(finalStatus);

        if (
          finalStatus.available &&
          finalStatus.tcc_screen_recording &&
          finalStatus.tcc_accessibility
        ) {
          setMessage(null);
        } else if (!finalStatus.tcc_screen_recording || !finalStatus.tcc_accessibility) {
          setMessage(
            "运行时准备完成，但系统权限尚未授予。请在系统设置中授权后点击刷新。",
          );
        } else {
          setMessage(finalStatus.message ?? "CUA Host 不可用。");
        }
      } catch (error: unknown) {
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
    [runtimeClient, persistSettings],
  );

  // Auto-setup when the setting is already enabled but the runtime is missing.
  useEffect(() => {
    if (
      document &&
      status &&
      document.value.enabled &&
      !status.available &&
      !autoSetupAttemptedRef.current &&
      !setupInFlightRef.current &&
      !settingsLoading
    ) {
      autoSetupAttemptedRef.current = true;
      void setupEnvironment(toFormValue(document));
    }
  }, [document, status, settingsLoading, setupEnvironment]);

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
      onRefresh={loadAll}
      onSettingsChange={handleSettingsChange}
      onSetupRuntime={() => {
        if (draft) {
          void setupEnvironment(draft);
        }
      }}
    />
  );
}
