import "./AwakenTaskWelcome.css";
import type { ComponentType } from "react";
import { AwakenTaskWelcome } from "./AwakenTaskWelcome";
import { AwakenTaskWelcomeStates } from "./AwakenTaskWelcome.states";

export function AwakenTaskWelcomeDemo() {
  const Component = AwakenTaskWelcome as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenTaskWelcomeStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="任务欢迎区 AwakenTaskWelcome 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          任务欢迎区 <code>AwakenTaskWelcome</code>
        </h2>
        <p>用于新建任务的空闲态，在中心 Composer 上方展示 Awaken 字标和一句说明文案。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenTaskWelcomeStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
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
