import type { ComponentType } from "react";
import { AwakenMediaGenerationCard } from "./AwakenMediaGenerationCard";
import { AwakenMediaGenerationCardStates } from "./AwakenMediaGenerationCard.states";

export function AwakenMediaGenerationCardDemo() {
  const Component = AwakenMediaGenerationCard as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="媒体生成卡片演示">
      <header className="ads-component-demo-header">
        <h2>媒体生成卡片 <code>AwakenMediaGenerationCard</code></h2>
        <p>生成过程使用紧凑画幅，单图和最多 15 张组图都在同一批次卡片内展示。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenMediaGenerationCardStates.map((state) => (
          <article className="ads-component-demo-state" key={state.id}>
            <h3>{state.name}</h3>
            <p>{state.description}</p>
            <div className="ads-component-demo-canvas">
              <Component {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
