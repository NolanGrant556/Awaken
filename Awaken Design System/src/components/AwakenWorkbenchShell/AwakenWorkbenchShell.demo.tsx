import "./AwakenWorkbenchShell.css";
import "./AwakenWorkbenchShell.demo.css";
import { Folder, Plug, Wrench } from "lucide-react";
import { AwakenAppShell } from "../AwakenAppShell/AwakenAppShell";
import { AwakenAppShellDemo } from "../AwakenAppShell/AwakenAppShell.demo";
import { AwakenComposerBar } from "../AwakenComposerBar/AwakenComposerBar";
import { AwakenInspectorRail } from "../AwakenInspectorRail/AwakenInspectorRail";
import { AwakenInspectorRailDemo } from "../AwakenInspectorRail/AwakenInspectorRail.demo";
import { AwakenTaskWelcome } from "../AwakenTaskWelcome/AwakenTaskWelcome";
import { AwakenWorkbenchChrome } from "../AwakenWorkbenchChrome/AwakenWorkbenchChrome";
import { AwakenWorkbenchChromeDemo } from "../AwakenWorkbenchChrome/AwakenWorkbenchChrome.demo";
import { AwakenWorkbenchShell } from "./AwakenWorkbenchShell";
import { AwakenWorkbenchShellStates } from "./AwakenWorkbenchShell.states";

const noop = () => {};

const anatomyLegend = [
  { number: "1", name: "AwakenAppShell", detail: "整窗外壳" },
  { number: "2", name: "AwakenWorkbenchChrome", detail: "顶部控制" },
  { number: "3", name: "AwakenWorkbenchShell", detail: "舞台与右栏布局" },
  { number: "4", name: "AwakenInspectorRail", detail: "右侧栏内容" },
] as const;

function AnatomyMarker({ number, name }: { number: string; name: string }) {
  return (
    <span className={`aw-workbench-anatomy__marker aw-workbench-anatomy__marker--${number}`} aria-hidden="true">
      <strong>{number}</strong>
      <code>{name}</code>
    </span>
  );
}

function WorkbenchAnatomySidebar() {
  return (
    <div className="aw-workbench-anatomy__sidebar" aria-label="Awaken 左侧栏示意">
      <button type="button" className="aw-workbench-anatomy__new-task">＋ New Agent</button>
      <div className="aw-workbench-anatomy__sidebar-links">
        <span><Plug size={12} aria-hidden="true" />连接器</span>
        <span><Wrench size={12} aria-hidden="true" />技能</span>
      </div>
      <div className="aw-workbench-anatomy__projects">
        <strong>项目工作区</strong>
        <span><Folder size={12} aria-hidden="true" />04_市场部</span>
        <span><Folder size={12} aria-hidden="true" />01_财务部</span>
      </div>
      <div className="aw-workbench-anatomy__profile"><b>AW</b><span>Nolan</span></div>
    </div>
  );
}

function WorkbenchAnatomyStage() {
  return (
    <div className="aw-workbench-anatomy__stage">
      <span className="aw-workbench-anatomy__slot-label">stage：中间交互区组件放在这里</span>
      <div className="aw-workbench-anatomy__center">
        <AwakenTaskWelcome />
        <AwakenComposerBar
          value=""
          onChange={noop}
          placeholder="输入任务"
          disabled
          variant="center"
          metaItems={[{ id: "model", label: "kimi-for-coding" }]}
        />
      </div>
    </div>
  );
}

