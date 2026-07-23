import "./AwakenSwitch.demo.css";
import { useState } from "react";
import { AwakenSwitch } from "./AwakenSwitch";
import { AwakenSwitchStates } from "./AwakenSwitch.states";

export function AwakenSwitchDemo() {
  const [checked, setChecked] = useState(true);

  return (
    <section className="ads-component-demo" aria-label="开关 AwakenSwitch 组件演示">
      <header className="ads-component-demo-header">
        <h2>开关 <code>AwakenSwitch</code></h2>
        <p>所有布尔启停入口共用的受控原子组件；开启为绿色，关闭为中性灰色。</p>
      </header>
      <div className="aw-switch-demo__interactive">
        <span>{checked ? "已开启" : "已关闭"}</span>
        <AwakenSwitch checked={checked} label="演示开关" onCheckedChange={setChecked} />
      </div>
      <div className="aw-switch-demo__states">
        {AwakenSwitchStates.map((state) => (
          <article key={state.id}>
            <span>{state.label}</span>
            <AwakenSwitch {...state.props} />
          </article>
        ))}
      </div>
    </section>
  );
}
