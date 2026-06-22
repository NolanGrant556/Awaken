import "./AwakenArtifactCard.css";
import type { ComponentType } from "react";
import { AwakenArtifactCard } from "./AwakenArtifactCard";
import { AwakenArtifactCardStates } from "./AwakenArtifactCard.states";

export function AwakenArtifactCardDemo() {
  const Component = AwakenArtifactCard as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenArtifactCardStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="产物卡片 AwakenArtifactCard 组件演示">
      <header className="ads-component-demo-header">
        <h2>产物卡片 <code>AwakenArtifactCard</code></h2>
        <p>用于展示单个文件产物或变更项，并用图标区分新增、修改、删除和移动。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenArtifactCardStates.map((state) => (
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
