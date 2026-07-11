import type { ComponentType } from "react";
import { AwakenDeliverableCard } from "./AwakenDeliverableCard";
import { AwakenDeliverableCardStates } from "./AwakenDeliverableCard.states";

export function AwakenDeliverableCardDemo() {
  const Component = AwakenDeliverableCard as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="最终交付物卡片组件演示">
      <header className="ads-component-demo-header">
        <h2>最终交付物卡片 <code>AwakenDeliverableCard</code></h2>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDeliverableCardStates.map((state) => (
          <article className="ads-component-state" key={state.id}>
            <h3>{state.name}</h3>
            <p>{state.description}</p>
            <Component {...state.props} />
          </article>
        ))}
      </div>
    </section>
  );
}
