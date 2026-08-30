import { useState } from "react";
import { AwakenPluginCard, type AwakenPluginCardItem } from "./AwakenPluginCard";
import { AwakenPluginDetailDialog } from "../AwakenPluginDetailDialog";
import {
  AwakenPluginInstallDialog,
  type AwakenPluginValidationSummary,
} from "../AwakenPluginInstallDialog";

const initial: AwakenPluginCardItem[] = [
  { name: "acme.tools", description: "Tools", dialect: "codex", enabled: true },
  { name: "lint-kit", description: "诊断示例", dialect: "cursor", enabled: false, diagnostic: "mcp.json invalid" },
];

const installValidation: AwakenPluginValidationSummary = {
  valid: true,
  name: "acme.tools",
  version: "1.2.0",
  digest: "sha256-v2:9320952a413947fc9afbd85f09dd98c3f6172ace924cb25dcd1c44ac85d5921f",
  diagnostics: [],
};

export function AwakenPluginCardDemo() {
  const [items, setItems] = useState(initial);
  const [detailOpen, setDetailOpen] = useState(false);
  const [installOpen, setInstallOpen] = useState(false);
  const [localPath, setLocalPath] = useState("");
  const [validation, setValidation] = useState<AwakenPluginValidationSummary | null>(null);
  function selectDemoZip(path: string): void {
    setLocalPath(path);
    setValidation(installValidation);
  }
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
        localPath={localPath}
        validation={validation}
        onOpenChange={setInstallOpen}
        onPickLocal={() => selectDemoZip("/tmp/acme.tools.zip")}
        onDropFile={(file) => selectDemoZip(file.name)}
        onSubmit={() => setInstallOpen(false)}
      />
      <AwakenPluginDetailDialog
        open={detailOpen}
        name="acme.tools"
        version="1.2.0"
        description="示例插件"
        sourceLabel="local_path · acme-tools.zip"
        digest="sha256:demo"
        skills={[{ id: "plugin__acme-tools__lint", name: "lint", previewable: true }]}
        servers={[{ shortName: "validator", identity: "plugin__acme.tools__validator", state: "connected", enabled: true }]}
        onOpenChange={setDetailOpen}
        onPreviewSkill={() => undefined}
        onServerEnabledChange={() => undefined}
        onUninstall={() => setDetailOpen(false)}
      />
    </section>
  );
}
