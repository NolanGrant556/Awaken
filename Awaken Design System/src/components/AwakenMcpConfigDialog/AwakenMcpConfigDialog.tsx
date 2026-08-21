import "./AwakenMcpConfigDialog.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";

export type AwakenMcpConfigMode = "stdio" | "streamable-http";

export type AwakenMcpConfigDialogProps = {
  open: boolean;
  mode: AwakenMcpConfigMode;
  value: string;
  errorMessage?: string | null;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: AwakenMcpConfigMode) => void;
  onValueChange: (value: string) => void;
  onSubmit: () => void;
};

export function AwakenMcpConfigDialog({
  open,
  mode,
  value,
  errorMessage,
  busy = false,
  onOpenChange,
  onModeChange,
  onValueChange,
  onSubmit,
}: AwakenMcpConfigDialogProps) {
  const modeDescription = mode === "stdio"
    ? <>粘贴通过 <code>command</code> / <code>args</code> 启动的本地 MCP 配置 JSON。</>
    : <>粘贴远程 MCP URL 配置；兼容 <code>type: "streamable-http"</code>、<code>type: "http"</code> 或省略 type。</>;
  const safetyNote = mode === "stdio"
    ? "配置前确认命令来源和启动参数，避免运行不可信程序。"
    : "确认远程地址来源与访问权限；URL 中的 key、token 等凭据不要公开分享。";

  return (
    <AwakenDialog
      open={open}
      className="aw-mcp-config-dialog"
      title="配置 MCP"
      onOpenChange={onOpenChange}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={busy} onClick={() => onOpenChange(false)}>取消</AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} disabled={value.trim() === ""} onClick={onSubmit}>
            <AwakenIcon concept="step-done" size={16} />确认
          </AwakenButton>
        </>
      }
    >
      <div className="aw-mcp-config-dialog__body">
        {errorMessage ? <div className="aw-mcp-config-dialog__error" role="alert">{errorMessage}</div> : null}
        <div className="aw-mcp-config-dialog__modes" role="tablist" aria-label="MCP 配置方式">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "stdio"}
            data-active={mode === "stdio" ? "true" : "false"}
            disabled={busy}
            onClick={() => onModeChange("stdio")}
          >
            本地进程（stdio）
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "streamable-http"}
            data-active={mode === "streamable-http" ? "true" : "false"}
            disabled={busy}
            onClick={() => onModeChange("streamable-http")}
          >
            Streamable HTTP
          </button>
        </div>
        <p>{modeDescription} 支持 <code>mcpServers</code> 简写和 Awaken <code>servers</code> 配置。</p>
        <textarea
          aria-label={mode === "stdio" ? "本地进程 MCP 配置 JSON" : "Streamable HTTP MCP 配置 JSON"}
          value={value}
          spellCheck={false}
          onChange={(event) => onValueChange(event.target.value)}
        />
        <div className="aw-mcp-config-dialog__note">{safetyNote}</div>
      </div>
    </AwakenDialog>
  );
}
