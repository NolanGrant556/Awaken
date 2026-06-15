import { CheckCircle2, Cpu, Sparkles } from "lucide-react";
import { ChatComposer } from "./ChatComposer";
import { StatusPill } from "./UiPrimitives";

type ChatHomeProps = {
  onSubmit?: () => void;
};

export function ChatHome({ onSubmit }: ChatHomeProps) {
  return (
    <div className="chat-home">
      <header className="top-strip">
        <StatusPill tone="local" icon={<CheckCircle2 size={15} />}>
          本地运行
        </StatusPill>
        <StatusPill tone="neutral" icon={<Cpu size={15} />}>
          awaken-core 待连接
        </StatusPill>
      </header>

      <section className="hero-chat">
        <div className="brand-glyph" aria-hidden="true">
          <Sparkles size={26} />
        </div>
        <h1>我们应该在 Awaken 中构建什么？</h1>
        <p>描述任务，添加上下文，让本地 Agent 接管下一步。</p>
        <ChatComposer
          placeholder="随心输入"
          modelName="Qwen3.7-Max"
          workspaceName="Awaken"
          permissionLabel="完全访问"
          onSubmit={onSubmit}
        />
      </section>
    </div>
  );
}