function WorkbenchAnatomyScene() {
  return (
    <article className="aw-workbench-anatomy">
      <div className="aw-workbench-anatomy__intro">
        <div>
          <span className="aw-workbench-anatomy__eyebrow">真实组合关系</span>
          <h3>这四个组件共同组成当前 Awaken Desktop 工作台</h3>
          <p>编号和同色边界直接标出每个组件在真实窗口中的位置；内部内容使用当前组件实际组合。</p>
        </div>
        <div className="aw-workbench-anatomy__legend" aria-label="组件位置图例">
          {anatomyLegend.map((item) => (
            <span key={item.number} className={`aw-workbench-anatomy__legend-item aw-workbench-anatomy__legend-item--${item.number}`}>
              <strong>{item.number}</strong>
              <span><code>{item.name}</code><small>{item.detail}</small></span>
            </span>
          ))}
        </div>
      </div>

      <div className="aw-workbench-anatomy__viewport">
        <div className="aw-workbench-anatomy__component aw-workbench-anatomy__component--app-shell">
          <AnatomyMarker number="1" name="AwakenAppShell" />
          <AwakenAppShell
            platform="macos"
            sidebar={<WorkbenchAnatomySidebar />}
            chrome={
              <div className="aw-workbench-anatomy__component aw-workbench-anatomy__component--chrome">
                <AnatomyMarker number="2" name="AwakenWorkbenchChrome" />
                <AwakenWorkbenchChrome
                  platform="macos"
                  onToggleSidebar={noop}
                  onNewTask={noop}
                  right="Awaken Desktop"
                />
              </div>
            }
            main={
              <div className="aw-workbench-anatomy__component aw-workbench-anatomy__component--workbench">
                <AnatomyMarker number="3" name="AwakenWorkbenchShell" />
                <AwakenWorkbenchShell
                  stage={<WorkbenchAnatomyStage />}
                  inspector={
                    <div className="aw-workbench-anatomy__component aw-workbench-anatomy__component--inspector">
                      <AnatomyMarker number="4" name="AwakenInspectorRail" />
                      <AwakenInspectorRail
                        ariaLabel="文件"
                        items={[
                          {
                            id: "files-placeholder",
                            label: "文件",
                            disabled: true,
                            icon: (
                              <Folder size={16} strokeWidth={1.8} aria-hidden="true" />
                            ),
                          },
                        ]}
                        activeId="files-placeholder"
                      />
                    </div>
                  }
                />
              </div>
            }
          />
        </div>
      </div>

      <div className="aw-workbench-anatomy__flow" aria-label="工作台组件嵌套顺序">
        <span className="aw-workbench-anatomy__flow-node aw-workbench-anatomy__flow-node--1">1 AppShell</span>
        <span className="aw-workbench-anatomy__flow-branches">
          <span>chrome 插槽</span>
          <span>main 插槽</span>
        </span>
        <span className="aw-workbench-anatomy__flow-stack">
          <span className="aw-workbench-anatomy__flow-node aw-workbench-anatomy__flow-node--2">2 WorkbenchChrome</span>
          <span className="aw-workbench-anatomy__flow-node aw-workbench-anatomy__flow-node--3">3 WorkbenchShell</span>
        </span>
        <span className="aw-workbench-anatomy__flow-arrow">→ inspector 插槽 →</span>
        <span className="aw-workbench-anatomy__flow-node aw-workbench-anatomy__flow-node--4">4 InspectorRail</span>
      </div>
    </article>
  );
}

export function AwakenWorkbenchShellDemo() {
  return (
    <section className="ads-component-demo aw-workbench-framework-demo" aria-label="工作台框架组件库演示">
      <header className="ads-component-demo-header">
        <h2>「工作台框架」组件库 <code>AwakenWorkbenchShell</code></h2>
        <p>先通过完整 Desktop 缩略图理解四个框架组件的嵌套位置，再向下检查各组件的独立状态。</p>
      </header>

      <WorkbenchAnatomyScene />

      <div className="aw-workbench-anatomy__details-heading">
        <span>独立状态</span>
        <p>以下状态用于检查单个组件的布局边界，不代表四套互相独立的 Desktop UI。</p>
      </div>

      <AwakenAppShellDemo />
      <AwakenWorkbenchChromeDemo />

      <section className="ads-component-demo aw-framework-subdemo" aria-label="工作台外壳 AwakenWorkbenchShell 组件演示">
        <header className="ads-component-demo-header">
          <h2>工作台外壳 <code>AwakenWorkbenchShell</code></h2>
          <p>它不是一张可见卡片，而是 Awaken 窗口中部的两列布局：左边放运行舞台，右边放 Inspector，并根据折叠状态重新分配两边宽度。</p>
        </header>
        <div className="ads-component-demo-grid">
          {AwakenWorkbenchShellStates.map((state) => (
            <article key={state.id} className="ads-component-demo-state">
              <div className="ads-component-demo-state-heading">
                <div>
                  <span className="ads-component-demo-state-eyebrow">布局状态</span>
                  <h3>{state.label}</h3>
                </div>
                <p>{state.description}</p>
              </div>
              <div className="ads-component-demo-canvas">
                <div className="aw-framework-preview aw-framework-preview--shell">
                  <AwakenWorkbenchShell
                    {...state.props}
                    stage={
                      <div className="aw-framework-preview__stage">
                        <strong>{state.props.stage}</strong>
                        <span>执行叙事、空任务欢迎区和 Composer 由 Desktop 放入此区域</span>
                      </div>
                    }
                  />
                </div>
              </div>
            </article>
          ))}
        </div>
      </section>

      <AwakenInspectorRailDemo />
    </section>
  );
}
