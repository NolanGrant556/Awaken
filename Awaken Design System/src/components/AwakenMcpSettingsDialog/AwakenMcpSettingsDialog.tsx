import "./AwakenMcpSettingsDialog.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import type { AwakenMcpLogItem, AwakenMcpToolItem } from "../AwakenConnectors/AwakenConnectors.types";

export type AwakenMcpSettingsDialogProps = {
  open: boolean;
  serverName?: string;
  scopeLabel?: string;
  configValue: string;
  tools: AwakenMcpToolItem[];
  logs: AwakenMcpLogItem[];
  errorMessage?: string | null;
  loading?: boolean;
  saving?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfigValueChange: (value: string) => void;
  onDelete: () => void;
  onSave: () => void;
};

function formatLogTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString();
}

export function AwakenMcpSettingsDialog({
  open,
  serverName,
  scopeLabel,
  configValue,
  tools,
  logs,
  errorMessage,
  loading = false,
  saving = false,
  onOpenChange,
  onConfigValueChange,
  onDelete,
  onSave,
}: AwakenMcpSettingsDialogProps) {
  return (
    <AwakenDialog
      open={open}
      className="aw-mcp-settings-dialog"
      title={serverName ? `${serverName} 设置` : "MCP 设置"}
      onOpenChange={onOpenChange}
      footer={
        <div className="aw-mcp-settings-dialog__footer">
          <AwakenButton className="aw-mcp-settings-dialog__delete" variant="default" size="md" disabled={saving} onClick={onDelete}>
            <AwakenIcon concept="delete" size={16} />删除
          </AwakenButton>
          <div>
            <AwakenButton variant="default" size="md" disabled={saving} onClick={() => onOpenChange(false)}>取消</AwakenButton>
            <AwakenButton variant="primary" size="md" loading={saving} disabled={configValue.trim() === ""} onClick={onSave}>
              <AwakenIcon concept="step-done" size={16} />保存
            </AwakenButton>
          </div>
        </div>
      }
    >
      <div className="aw-mcp-settings-dialog__body">
        {errorMessage ? <div className="aw-mcp-settings-dialog__error" role="alert">{errorMessage}</div> : null}
        <section>
          <header><h3>配置</h3>{scopeLabel ? <span>{scopeLabel}</span> : null}</header>
          <textarea
            value={configValue}
            spellCheck={false}
            disabled={loading}
            onChange={(event) => onConfigValueChange(event.target.value)}
          />
        </section>
        <section>
          <h3>方法</h3>
          {tools.length > 0 ? (
            <ul className="aw-mcp-settings-dialog__tools">
              {tools.map((tool) => (
                <li key={tool.name}>
                  <code>{tool.name}</code>
                  <span>{tool.description}</span>
                  {tool.readOnly ? <em>只读</em> : null}
                </li>
              ))}
            </ul>
          ) : <div className="aw-mcp-settings-dialog__empty">尚未获取到工具</div>}
        </section>
        <section>
          <h3>日志</h3>
          {logs.length > 0 ? (
            <div className="aw-mcp-settings-dialog__logs">
              {logs.map((entry) => (
                <div key={entry.id} data-stream={entry.stream}>
                  <time>{formatLogTime(entry.timestamp)}</time>
                  <span>{entry.line}</span>
                </div>
              ))}
            </div>
          ) : <div className="aw-mcp-settings-dialog__empty">暂无相关日志</div>}
        </section>
      </div>
    </AwakenDialog>
  );
}
