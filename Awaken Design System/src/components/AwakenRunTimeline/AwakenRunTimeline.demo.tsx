import "./AwakenRunTimeline.css";
import type { ComponentType } from "react";
import { AwakenRunTimeline } from "./AwakenRunTimeline";
import { AwakenRunTimelineStates } from "./AwakenRunTimeline.states";

export function AwakenRunTimelineDemo() {
  const Component = AwakenRunTimeline as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenRunTimelineStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="运行时间线 AwakenRunTimeline 组件演示">
      <header className="ads-component-demo-header">
        <h2>运行时间线 <code>AwakenRunTimeline</code></h2>
        <p>用于汇总一次运行的阶段状态和步骤进度。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenRunTimelineStates.map((state) => (
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
