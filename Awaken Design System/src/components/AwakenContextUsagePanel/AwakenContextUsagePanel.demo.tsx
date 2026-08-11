import "./AwakenContextUsagePanel.css";
import type { ComponentType } from "react";
import { AwakenContextUsagePanel } from "./AwakenContextUsagePanel";
import { AwakenContextUsagePanelStates } from "./AwakenContextUsagePanel.states";

export function AwakenContextUsagePanelDemo() {
  const Component = AwakenContextUsagePanel as unknown as ComponentType<
    Record<string, unknown>
  >;
  const defaultProps = AwakenContextUsagePanelStates[0]?.props ?? {};
  return (
    <section className="ads-component-demo" aria-label="上下文用量面板组件演示">
      <header className="ads-component-demo-header">
        <h2>
          上下文用量面板 <code>AwakenContextUsagePanel</code>
        </h2>
        <p>展示当前下一次模型请求的六项分类、可选协议开销与输入窗口。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenContextUsagePanelStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div
              className="ads-component-demo-canvas"
              style={{ minHeight: 520 }}
            >
              <Component
                {...(defaultProps as Record<string, unknown>)}
                {...(state.props as Record<string, unknown>)}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
