import "./AwakenToolTraceLine.css";
import type { ComponentType } from "react";
import { AwakenToolTraceLine } from "./AwakenToolTraceLine";
import { AwakenToolTraceLineStates } from "./AwakenToolTraceLine.states";

export function AwakenToolTraceLineDemo() {
  const Component = AwakenToolTraceLine as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="工具轨迹行 AwakenToolTraceLine 组件演示">
      <header className="ads-component-demo-header">
        <h2>工具轨迹行 <code>AwakenToolTraceLine</code></h2>
        <p>Tool、MCP、Skill 与终端共用单行摘要，点击后查看独立输出卡片。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenToolTraceLineStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <Component {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
