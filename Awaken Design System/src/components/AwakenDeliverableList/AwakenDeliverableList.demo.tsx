import type { ComponentType } from "react";
import { AwakenDeliverableCard } from "../AwakenDeliverableCard/AwakenDeliverableCard";
import { AwakenDeliverableCardStates } from "../AwakenDeliverableCard/AwakenDeliverableCard.states";
import { AwakenDeliverableList } from "./AwakenDeliverableList";
import { AwakenDeliverableListStates } from "./AwakenDeliverableList.states";

export function AwakenDeliverableListDemo() {
  const CardComponent = AwakenDeliverableCard as unknown as ComponentType<Record<string, unknown>>;
  const ListComponent = AwakenDeliverableList as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="最终交付物组件库演示">
      <header className="ads-component-demo-header">
        <h2>
          最终交付物 <code>AwakenDeliverableCard + AwakenDeliverableList</code>
        </h2>
        <p>
          集中观测单个交付物卡片与列表组合状态。Desktop 中卡片负责文件类型与打开操作，
          列表负责排列、展开和终态提示。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDeliverableCardStates.map((state) => (
          <article className="ads-component-demo-state" key={`card:${state.id}`}>
            <h3>交付物卡片 · {state.name}</h3>
            <p>{state.description}</p>
            <div className="ads-component-demo-canvas">
              <CardComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
        {AwakenDeliverableListStates.map((state) => (
          <article className="ads-component-demo-state" key={`list:${state.id}`}>
            <h3>交付物列表 · {state.name}</h3>
            <p>{state.description}</p>
            <div className="ads-component-demo-canvas">
              <ListComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
