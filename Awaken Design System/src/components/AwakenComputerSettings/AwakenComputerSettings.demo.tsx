import { useState } from "react";
import {
  AwakenComputerSettings,
  type AwakenComputerHostStatus,
  type AwakenComputerSettingsValue,
} from "./AwakenComputerSettings";

export function AwakenComputerSettingsDemo() {
  const [status] = useState<AwakenComputerHostStatus>({
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
  });

  const [settings, setSettings] = useState<AwakenComputerSettingsValue>({
    enabled: true,
    allow_background_window_actions: false,
  });

  return (
    <AwakenComputerSettings
      status={status}
      settings={settings}
      onRefresh={() => {
        // Demo only: real refresh would call the Core status API.
      }}
      onSettingsChange={setSettings}
      onSetupRuntime={() => {
        // Demo only: real setup would call the Desktop main process API.
      }}
    />
  );
}
