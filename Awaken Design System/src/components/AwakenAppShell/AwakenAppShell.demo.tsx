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
      className="ads-component-demo aw-app-shell-demo"
      aria-label="应用外壳 AwakenAppShell 组件演示"
    >
      <header className="ads-component-demo-header">
        <h2>应用外壳 <code>AwakenAppShell</code></h2>
        <p>用于搭建 Awaken 桌面外壳的全局布局，分配顶部 chrome、侧边栏和主工作区。</p>
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
