import "./AwakenNewTaskButton.css";
import type { ComponentType } from "react";
import { AwakenNewTaskButton } from "./AwakenNewTaskButton";
import { AwakenNewTaskButtonStates } from "./AwakenNewTaskButton.states";

export function AwakenNewTaskButtonDemo() {
  const Component = AwakenNewTaskButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenNewTaskButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="新任务按钮 AwakenNewTaskButton 组件演示">
      <header className="ads-component-demo-header">
        <h2>新任务按钮 <code>AwakenNewTaskButton</code></h2>
        <p>用于在侧边栏或任务入口区域触发新建任务。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenNewTaskButtonStates.map((state) => (
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
