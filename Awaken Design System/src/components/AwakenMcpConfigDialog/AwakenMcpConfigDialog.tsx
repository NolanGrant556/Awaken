import "./AwakenMcpConfigDialog.css";
import { Check } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";

export type AwakenMcpConfigDialogProps = {
  open: boolean;
  value: string;
  errorMessage?: string | null;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onValueChange: (value: string) => void;
  onSubmit: () => void;
};

export function AwakenMcpConfigDialog({
  open,
  value,
  errorMessage,
  busy = false,
  onOpenChange,
  onValueChange,
  onSubmit,
}: AwakenMcpConfigDialogProps) {
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
            <Check size={15} strokeWidth={1.9} />确认
          </AwakenButton>
        </>
      }
    >
      <div className="aw-mcp-config-dialog__body">
        {errorMessage ? <div className="aw-mcp-config-dialog__error" role="alert">{errorMessage}</div> : null}
        <p>粘贴 MCP Servers 配置 JSON。支持 <code>mcpServers</code> 简写和 Awaken <code>servers</code> 配置。</p>
        <textarea value={value} spellCheck={false} onChange={(event) => onValueChange(event.target.value)} />
        <div className="aw-mcp-config-dialog__note">配置前确认来源和命令参数，避免运行不可信服务。</div>
      </div>
    </AwakenDialog>
  );
}
