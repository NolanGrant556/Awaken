import type { ComponentType } from "react";
import { AwakenDeliverableList } from "./AwakenDeliverableList";
import { AwakenDeliverableListStates } from "./AwakenDeliverableList.states";

export function AwakenDeliverableListDemo() {
  const Component = AwakenDeliverableList as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="最终交付物列表组件演示">
      <header className="ads-component-demo-header">
        <h2>最终交付物列表 <code>AwakenDeliverableList</code></h2>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDeliverableListStates.map((state) => (
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
