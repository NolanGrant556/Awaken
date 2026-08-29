import "./AwakenPluginInstallDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenIcon } from "../AwakenIcon";
import { useId } from "react";

export type AwakenPluginInstallMode = "local" | "url";

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
  skillCount: number;
  serverCount: number;
  diagnostics?: AwakenPluginValidationDiagnostic[];
};

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
  validation?: AwakenPluginValidationSummary | null;
  validating?: boolean;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: AwakenPluginInstallMode) => void;
  onLocalPathChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onRefChange?: (value: string) => void;
  onSubpathChange?: (value: string) => void;
  onReplaceChange?: (value: boolean) => void;
  onPickLocal?: () => void;
  onConfirmChange?: (value: boolean) => void;
  onValidate: () => void;
  onSubmit: () => void;
};

const RISK_TEXT =
  "安装后，插件内的 MCP 子进程将以当前用户身份运行，可以读写本机文件并联网，不受沙盒约束。只安装你信任的来源。";
const MAX_VISIBLE_DIAGNOSTICS = 4;
const MAX_DIAGNOSTIC_CHARACTERS = 180;

function diagnosticText(diagnostic: AwakenPluginValidationDiagnostic): string {
  return diagnostic.message?.trim() || diagnostic.code?.trim() || "未知诊断";
}

function boundedDiagnosticText(value: string): string {
  if (value.length <= MAX_DIAGNOSTIC_CHARACTERS) {
    return value;
  }
  return `${value.slice(0, MAX_DIAGNOSTIC_CHARACTERS - 1)}…`;
}

function diagnosticSeverityLabel(severity?: string): string {
  switch (severity?.toLowerCase()) {
    case "error":
      return "错误";
    case "warning":
      return "警告";
    default:
      return "提示";
  }
}

function PluginValidationResult({
  validation,
}: {
  validation: AwakenPluginValidationSummary;
}) {
  const diagnostics = validation.diagnostics ?? [];
  const visibleDiagnostics = diagnostics.slice(0, MAX_VISIBLE_DIAGNOSTICS);
  const hiddenDiagnosticCount = diagnostics.length - visibleDiagnostics.length;
  const digest = validation.digest?.trim() ?? "";

  return (
    <section
      className="aw-plugin-install-dialog__validation"
      data-valid={validation.valid ? "true" : "false"}
      role="status"
      aria-live="polite"
    >
      <header className="aw-plugin-install-dialog__validation-header">
        <span className="aw-plugin-install-dialog__validation-icon" aria-hidden="true">
          <AwakenIcon concept={validation.valid ? "success" : "error"} size={16} />
        </span>
        <div>
          <h3>{validation.valid ? "检查通过" : "检查未通过"}</h3>
          <p>
            {validation.valid
              ? "插件包可以安装，请核对可加载组件数量是否符合预期。"
              : "请修正插件包后重新检查。"}
          </p>
        </div>
      </header>
      <dl className="aw-plugin-install-dialog__validation-facts">
        <div>
          <dt>插件</dt>
          <dd>
            <strong>{validation.name?.trim() || "未识别"}</strong>
            <span>{validation.version?.trim() ? `v${validation.version.trim()}` : "未提供版本"}</span>
          </dd>
        </div>
        <div>
          <dt>可加载组件</dt>
          <dd>
            {validation.skillCount} Skills · {validation.serverCount} MCP Servers
          </dd>
        </div>
        <div className="aw-plugin-install-dialog__validation-digest">
          <dt>Digest</dt>
          <dd title={digest || undefined}>{digest || "未生成"}</dd>
        </div>
      </dl>
      {visibleDiagnostics.length > 0 ? (
        <div className="aw-plugin-install-dialog__diagnostics">
          <h4>诊断（{diagnostics.length}）</h4>
          <ul>
            {visibleDiagnostics.map((diagnostic, index) => {
              const fullText = diagnosticText(diagnostic);
              return (
                <li
                  key={`${diagnostic.code ?? "diagnostic"}-${index}`}
                  data-severity={diagnostic.severity?.toLowerCase() ?? "info"}
                >
                  <span>{diagnosticSeverityLabel(diagnostic.severity)}</span>
                  <p title={fullText}>
                    {diagnostic.code?.trim() ? <code>{diagnostic.code}</code> : null}
                    {fullText !== diagnostic.code?.trim() ? (
                      <span>{boundedDiagnosticText(fullText)}</span>
                    ) : null}
                  </p>
                </li>
              );
            })}
          </ul>
          {hiddenDiagnosticCount > 0 ? (
            <p className="aw-plugin-install-dialog__diagnostics-more">
              另有 {hiddenDiagnosticCount} 条诊断未展开
            </p>
          ) : null}
        </div>
      ) : (
        <p className="aw-plugin-install-dialog__diagnostics-empty">没有诊断</p>
      )}
    </section>
  );
}

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
  validation,
  validating = false,
  onOpenChange,
  onModeChange,
  onLocalPathChange,
  onUrlChange,
  onRefChange,
  onSubpathChange,
  onReplaceChange,
  onPickLocal,
  onConfirmChange,
  onValidate,
  onSubmit,
}: AwakenPluginInstallDialogProps) {
  const riskTitleId = useId();
  const sourcePresent = mode === "local" ? localPath.trim() !== "" : url.trim() !== "";
  const formLocked = busy || validating;
  const canSubmit = Boolean(
    validation?.valid
      && validation.digest?.trim()
      && sourcePresent
      && confirm
      && !formLocked,
  );
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
          <AwakenButton
            variant="default"
            size="md"
            loading={validating}
            disabled={!sourcePresent || formLocked}
            onClick={onValidate}
          >
            {validating ? "检查中…" : "检查插件"}
          </AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} disabled={!canSubmit} onClick={onSubmit}>
            {busy ? "安装中…" : "确认安装"}
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
            disabled={formLocked}
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
            disabled={formLocked}
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
                  disabled={formLocked}
                  onChange={(event) => onLocalPathChange(event.target.value)}
                />
                {onPickLocal ? (
                  <AwakenButton
                    variant="default"
                    size="lg"
                    disabled={formLocked}
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
                  disabled={formLocked}
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
                    disabled={formLocked}
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
                    disabled={formLocked}
                    onChange={(event) => onSubpathChange?.(event.target.value)}
                  />
                </label>
              </div>
            </>
          )}
        </div>
        {validation ? <PluginValidationResult validation={validation} /> : null}
        <label className="aw-plugin-install-dialog__option">
          <input
            type="checkbox"
            checked={replace}
            disabled={formLocked}
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
              disabled={formLocked}
              onChange={(event) => onConfirmChange?.(event.target.checked)}
            />
            <span>我已了解 MCP 子进程无沙盒，并以当前用户身份运行</span>
          </label>
        </section>
      </div>
    </AwakenDialog>
  );
}
