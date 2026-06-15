import { ArrowUp, ChevronDown, FolderOpen, GitBranch, Mic, Paperclip, Plus, ShieldCheck, Sparkles } from "lucide-react";
import { IconButton, StatusPill, UiButton } from "./UiPrimitives";

type ChatComposerProps = {
  placeholder: string;
  modelName: string;
  workspaceName: string;
  permissionLabel: string;
  onSubmit?: () => void;
};

export function ChatComposer({
  placeholder,
  modelName,
  workspaceName,
  permissionLabel,
  onSubmit,
}: ChatComposerProps) {
  return (
    <section className="composer-card" aria-label="聊天输入">
      <textarea placeholder={placeholder} aria-label="输入任务" rows={3} />

      <div className="composer-actions">
        <div className="composer-left-actions">
          <IconButton label="添加上下文" className="composer-plus">
            <Plus size={20} />
          </IconButton>
          <StatusPill tone="permission" icon={<ShieldCheck size={16} />}>
            {permissionLabel}
          </StatusPill>
          <StatusPill tone="local" icon={<Sparkles size={15} />}>
            本地模式
          </StatusPill>
        </div>

        <div className="composer-right-actions">
          <UiButton className="model-button" icon={<Sparkles size={15} />}>
            {modelName}
          </UiButton>
          <IconButton label="语音输入">
            <Mic size={18} />
          </IconButton>
          <IconButton label="发送" className="send-button" onClick={onSubmit}>
            <ArrowUp size={20} />
          </IconButton>
        </div>
      </div>

      <div className="composer-context">
        <UiButton icon={<FolderOpen size={16} />}>
          {workspaceName}
          <ChevronDown size={15} />
        </UiButton>
        <UiButton icon={<GitBranch size={15} />}>
          main
          <ChevronDown size={15} />
        </UiButton>
        <UiButton icon={<Paperclip size={15} />}>添加文件</UiButton>
      </div>
    </section>
  );
}
