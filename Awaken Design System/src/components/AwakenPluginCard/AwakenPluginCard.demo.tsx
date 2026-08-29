import { useState } from "react";
import { AwakenPluginCard, type AwakenPluginCardItem } from "./AwakenPluginCard";
import { AwakenPluginDetailDialog } from "../AwakenPluginDetailDialog";
import {
  AwakenPluginInstallDialog,
  type AwakenPluginInstallMode,
  type AwakenPluginValidationSummary,
} from "../AwakenPluginInstallDialog";

const initial: AwakenPluginCardItem[] = [
  { name: "acme.tools", version: "1.2.0", description: "Tools", skillCount: 1, serverCount: 2, enabled: true },
  { name: "lint-kit", version: "0.9.0", description: "诊断示例", skillCount: 0, serverCount: 1, enabled: false, diagnostic: "mcp.json invalid" },
];

const installValidation: AwakenPluginValidationSummary = {
  valid: true,
  name: "acme.tools",
  version: "1.2.0",
  digest: "sha256-v2:9320952a413947fc9afbd85f09dd98c3f6172ace924cb25dcd1c44ac85d5921f",
  skillCount: 1,
  serverCount: 2,
  diagnostics: [],
};

export function AwakenPluginCardDemo() {
  const [items, setItems] = useState(initial);
  const [detailOpen, setDetailOpen] = useState(false);
  const [installOpen, setInstallOpen] = useState(false);
  const [mode, setMode] = useState<AwakenPluginInstallMode>("local");
  const [localPath, setLocalPath] = useState("");
  const [url, setUrl] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [validation, setValidation] = useState<AwakenPluginValidationSummary | null>(null);
  return (
    <section className="ads-component-demo" aria-label="Plugin 组件族演示">
      <header className="ads-component-demo-header">
        <h2>Plugin 卡片 <code>AwakenPluginCard</code></h2>
      </header>
      <div className="aw-plugin-card-demo__stack">
        {items.map((plugin) => (
          <AwakenPluginCard
            key={plugin.name}
            plugin={plugin}
            onOpen={() => setDetailOpen(true)}
            onEnabledChange={(enabled) => {
              setItems((current) => current.map((item) => item.name === plugin.name ? { ...item, enabled } : item));
            }}
          />
        ))}
      </div>
      <button type="button" onClick={() => setInstallOpen(true)}>
        打开安装弹窗
      </button>
      <AwakenPluginInstallDialog
        open={installOpen}
        mode={mode}
        localPath={localPath}
        url={url}
        confirm={confirm}
        validation={validation}
        onOpenChange={setInstallOpen}
        onModeChange={(value) => {
          setMode(value);
          setValidation(null);
        }}
        onLocalPathChange={(value) => {
          setLocalPath(value);
          setValidation(null);
        }}
        onUrlChange={(value) => {
          setUrl(value);
          setValidation(null);
        }}
        onConfirmChange={setConfirm}
        onValidate={() => setValidation(installValidation)}
        onSubmit={() => setInstallOpen(false)}
      />
      <AwakenPluginDetailDialog
        open={detailOpen}
        name="acme.tools"
        version="1.2.0"
        description="示例插件"
        sourceLabel="remote_git https://example.test/acme-tools.git"
        digest="sha256:demo"
        skills={[{ id: "plugin__acme-tools__lint", name: "lint", previewable: true }]}
        servers={[{ shortName: "validator", identity: "plugin__acme.tools__validator", state: "connected", enabled: true }]}
        onOpenChange={setDetailOpen}
        onPreviewSkill={() => undefined}
        onServerEnabledChange={() => undefined}
        onUpgrade={() => undefined}
        onUninstall={() => setDetailOpen(false)}
      />
    </section>
  );
}
