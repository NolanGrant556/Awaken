import { useState, type DragEvent } from "react";
import "./AwakenPluginInstallDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";

export type AwakenPluginValidationDiagnostic = {
  code?: string;
  message?: string;
  severity?: string;
};

export type AwakenPluginValidationSummary = {
  valid: boolean;
  name?: string;
  version?: string;
  digest?: string;
  diagnostics?: AwakenPluginValidationDiagnostic[];
};

export type AwakenPluginInstallDialogProps = {
  open: boolean;
  localPath: string;
  errorMessage?: string | null;
  busy?: boolean;
  validation?: AwakenPluginValidationSummary | null;
  validating?: boolean;
  onOpenChange: (open: boolean) => void;
  onPickLocal?: () => void;
  onDropFile?: (file: File) => void;
  onSubmit: () => void;
};

function fileNameFromPath(value: string): string {
  const normalized = value.trim().replace(/\\/g, "/");
  return normalized.split("/").pop() || normalized;
}

export function AwakenPluginInstallDialog({
  open,
  localPath,
  errorMessage,
  busy = false,
  validation,
  validating = false,
  onOpenChange,
  onPickLocal,
  onDropFile,
  onSubmit,
}: AwakenPluginInstallDialogProps) {
  const [dragActive, setDragActive] = useState(false);
  const sourcePresent = localPath.trim() !== "";
  const formLocked = busy || validating;
  const valid = Boolean(validation?.valid && validation.digest?.trim() && sourcePresent);
  const invalid = Boolean(errorMessage || (validation && !validation.valid));
  const showErrorDetail = Boolean(
    errorMessage && errorMessage !== "插件 ZIP 格式不符合要求",
  );
  const state = validating ? "validating" : valid ? "valid" : invalid ? "invalid" : "idle";
  const fileName = fileNameFromPath(localPath);
  const pluginLabel = [validation?.name?.trim(), validation?.version?.trim() ? `v${validation.version.trim()}` : ""]
    .filter(Boolean)
    .join(" · ");

  function handleDragOver(event: DragEvent<HTMLButtonElement>): void {
    event.preventDefault();
    if (formLocked) return;
    event.dataTransfer.dropEffect = "copy";
    setDragActive(true);
  }

  function handleDragLeave(event: DragEvent<HTMLButtonElement>): void {
    const nextTarget = event.relatedTarget;
    if (!(nextTarget instanceof Node) || !event.currentTarget.contains(nextTarget)) {
      setDragActive(false);
    }
  }

  function handleDrop(event: DragEvent<HTMLButtonElement>): void {
    event.preventDefault();
    setDragActive(false);
    if (formLocked) return;
    const file = event.dataTransfer.files.item(0);
    if (file) onDropFile?.(file);
  }

  return (
    <AwakenDialog
      open={open}
      className="aw-plugin-install-dialog"
      title="安装插件"
      onOpenChange={onOpenChange}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={formLocked} onClick={() => onOpenChange(false)}>
            取消
          </AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} disabled={!valid || formLocked} onClick={onSubmit}>
            {busy ? "安装中…" : "安装"}
          </AwakenButton>
        </>
      }
    >
      <div className="aw-plugin-install-dialog__body">
        <p className="aw-plugin-install-dialog__compatibility">
          兼容 Codex、Cursor、Claude Code、Agent Plugins 格式
        </p>
        <button
          type="button"
          className="aw-plugin-install-dialog__dropzone"
          data-state={state}
          data-drag-active={dragActive ? "true" : "false"}
          disabled={formLocked}
          aria-busy={validating}
          aria-label="选择或拖放插件 ZIP 包"
          onClick={onPickLocal}
          onDragEnter={handleDragOver}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <strong role="status" aria-live="polite">
            {validating
              ? "正在校验插件 ZIP…"
              : valid
                ? "插件 ZIP 格式正确"
                : invalid
                  ? "插件 ZIP 格式不符合要求"
                  : "点击选择或拖放插件 ZIP"}
          </strong>
          <span>
            {validating
              ? fileName
              : valid
                ? pluginLabel || fileName
                : invalid
                  ? "请重新选择 .zip 插件包"
                  : "仅支持 .zip 文件"}
          </span>
          {fileName && !validating && !valid ? <code>{fileName}</code> : null}
        </button>
        {showErrorDetail ? (
          <div className="aw-plugin-install-dialog__error" role="alert">
            {errorMessage}
          </div>
        ) : null}
      </div>
    </AwakenDialog>
  );
}
