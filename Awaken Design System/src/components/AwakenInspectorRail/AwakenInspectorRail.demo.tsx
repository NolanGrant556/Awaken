import "./AwakenInspectorRail.css";
import "./AwakenInspectorRail.demo.css";
import { AwakenInspectorRail } from "./AwakenInspectorRail";
import { AwakenInspectorRailStates } from "./AwakenInspectorRail.states";

export function AwakenInspectorRailDemo() {
  return (
    <section className="ads-component-demo aw-framework-subdemo aw-inspector-rail-demo" aria-label="检查器侧轨 AwakenInspectorRail 组件演示">
      <header className="ads-component-demo-header">
        <h2>检查器侧轨 <code>AwakenInspectorRail</code></h2>
        <p>它是 Workbench 右侧 300px 栏内的内容骨架。当前 Desktop 只有 “On Awaken” 标题和禁用的“右侧栏待开发”占位项；下面的完整高度为空，因为详情能力尚未接线。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenInspectorRailStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <div className="ads-component-demo-state-heading">
              <div>
                <span className="ads-component-demo-state-eyebrow">Desktop 当前位置</span>
                <h3>{state.label}</h3>
              </div>
              <p>左边是运行舞台；右边整列才是当前 InspectorRail，而不是只有那一行占位文字。</p>
            </div>
            <div className="ads-component-demo-canvas">
              <div className="aw-inspector-rail-demo__context">
                <div className="aw-inspector-rail-demo__stage" aria-hidden="true">
                  <strong>运行舞台</strong>
                  <span>执行叙事与 Composer 位于这里</span>
                </div>
                <div className="aw-inspector-rail-demo__surface">
                  <span className="aw-inspector-rail-demo__width-label">当前右侧栏 · 300px</span>
                  <AwakenInspectorRail {...state.props} />
                </div>
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
