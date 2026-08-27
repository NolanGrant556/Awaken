import { useState } from "react";
import { AwakenPluginDetailDialog } from "../AwakenPluginDetailDialog";
import {
  AwakenPluginInstallDialog,
  type AwakenPluginInstallMode,
  type AwakenPluginValidationSummary,
} from "../AwakenPluginInstallDialog";
import { AwakenPlugins } from "./AwakenPlugins";
import { awakenPluginsDemoItems } from "./AwakenPlugins.states";

const installValidation: AwakenPluginValidationSummary = {
  valid: true,
  name: "acme.tools",
  version: "1.2.0",
  digest: "sha256-v2:9320952a413947fc9afbd85f09dd98c3f6172ace924cb25dcd1c44ac85d5921f",
  skillCount: 2,
  serverCount: 1,
  diagnostics: [],
};

export function AwakenPluginsDemo() {
  const [items, setItems] = useState(awakenPluginsDemoItems);
  const [searchValue, setSearchValue] = useState("");
  const [detailName, setDetailName] = useState<string | null>(null);
  const [installOpen, setInstallOpen] = useState(false);
  const [mode, setMode] = useState<AwakenPluginInstallMode>("local");
  const [localPath, setLocalPath] = useState("");
  const [url, setUrl] = useState("");
  const [replace, setReplace] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [validation, setValidation] = useState<AwakenPluginValidationSummary | null>(null);

  return (
    <section className="ads-component-demo" aria-label="插件组件族演示">
      <header className="ads-component-demo-header">
        <h2>
          插件 <code>AwakenPlugins</code>
        </h2>
      </header>
      <div style={{ height: 640 }}>
        <AwakenPlugins
          searchValue={searchValue}
          plugins={items}
          onSearchChange={setSearchValue}
          onRefresh={() => undefined}
          onInstall={() => setInstallOpen(true)}
          onPluginOpen={(plugin) => setDetailName(plugin.name)}
          onPluginEnabledChange={(plugin, enabled) => {
            setItems((current) =>
              current.map((item) =>
                item.name === plugin.name ? { ...item, enabled } : item,
              ),
            );
          }}
        />
      </div>
      <AwakenPluginInstallDialog
        open={installOpen}
        mode={mode}
        localPath={localPath}
        url={url}
        replace={replace}
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
        onReplaceChange={(value) => {
          setReplace(value);
          setValidation(null);
        }}
        onConfirmChange={setConfirm}
        onValidate={() => setValidation(installValidation)}
        onSubmit={() => setInstallOpen(false)}
      />
      <AwakenPluginDetailDialog
        open={detailName !== null}
        name={detailName ?? ""}
        version="1.2.0"
        description="示例插件"
        sourceLabel="remote_git https://example.test/acme-tools.git"
        digest="sha256:demo"
        skills={[
          {
            id: "plugin__acme-tools__lint",
            name: "lint",
            previewable: true,
          },
        ]}
        servers={[
          {
            shortName: "validator",
            identity: "plugin__acme.tools__validator",
            state: "connected",
            enabled: true,
          },
        ]}
        onOpenChange={(open) => {
          if (!open) setDetailName(null);
        }}
        onPreviewSkill={() => undefined}
        onServerEnabledChange={() => undefined}
        onUpgrade={() => undefined}
        onUninstall={() => setDetailName(null)}
      />
    </section>
  );
}
