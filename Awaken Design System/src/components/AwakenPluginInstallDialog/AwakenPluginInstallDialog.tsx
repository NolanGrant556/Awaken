import "./AwakenPluginInstallDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenIcon } from "../AwakenIcon";
import { useId } from "react";

export type AwakenPluginInstallMode = "local" | "url";

export type AwakenPluginInstallDialogProps = {
  open: boolean;
  mode: AwakenPluginInstallMode;
  localPath: string;
  url: string;
  refValue?: string;
  subpath?: string;
  replace?: boolean;
  confirm?: boolean;
  errorMessage?: string | null;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: AwakenPluginInstallMode) => void;
  onLocalPathChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onRefChange?: (value: string) => void;
  onSubpathChange?: (value: string) => void;
  onReplaceChange?: (value: boolean) => void;
  onPickLocal?: () => void;
  onConfirmChange?: (value: boolean) => void;
  onSubmit: () => void;
};

const RISK_TEXT =
  "安装后，插件内的 MCP 子进程将以当前用户身份运行，可以读写本机文件并联网，不受沙盒约束。只安装你信任的来源。";

export function AwakenPluginInstallDialog({
  open,
  mode,
  localPath,
  url,
  refValue = "",
  subpath = "",
  replace = false,
  confirm = false,
  errorMessage,
  busy = false,
  onOpenChange,
  onModeChange,
  onLocalPathChange,
  onUrlChange,
  onRefChange,
  onSubpathChange,
  onReplaceChange,
  onPickLocal,
  onConfirmChange,
  onSubmit,
}: AwakenPluginInstallDialogProps) {
  const riskTitleId = useId();
  const canSubmit = confirm && (mode === "local" ? localPath.trim() !== "" : url.trim() !== "");
  return (
    <AwakenDialog
      open={open}
      className="aw-plugin-install-dialog"
      title="安装插件"
      onOpenChange={onOpenChange}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={busy} onClick={() => onOpenChange(false)}>
            取消
          </AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} disabled={!canSubmit} onClick={onSubmit}>
            确认安装
          </AwakenButton>
        </>
      }
    >
      <div className="aw-plugin-install-dialog__body">
        <div className="aw-plugin-install-dialog__intro">
          <h3>选择插件来源</h3>
          <p>支持本地目录、zip 文件，以及 HTTPS zip 或 Git 仓库。</p>
        </div>
        {errorMessage ? (
          <div className="aw-plugin-install-dialog__error" role="alert">
            {errorMessage}
          </div>
        ) : null}
        <div className="aw-plugin-install-dialog__modes" role="tablist" aria-label="插件安装来源">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "local"}
            data-active={mode === "local" ? "true" : "false"}
            disabled={busy}
            onClick={() => onModeChange("local")}
          >
            <AwakenIcon concept="project" size={16} />
            本地目录 / zip
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "url"}
            data-active={mode === "url" ? "true" : "false"}
            disabled={busy}
            onClick={() => onModeChange("url")}
          >
            <AwakenIcon concept="provider-link" size={16} />
            URL
          </button>
        </div>
        <div className="aw-plugin-install-dialog__fields">
          {mode === "local" ? (
            <label className="aw-plugin-install-dialog__field">
              <span>本地路径</span>
              <span className="aw-plugin-install-dialog__path">
                <input
                  value={localPath}
                  placeholder="选择插件目录，或输入 zip 文件路径"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={busy}
                  onChange={(event) => onLocalPathChange(event.target.value)}
                />
                {onPickLocal ? (
                  <AwakenButton
                    variant="default"
                    size="lg"
                    disabled={busy}
                    onClick={onPickLocal}
                  >
                    浏览
                  </AwakenButton>
                ) : null}
              </span>
            </label>
          ) : (
            <>
              <label className="aw-plugin-install-dialog__field">
                <span>HTTPS URL</span>
                <input
                  value={url}
                  placeholder="https://example.com/plugin.zip"
                  inputMode="url"
                  autoComplete="url"
                  spellCheck={false}
                  disabled={busy}
                  onChange={(event) => onUrlChange(event.target.value)}
                />
              </label>
              <div className="aw-plugin-install-dialog__optional-fields">
                <label className="aw-plugin-install-dialog__field">
                  <span>
                    Git ref <small>可选</small>
                  </span>
                  <input
                    value={refValue}
                    placeholder="main 或版本标签"
                    autoComplete="off"
                    spellCheck={false}
                    disabled={busy}
                    onChange={(event) => onRefChange?.(event.target.value)}
                  />
                </label>
                <label className="aw-plugin-install-dialog__field">
                  <span>
                    子目录 <small>可选</small>
                  </span>
                  <input
                    value={subpath}
                    placeholder="plugins/example"
                    autoComplete="off"
                    spellCheck={false}
                    disabled={busy}
                    onChange={(event) => onSubpathChange?.(event.target.value)}
                  />
                </label>
              </div>
            </>
          )}
        </div>
        <label className="aw-plugin-install-dialog__option">
          <input
            type="checkbox"
            checked={replace}
            disabled={busy}
            onChange={(event) => onReplaceChange?.(event.target.checked)}
          />
          <span>
            <strong>同名时替换已安装插件</strong>
            <small>旧插件包会先移入回收站；插件数据在升级时保留。</small>
          </span>
        </label>
        <section
          className="aw-plugin-install-dialog__risk"
          aria-labelledby={riskTitleId}
        >
          <span className="aw-plugin-install-dialog__risk-icon" aria-hidden="true">
            <AwakenIcon concept="warning" size={16} />
          </span>
          <div className="aw-plugin-install-dialog__risk-copy">
            <h3 id={riskTitleId}>插件可访问你的电脑</h3>
            <p>{RISK_TEXT}</p>
          </div>
          <label className="aw-plugin-install-dialog__risk-confirm">
            <input
              type="checkbox"
              checked={confirm}
              disabled={busy}
              onChange={(event) => onConfirmChange?.(event.target.checked)}
            />
            <span>我已了解 MCP 子进程无沙盒，并以当前用户身份运行</span>
          </label>
        </section>
      </div>
    </AwakenDialog>
  );
}
