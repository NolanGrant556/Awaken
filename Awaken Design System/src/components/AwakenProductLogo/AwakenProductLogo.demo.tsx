import { AwakenProductLogo } from "./AwakenProductLogo";
import { AwakenProductLogoStates } from "./AwakenProductLogo.states";

export function AwakenProductLogoDemo() {
  return (
    <section className="ads-component-demo" aria-label="产品 LOGO AwakenProductLogo 组件演示">
      <header className="ads-component-demo-header">
        <h2>产品 LOGO <code>AwakenProductLogo</code></h2>
        <p>Awaken 官方方形产品标志。组件与 Desktop 共同使用同一份设计系统品牌资源。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenProductLogoStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenProductLogo {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
