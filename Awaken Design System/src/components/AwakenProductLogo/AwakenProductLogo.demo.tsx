import { AwakenProductLogo } from "./AwakenProductLogo";
import { AwakenProductLogoStates } from "./AwakenProductLogo.states";

export function AwakenProductLogoDemo() {
  return (
    <section className="ads-component-demo" aria-label="产品 LOGO AwakenProductLogo 组件演示">
      <header className="ads-component-demo-header">
        <h2>产品 LOGO <code>AwakenProductLogo</code></h2>
        <p>应用内默认使用透明双柱标记；完整 App Icon PNG 只留给 Dock 与安装包场景。</p>
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
