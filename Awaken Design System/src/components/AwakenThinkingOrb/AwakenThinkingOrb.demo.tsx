import "./AwakenThinkingOrb.demo.css";

import { AwakenThinkingOrb } from "./AwakenThinkingOrb";
import { AwakenThinkingOrbStates } from "./AwakenThinkingOrb.states";
import { AwakenThinkingOrbStatus } from "./AwakenThinkingOrbStatus";

export function AwakenThinkingOrbDemo() {
  return (
    <section
      className="ads-component-demo aw-thinking-orb-demo"
      aria-label="思考球 AwakenThinkingOrb 组件演示"
    >
      <header className="ads-component-demo-header">
        <h2>
          思考球 <code>AwakenThinkingOrb</code>
        </h2>
        <p>
          基于官方 <code>thinking-orbs</code> 的 Canvas 动效，完整观测六种视觉状态与 64px、20px 两档专用尺寸。
        </p>
      </header>

      <section className="aw-thinking-orb-demo__featured" aria-labelledby="aw-thinking-orb-featured-title">
        <div className="aw-thinking-orb-demo__featured-stage" data-theme="dark">
          <AwakenThinkingOrbStatus theme="dark" />
        </div>
        <div className="aw-thinking-orb-demo__featured-copy">
          <h3 id="aw-thinking-orb-featured-title">思考中行内状态</h3>
          <p>
            思考球、<code>思考中</code> 文案和三点节奏动效使用透明行内布局；<code>state=&quot;composing&quot;</code> 仍是 Canvas 状态，不是第七种 state。
          </p>
        </div>
      </section>

      <div className="aw-thinking-orb-demo__grid">
        {AwakenThinkingOrbStates.map((state) => (
          <article key={state.id} className="aw-thinking-orb-demo__card">
            <div className="aw-thinking-orb-demo__stage" data-theme="dark">
              <AwakenThinkingOrb {...state.props} />
            </div>
            <div className="aw-thinking-orb-demo__meta">
              <strong>{state.label}</strong>
              <code>{state.props.state}</code>
              <span>{state.description}</span>
            </div>
          </article>
        ))}
      </div>

      <section className="aw-thinking-orb-demo__compact" aria-labelledby="aw-thinking-orb-compact-title">
        <header>
          <h3 id="aw-thinking-orb-compact-title">20px 行内尺寸</h3>
          <p>这是一套单独调校的紧凑预设，不是 64px Canvas 的缩放副本。</p>
        </header>
        <div className="aw-thinking-orb-demo__compact-list">
          {AwakenThinkingOrbStates.map((state) => (
            <div key={state.id} className="aw-thinking-orb-demo__compact-item" data-theme="dark">
              <AwakenThinkingOrb
                {...state.props}
                size={20}
                aria-label={`${state.props["aria-label"]}，紧凑尺寸`}
              />
              <span>{state.label}</span>
            </div>
          ))}
        </div>
      </section>
    </section>
  );
}
