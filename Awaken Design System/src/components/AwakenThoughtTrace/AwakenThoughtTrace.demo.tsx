import "./AwakenThoughtTrace.css";
import type { ComponentType } from "react";
import { AwakenThoughtTrace } from "./AwakenThoughtTrace";
import { AwakenThoughtTraceStates } from "./AwakenThoughtTrace.states";

export function AwakenThoughtTraceDemo() {
  const Component = AwakenThoughtTrace as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenThoughtTraceStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="思路轨迹 AwakenThoughtTrace 组件演示">
      <header className="ads-component-demo-header">
        <h2>思路轨迹 <code>AwakenThoughtTrace</code></h2>
        <p>
          思考中行内状态从提交后的首个等待帧开始持续显示；runtime 提供 reasoning
          summary 后提供正文展开入口；用户展开后文本继续流式更新，完成后保留用户选择和轻量回看入口。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenThoughtTraceStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <Component {...(defaultProps as Record<string, unknown>)} {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
