import "./AwakenRunPlan.css";
import { useState } from "react";
import { AwakenAssistantMessage } from "../AwakenAssistantMessage/AwakenAssistantMessage";
import { AwakenComposerBar } from "../AwakenComposerBar/AwakenComposerBar";
import { AwakenComposerDock } from "../AwakenComposerDock/AwakenComposerDock";
import { AwakenExecutionFeed } from "../AwakenExecutionFeed/AwakenExecutionFeed";
import { AwakenThoughtTrace } from "../AwakenThoughtTrace/AwakenThoughtTrace";
import { AwakenToolTraceLine } from "../AwakenToolTraceLine/AwakenToolTraceLine";
import { AwakenUserPromptBlock } from "../AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenRunStage } from "../AwakenRunStage/AwakenRunStage";
import {
  AwakenRunPlanHistorical,
  AwakenRunPlanPanel,
  AwakenRunPlanTrigger,
} from "./AwakenRunPlan";
import type { AwakenRunPlanItemView } from "./AwakenRunPlan";
import { AwakenRunPlanStates } from "./AwakenRunPlan.states";

const noop = () => {};

function deriveCounts(items: AwakenRunPlanItemView[]) {
  const completedCount = items.filter(
    (item) => item.status === "completed",
  ).length;
  const totalCount = items.length;
  const allCompleted = totalCount > 0 && completedCount === totalCount;
  return {
    completedCount,
    totalCount,
    allCompleted,
    currentStep: allCompleted ? totalCount : completedCount + 1,
  };
}

const sceneState = AwakenRunPlanStates[0];

export function AwakenRunPlanDemo() {
  const [open, setOpen] = useState(true);
  const sceneCounts = deriveCounts(sceneState.items);

  return (
    <section className="ads-component-demo" aria-label="运行计划 AwakenRunPlan 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          运行计划 <code>AwakenRunPlan</code>
        </h2>
        <p>
          UI-only 的运行计划/任务进度组件族：ComposerDock stack 内的紧凑入口、RunStage.overlay
          内的只读展开面板，以及执行过程内的历史快照。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state ads-component-demo-state--workbench-scene">
          <div className="ads-component-demo-state-heading">
            <div>
              <span className="ads-component-demo-state-eyebrow">真实位置</span>
              <h3>进度入口与展开面板在工作台中的层次</h3>
            </div>
            <p>
              入口位于 ComposerDock stack、ComposerBar 上方；展开面板位于 RunStage.overlay
              浮层，距 composer inset 12px 的定位由调用方负责。
            </p>
          </div>
          <div className="ads-component-demo-canvas">
            <div className="ads-workbench-scene ads-workbench-scene--feed">
              <span className="ads-workbench-scene-label ads-workbench-scene-label--stage">
                <b>1</b> AwakenRunStage · 唯一滚动与停靠容器
              </span>
              <AwakenRunStage
                overlay={open ? (
                  <div
                    style={{
                      position: "absolute",
                      inset: "0 0 162px 0",
                      padding: "0 32px",
                      display: "flex",
                    }}
                  >
                    <AwakenRunPlanPanel
                      items={sceneState.items}
                      explanation={sceneState.explanation}
                      onClose={() => setOpen(false)}
                    />
                  </div>
                ) : undefined}
                composer={(
                  <AwakenComposerDock>
                    <div className="ads-workbench-scene__dock-stack">
                      <span className="ads-workbench-scene-label">
                        <b>3</b> ComposerDock · feed 外的输入区
                      </span>
                      <AwakenRunPlanTrigger
                        items={sceneState.items}
                        {...sceneCounts}
                        open={open}
                        onToggle={() => setOpen((value) => !value)}
                      />
                      <AwakenComposerBar
                        value=""
                        onChange={noop}
                        onSubmit={noop}
                        onAddContext={noop}
                        placeholder="继续输入任务..."
                        variant="dock"
                        metaItems={[{ id: "status", label: "Awaken" }]}
                      />
                    </div>
                  </AwakenComposerDock>
                )}
              >
                <div className="ads-workbench-scene__feed-focus">
                  <span className="ads-workbench-scene-label ads-workbench-scene-label--focus">
                    <b>2</b> AwakenExecutionFeed · 本轮执行叙事
                  </span>
                  <AwakenExecutionFeed label="运行计划场景叙事示例">
                    <section className="ads-workbench-scene__turn" aria-label="本轮任务">
                      <div className="ads-workbench-scene__turn-header">
                        <span className="ads-workbench-scene__part-label">用户提示 · Turn header</span>
                        <AwakenUserPromptBlock text="为这次复杂任务建立运行计划，并实时更新进度。" />
                      </div>
                      <div className="ads-workbench-scene__turn-body">
                        <span className="ads-workbench-scene__part-label">执行过程 · Turn body</span>
                        <AwakenThoughtTrace
                          label="思考"
                          durationLabel="6s"
                          status="done"
                          text="任务包含调查、实现和验收三个可区分步骤，建立运行计划。"
                        />
                        <AwakenToolTraceLine
                          title="读取运行计划设计契约"
                          detail="已确认入口、面板与历史快照的 ownership。"
                          status="done"
                        />
                        <AwakenAssistantMessage
                          markdown="计划已建立，进度入口显示在输入区上方。"
                          status="active"
                        />
                      </div>
                    </section>
                  </AwakenExecutionFeed>
                </div>
              </AwakenRunStage>
            </div>
          </div>
        </article>
        {AwakenRunPlanStates.map((state) => {
          const counts = deriveCounts(state.items);
          return (
            <article key={state.id} className="ads-component-demo-state">
              <h3>{state.label}</h3>
              <div className="ads-component-demo-canvas" style={{ minHeight: 320 }}>
                <div
                  style={{
                    display: "grid",
                    gap: 16,
                    justifyItems: "start",
                    padding: 16,
                  }}
                >
                  <AwakenRunPlanTrigger
                    items={state.items}
                    {...counts}
                    open={false}
                    onToggle={noop}
                  />
                  <div style={{ height: 340, width: "min(640px, 100%)", display: "flex" }}>
                    <AwakenRunPlanPanel
                      items={state.items}
                      explanation={state.explanation}
                      onClose={noop}
                    />
                  </div>
                  <AwakenRunPlanHistorical
                    items={state.items}
                    completedCount={counts.completedCount}
                    totalCount={counts.totalCount}
                  />
                </div>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
