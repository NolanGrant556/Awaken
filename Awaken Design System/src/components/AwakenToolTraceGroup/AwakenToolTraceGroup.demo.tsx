import "./AwakenToolTraceGroup.css";
import type { ComponentType } from "react";
import { AwakenDisclosure } from "../AwakenDisclosure/AwakenDisclosure";
import { AwakenToolTraceLine } from "../AwakenToolTraceLine/AwakenToolTraceLine";
import { AwakenToolTraceLineStates } from "../AwakenToolTraceLine/AwakenToolTraceLine.states";
import { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";
import { AwakenToolTraceGroupStates } from "./AwakenToolTraceGroup.states";

export function AwakenToolTraceGroupDemo() {
  const GroupComponent = AwakenToolTraceGroup as unknown as ComponentType<Record<string, unknown>>;
  const LineComponent = AwakenToolTraceLine as unknown as ComponentType<Record<string, unknown>>;

  return (
    <section className="ads-component-demo" aria-label="工具轨迹组件库演示">
      <header className="ads-component-demo-header">
        <h2>
          工具轨迹 <code>AwakenDisclosure + AwakenToolTraceGroup + AwakenToolTraceLine</code>
        </h2>
        <p>
          先按真实 Desktop 终态展示“执行过程”如何收纳工具轨迹，再集中观测只读探索
          工具组与普通单次工具调用。示例标题使用 Desktop 当前投影的工具名或英文只读
          摘要，不额外翻译成“读取”“运行”等中文动作。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state">
          <h3>执行过程 · 默认收起</h3>
          <div className="ads-component-demo-canvas">
            <AwakenDisclosure title="执行过程" defaultOpen={false}>
              <GroupComponent
                {...(AwakenToolTraceGroupStates[0].props as Record<string, unknown>)}
                animateItems={false}
              />
            </AwakenDisclosure>
          </div>
        </article>
        <article className="ads-component-demo-state">
          <h3>执行过程 · 已展开</h3>
          <div className="ads-component-demo-canvas">
            <AwakenDisclosure title="执行过程" defaultOpen>
              <GroupComponent
                {...(AwakenToolTraceGroupStates[0].props as Record<string, unknown>)}
                animateItems={false}
              />
            </AwakenDisclosure>
          </div>
        </article>
        {AwakenToolTraceGroupStates.map((state) => (
          <article key={`group:${state.id}`} className="ads-component-demo-state">
            <h3>工具轨迹组 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <GroupComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
        {AwakenToolTraceLineStates.map((state) => (
          <article key={`line:${state.id}`} className="ads-component-demo-state">
            <h3>工具轨迹行 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <LineComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
