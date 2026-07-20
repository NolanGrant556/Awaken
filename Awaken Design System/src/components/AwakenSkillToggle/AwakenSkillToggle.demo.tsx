import { useState } from "react";
import { AwakenSkillToggle } from "./AwakenSkillToggle";
import { AwakenSkillToggleStates } from "./AwakenSkillToggle.states";

export function AwakenSkillToggleDemo() {
  const [checked, setChecked] = useState(true);

  return (
    <section className="ads-component-demo" aria-label="Skill 开关 AwakenSkillToggle 组件演示">
      <header className="ads-component-demo-header">
        <h2>Skill 开关 <code>AwakenSkillToggle</code></h2>
        <p>受控启停开关，提供启用、停用、处理中和不可操作状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state">
          <h3>实时交互</h3>
          <div className="ads-component-demo-canvas">
            <AwakenSkillToggle checked={checked} label="月度结账复核" onCheckedChange={setChecked} />
            <span>{checked ? "已启用" : "已停用"}</span>
          </div>
        </article>
        {AwakenSkillToggleStates.map((state) => (
          <article className="ads-component-demo-state" key={state.id}>
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenSkillToggle {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
