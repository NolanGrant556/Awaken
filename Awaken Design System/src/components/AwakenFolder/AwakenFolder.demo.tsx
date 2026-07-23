import "./AwakenFolder.demo.css";
import { useState } from "react";
import { AwakenFolder } from "./AwakenFolder";
import { AwakenFolderStates } from "./AwakenFolder.states";

export function AwakenFolderDemo() {
  const [openId, setOpenId] = useState<string | null>(null);

  return (
    <section className="ads-component-demo" aria-label="文件夹 AwakenFolder 组件演示">
      <header className="ads-component-demo-header">
        <h2>文件夹 <code>AwakenFolder</code></h2>
        <p>Skills 与记忆共用的中性文件夹原子组件，只表达名称、辅助信息和打开状态。</p>
      </header>
      <div className="aw-folder-demo__grid">
        {AwakenFolderStates.map((state) => (
          <div className="aw-folder-demo__state" key={state.id}>
            <span>{state.label}</span>
            <AwakenFolder
              {...state.props}
              open={state.id === "open" || openId === state.id}
              onOpen={() => setOpenId((current) => current === state.id ? null : state.id)}
            />
          </div>
        ))}
      </div>
    </section>
  );
}
