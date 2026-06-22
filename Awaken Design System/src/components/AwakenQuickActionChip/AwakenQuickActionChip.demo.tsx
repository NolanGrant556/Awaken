import "./AwakenQuickActionChip.css";
import type { ComponentType } from "react";
import { AwakenQuickActionChip } from "./AwakenQuickActionChip";
import { AwakenQuickActionChipStates } from "./AwakenQuickActionChip.states";

export function AwakenQuickActionChipDemo() {
  const Component = AwakenQuickActionChip as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenQuickActionChipStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="快捷动作标签 AwakenQuickActionChip 组件演示">
      <header className="ads-component-demo-header">
        <h2>快捷动作标签 <code>AwakenQuickActionChip</code></h2>
        <p>用于展示可一键填入 composer 的快捷任务建议。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenQuickActionChipStates.map((state) => (
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
