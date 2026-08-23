import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenPluginInstallDialog, type AwakenPluginInstallMode } from "./AwakenPluginInstallDialog";

export function AwakenPluginInstallDialogDemo() {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<AwakenPluginInstallMode>("local");
  const [localPath, setLocalPath] = useState("");
  const [url, setUrl] = useState("");
  const [confirm, setConfirm] = useState(false);
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
        onOpenChange={setOpen}
        onModeChange={setMode}
        onLocalPathChange={setLocalPath}
        onUrlChange={setUrl}
        onConfirmChange={setConfirm}
        onSubmit={() => setOpen(false)}
      />
    </section>
  );
}
