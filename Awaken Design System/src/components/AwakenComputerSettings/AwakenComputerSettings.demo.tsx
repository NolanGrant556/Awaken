import { useState } from "react";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
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
    <section
      className="ads-component-demo"
      aria-label="计算机操作设置 AwakenComputerSettings 组件演示"
    >
      <header className="ads-component-demo-header">
        <h2>
          计算机操作设置 <code>AwakenComputerSettings</code>
        </h2>
        <p>展示运行时、系统权限与固定产品策略状态。</p>
      </header>
      <AwakenSettingsSectionPreview
        activeSectionId="computer"
        sectionTitle="计算机操作"
      >
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
      </AwakenSettingsSectionPreview>
    </section>
  );
}
