import "./AwakenPluginInstallDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";

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
        {errorMessage ? <div role="alert">{errorMessage}</div> : null}
        <div className="aw-plugin-install-dialog__modes" role="tablist" aria-label="插件安装来源">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "local"}
            data-active={mode === "local" ? "true" : "false"}
            disabled={busy}
            onClick={() => onModeChange("local")}
          >
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
            URL
          </button>
        </div>
        {mode === "local" ? (
          <label>
            本地路径
            <span className="aw-plugin-install-dialog__path">
              <input
                value={localPath}
                disabled={busy}
                onChange={(event) => onLocalPathChange(event.target.value)}
              />
              {onPickLocal ? (
                <AwakenButton variant="default" size="sm" disabled={busy} onClick={onPickLocal}>
                  浏览
                </AwakenButton>
              ) : null}
            </span>
          </label>
        ) : (
          <>
            <label>
              HTTPS URL
              <input value={url} disabled={busy} onChange={(event) => onUrlChange(event.target.value)} />
            </label>
            <label>
              Git ref（可选）
              <input value={refValue} disabled={busy} onChange={(event) => onRefChange?.(event.target.value)} />
            </label>
            <label>
              子目录（可选）
              <input value={subpath} disabled={busy} onChange={(event) => onSubpathChange?.(event.target.value)} />
            </label>
          </>
        )}
        <label className="aw-plugin-install-dialog__check">
          <input
            type="checkbox"
            checked={replace}
            disabled={busy}
            onChange={(event) => onReplaceChange?.(event.target.checked)}
          />
          同名时替换已安装插件
        </label>
        <p className="aw-plugin-install-dialog__risk">{RISK_TEXT}</p>
        <label className="aw-plugin-install-dialog__check">
          <input
            type="checkbox"
            checked={confirm}
            disabled={busy}
            onChange={(event) => onConfirmChange?.(event.target.checked)}
          />
          我已了解 MCP 子进程无沙盒，并以当前用户身份运行
        </label>
      </div>
    </AwakenDialog>
  );
}
