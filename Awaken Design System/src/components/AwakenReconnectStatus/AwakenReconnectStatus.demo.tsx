import "./AwakenReconnectStatus.css";
import type { ComponentType } from "react";
import { AwakenReconnectStatus } from "./AwakenReconnectStatus";
import { AwakenReconnectStatusStates } from "./AwakenReconnectStatus.states";

export function AwakenReconnectStatusDemo() {
  const Component = AwakenReconnectStatus as unknown as ComponentType<
    Record<string, unknown>
  >;
  const defaultProps = AwakenReconnectStatusStates[0]?.props ?? {};

  return (
    <section
      className="ads-component-demo"
      aria-label="供应商重连状态 AwakenReconnectStatus 组件演示"
    >
      <header className="ads-component-demo-header">
        <h2>
          供应商重连 <code>AwakenReconnectStatus</code>
        </h2>
        <p>
          用于在 turn body 展示 live-only 的网络等待或重连状态，细节可展开，不读取运行时。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenReconnectStatusStates.map((state) => (
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
