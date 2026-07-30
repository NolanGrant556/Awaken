import type { AwakenComputerSettingsProps } from "./AwakenComputerSettings";

const BASE_STATUS = {
  available: true,
  healthy: true,
  version: "0.12.5",
  protocol_version: 1,
  os: "darwin",
  tcc_screen_recording: true,
  tcc_accessibility: true,
  node_path: "/Applications/Awaken.app/Contents/Resources/bin/darwin-arm64/node/bin/node",
  host_script_path: "/Applications/Awaken.app/Contents/Resources/cua-driver/host.mjs",
  message: "healthy",
} satisfies AwakenComputerSettingsProps["status"];

const BASE_SETTINGS = {
  enabled: true,
  allow_background_window_actions: false,
} satisfies AwakenComputerSettingsProps["settings"];

export const AwakenComputerSettingsStates = [
  {
    id: "ready",
    label: "全部就绪",
    props: {
      status: BASE_STATUS,
      settings: BASE_SETTINGS,
      loading: false,
      settingsLoading: false,
      settingsBusy: false,
      settingsError: null,
      onRefresh: () => undefined,
      onSettingsChange: () => undefined,
      onSetupRuntime: () => undefined,
    } satisfies AwakenComputerSettingsProps,
  },
  {
    id: "missing_permissions",
    label: "缺少权限",
    props: {
      status: {
        ...BASE_STATUS,
        tcc_screen_recording: false,
        tcc_accessibility: false,
      },
      settings: BASE_SETTINGS,
      loading: false,
      settingsLoading: false,
      settingsBusy: false,
      settingsError: null,
      onRefresh: () => undefined,
      onSettingsChange: () => undefined,
      onSetupRuntime: () => undefined,
    } satisfies AwakenComputerSettingsProps,
  },
  {
    id: "unavailable",
    label: "Host 不可用",
    props: {
      status: {
        available: false,
        healthy: false,
        tcc_screen_recording: false,
        tcc_accessibility: false,
        message: "cua host script not found; set AWAKEN_CUA_HOST_SCRIPT or install the host sidecar",
      },
      settings: BASE_SETTINGS,
      loading: false,
      settingsLoading: false,
      settingsBusy: false,
      settingsError: null,
      onRefresh: () => undefined,
      onSettingsChange: () => undefined,
      onSetupRuntime: () => undefined,
    } satisfies AwakenComputerSettingsProps,
  },
  {
    id: "loading",
    label: "检测中",
    props: {
      status: null,
      settings: null,
      loading: true,
      settingsLoading: true,
      settingsBusy: false,
      settingsError: null,
      onRefresh: () => undefined,
      onSettingsChange: () => undefined,
      onSetupRuntime: () => undefined,
    } satisfies AwakenComputerSettingsProps,
  },
] satisfies Array<{ id: string; label: string; props: AwakenComputerSettingsProps }>;
