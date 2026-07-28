import "./AwakenWorkbenchChrome.css";
import "./AwakenWorkbenchChrome.demo.css";
import { AwakenWorkbenchChrome } from "./AwakenWorkbenchChrome";
import { AwakenWorkbenchChromeStates } from "./AwakenWorkbenchChrome.states";

export function AwakenWorkbenchChromeDemo() {
  return (
    <section className="ads-component-demo aw-framework-subdemo aw-workbench-chrome-demo" aria-label="工作台顶部控制 AwakenWorkbenchChrome 组件演示">
      <header className="ads-component-demo-header">
        <h2>工作台顶部控制 <code>AwakenWorkbenchChrome</code></h2>
        <p>它就是 Awaken 窗口最上方的控制栏：左侧开合项目侧栏，中间可承载当前会话标签，右侧由 Desktop 放入文件标签、启动状态和 Inspector 开关。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenWorkbenchChromeStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <div className="ads-component-demo-state-heading">
              <div>
                <span className="ads-component-demo-state-eyebrow">Desktop 状态</span>
                <h3>{state.label}</h3>
              </div>
              <p>{state.description}</p>
            </div>
            <div className="ads-component-demo-canvas">
              <div className="aw-framework-preview aw-framework-preview--chrome">
                <AwakenWorkbenchChrome {...state.props} platform="macos" />
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
