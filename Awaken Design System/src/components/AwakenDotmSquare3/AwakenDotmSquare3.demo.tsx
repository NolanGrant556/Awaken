import "./AwakenDotmSquare3.demo.css";
import { AwakenDotmSquare3 } from "./AwakenDotmSquare3";
import { AwakenDotmSquare3States } from "./AwakenDotmSquare3.states";

export function AwakenDotmSquare3Demo() {
  return (
    <section className="ads-component-demo aw-dotm-square3-demo" aria-label="矩阵方块 AwakenDotmSquare3 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          矩阵方块 <code>AwakenDotmSquare3</code>
        </h2>
        <p>固定的 5×5 方形点阵加载指示器；侧栏线程运行状态复用同一组件。</p>
      </header>
      <div className="aw-dotm-square3-demo__grid">
        {AwakenDotmSquare3States.map((state) => (
          <div key={state.id} className="aw-dotm-square3-demo__cell">
            <div className="aw-dotm-square3-demo__orb">
              <AwakenDotmSquare3 {...state.props} />
            </div>
            <span className="aw-dotm-square3-demo__label">{state.label}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
