import "./AwakenAppShell.css";
import "./AwakenAppShell.demo.css";
import { AwakenAppShell } from "./AwakenAppShell";
import {
  AwakenAppShellStates,
  type AwakenAppShellState,
} from "./AwakenAppShell.states";

function AwakenAppShellPreview({ state }: { state: AwakenAppShellState }) {
  const { sidebar, main, chrome, ...shellProps } = state.props;

  return (
    <div className="aw-app-shell-demo__viewport">
      <AwakenAppShell
        {...shellProps}
        sidebar={<div className="aw-app-shell-demo__sidebar">{sidebar}</div>}
        main={<div className="aw-app-shell-demo__main">{main}</div>}
        chrome={
          chrome ? <div className="aw-app-shell-demo__chrome">{chrome}</div> : undefined
        }
      />
    </div>
  );
}

export function AwakenAppShellDemo() {
  return (
    <section
      className="ads-component-demo aw-app-shell-demo aw-framework-subdemo"
      aria-label="应用外壳 AwakenAppShell 组件演示"
    >
      <header className="ads-component-demo-header">
        <h2>应用外壳 <code>AwakenAppShell</code></h2>
        <p>它是整个 Awaken 窗口最外层的布局：先划分顶部控制栏、左侧项目导航和右侧主工作区，再由主工作区继续放入运行舞台与 Inspector。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenAppShellStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenAppShellPreview state={state} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
