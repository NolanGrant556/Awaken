import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenPluginDetailDialog } from "./AwakenPluginDetailDialog";

export function AwakenPluginDetailDialogDemo() {
  const [open, setOpen] = useState(false);
  return (
    <section className="ads-component-demo" aria-label="Plugin 详情 AwakenPluginDetailDialog 组件演示">
      <header className="ads-component-demo-header">
        <h2>Plugin 详情 <code>AwakenPluginDetailDialog</code></h2>
      </header>
      <AwakenButton onClick={() => setOpen(true)}>打开详情</AwakenButton>
      <AwakenPluginDetailDialog
        open={open}
        name="acme.tools"
        version="1.2.0"
        description="示例插件"
        sourceLabel="remote_git"
        digest="sha256:demo"
        skills={[{ id: "plugin__acme-tools__lint", name: "lint", description: "Lint files", previewable: true }]}
        servers={[{ shortName: "validator", identity: "plugin__acme.tools__validator", state: "connected", enabled: true }]}
        onOpenChange={setOpen}
      />
    </section>
  );
}
