import "./AwakenRunStage.css";
import type { ComponentType } from "react";
import { AwakenAssistantMessage } from "../AwakenAssistantMessage/AwakenAssistantMessage";
import { AwakenComposerBar } from "../AwakenComposerBar/AwakenComposerBar";
import { AwakenComposerDock } from "../AwakenComposerDock/AwakenComposerDock";
import { AwakenDeliverableList } from "../AwakenDeliverableList/AwakenDeliverableList";
import { AwakenExecutionFeed } from "../AwakenExecutionFeed/AwakenExecutionFeed";
import { AwakenThoughtTrace } from "../AwakenThoughtTrace/AwakenThoughtTrace";
import { AwakenToolTraceLine } from "../AwakenToolTraceLine/AwakenToolTraceLine";
import { AwakenUserPromptBlock } from "../AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenRunStage } from "./AwakenRunStage";
import { AwakenRunStageStates } from "./AwakenRunStage.states";

const noop = () => {};

export function AwakenRunStageDemo() {
  const Component = AwakenRunStage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenRunStageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="运行阶段 AwakenRunStage 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          运行阶段 <code>AwakenRunStage + AwakenExecutionFeed</code>
        </h2>
        <p>
          用真实运行叙事说明容器关系：RunStage 拥有滚动区和底部插槽，ExecutionFeed
          只承载用户提示、执行过程、最终回答和交付物。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state ads-component-demo-state--workbench-scene">
          <div className="ads-component-demo-state-heading">
            <div>
              <span className="ads-component-demo-state-eyebrow">真实位置</span>
              <h3>一次完整运行如何进入中间工作区</h3>
            </div>
            <p>
              蓝色边界是 <code>AwakenExecutionFeed</code>；底部输入属于独立的 ComposerDock，
              不会进入执行叙事。
            </p>
          </div>
          <div className="ads-component-demo-canvas">
            <div className="ads-workbench-scene ads-workbench-scene--feed">
              <span className="ads-workbench-scene-label ads-workbench-scene-label--stage">
                <b>1</b> AwakenRunStage · 唯一滚动与停靠容器
              </span>
              <AwakenRunStage
                composer={(
                  <AwakenComposerDock>
                    <div className="ads-workbench-scene__dock-stack">
                      <span className="ads-workbench-scene-label">
                        <b>3</b> ComposerDock · feed 外的输入区
                      </span>
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
                  <AwakenExecutionFeed label="完整运行叙事示例">
                    <section className="ads-workbench-scene__turn" aria-label="本轮任务">
                      <div className="ads-workbench-scene__turn-header">
                        <span className="ads-workbench-scene__part-label">用户提示 · Turn header</span>
                        <AwakenUserPromptBlock
                          text="请检查 Design System 中间区域的组件归属，并给出直观结果。"
                          tokens={[{ id: "skill:design-review", type: "skill", label: "design-review" }]}
                        />
                      </div>
                      <div className="ads-workbench-scene__turn-body">
                        <span className="ads-workbench-scene__part-label">执行过程 · Turn body</span>
                        <AwakenThoughtTrace
                          label="思考"
                          durationLabel="8s"
                          status="done"
                          text="先确认容器 ownership，再检查 Gallery 是否把结构组件展示成真实工作台位置。"
                        />
                        <AwakenToolTraceLine
                          title="读取中间区域架构契约"
                          detail="已确认 RunStage、ExecutionFeed 与 ComposerDock 的边界。"
                          status="done"
                        />
                        <AwakenAssistantMessage
                          markdown="**检查完成。** 执行叙事保持在中间滚动区，输入栏固定在底部。"
                          status="done"
                          elapsedMs={26000}
                        />
                        <AwakenDeliverableList
                          items={[
                            {
                              id: "workbench-review",
                              title: "中间区域复核结果.md",
                              extension: "md",
                              fileType: "markdown",
                              onOpen: noop,
                            },
                          ]}
                        />
                      </div>
                    </section>
                  </AwakenExecutionFeed>
                </div>
              </AwakenRunStage>
            </div>
          </div>
        </article>
        {AwakenRunStageStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <Component {...(defaultProps as Record<string, unknown>)} {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
