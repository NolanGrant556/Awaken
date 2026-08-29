import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import {
  AwakenPluginInstallDialog,
  type AwakenPluginInstallMode,
  type AwakenPluginValidationSummary,
} from "./AwakenPluginInstallDialog";

const demoValidation: AwakenPluginValidationSummary = {
  valid: true,
  name: "acme.tools",
  version: "1.2.0",
  digest: "sha256-v2:9320952a413947fc9afbd85f09dd98c3f6172ace924cb25dcd1c44ac85d5921f",
  skillCount: 2,
  serverCount: 1,
  diagnostics: [
    {
      code: "plugin_skill_shadowed",
      message: "legacy-helper is shadowed inside the plugin package",
      severity: "warning",
    },
  ],
};

export function AwakenPluginInstallDialogDemo() {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<AwakenPluginInstallMode>("local");
  const [localPath, setLocalPath] = useState("");
  const [url, setUrl] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [validation, setValidation] = useState<AwakenPluginValidationSummary | null>(null);
  return (
    <section className="ads-component-demo" aria-label="Plugin 安装 AwakenPluginInstallDialog 组件演示">
      <header className="ads-component-demo-header">
        <h2>Plugin 安装 <code>AwakenPluginInstallDialog</code></h2>
      </header>
      <AwakenButton onClick={() => setOpen(true)}>打开安装</AwakenButton>
      <AwakenPluginInstallDialog
        open={open}
        mode={mode}
        localPath={localPath}
        url={url}
        confirm={confirm}
        validation={validation}
        onOpenChange={setOpen}
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
        onValidate={() => setValidation(demoValidation)}
        onSubmit={() => setOpen(false)}
      />
    </section>
  );
}
