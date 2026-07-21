import "./AwakenToolTraceGroup.css";
import type { ComponentType } from "react";
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
          工具轨迹 <code>AwakenToolTraceGroup + AwakenToolTraceLine</code>
        </h2>
        <p>
          集中观测只读探索工具组与普通单次工具调用。示例标题使用 Desktop 当前投影的
          工具名或英文只读摘要，不额外翻译成“读取”“运行”等中文动作。
        </p>
      </header>
      <div className="ads-component-demo-grid">
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
