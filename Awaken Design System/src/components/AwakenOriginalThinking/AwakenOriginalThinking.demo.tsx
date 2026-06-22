import "./AwakenOriginalThinking.css";
import { AwakenOriginalThinking } from "./AwakenOriginalThinking";
import { AwakenOriginalThinkingStates } from "./AwakenOriginalThinking.states";

export function AwakenOriginalThinkingDemo() {
  const referenceState = AwakenOriginalThinkingStates[0];

  return (
    <section className="ads-component-demo aw-original-thinking-demo" aria-label="原始思考动效 AwakenOriginalThinking 组件演示">
      <header className="ads-component-demo-header">
        <h2>原始思考动效 <code>AwakenOriginalThinking</code></h2>
        <p>100% 复刻 Original Thinking 数学曲线 Loader 的默认视觉、公式、粒子尾迹与外链。</p>
      </header>
      <AwakenOriginalThinking {...referenceState.props} />
    </section>
  );
}
