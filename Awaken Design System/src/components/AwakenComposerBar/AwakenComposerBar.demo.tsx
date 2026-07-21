import "./AwakenComposerBar.css";
import { useMemo, useState } from "react";
import type { ComponentProps, ComponentType } from "react";
import { AwakenAssistantMessage } from "../AwakenAssistantMessage/AwakenAssistantMessage";
import { AwakenComposerDock } from "../AwakenComposerDock/AwakenComposerDock";
import { AwakenExecutionFeed } from "../AwakenExecutionFeed/AwakenExecutionFeed";
import { AwakenRunStage } from "../AwakenRunStage/AwakenRunStage";
import { AwakenUserPromptBlock } from "../AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenUserPromptBlockStates } from "../AwakenUserPromptBlock/AwakenUserPromptBlock.states";
import { AwakenComposerBar } from "./AwakenComposerBar";
import { AwakenComposerBarStates } from "./AwakenComposerBar.states";

type ComposerDemoStateProps = ComponentProps<typeof AwakenComposerBar>;

function InteractiveComposerDemoState(props: ComposerDemoStateProps) {
  const modelItem = props.metaItems?.find((item) => item.id === "model");
  const initialModelId =
    modelItem?.menuItems?.find((item) => item.selected)?.id ??
    modelItem?.menuItems?.[0]?.id ??
    null;
  const [selectedModelId, setSelectedModelId] = useState(initialModelId);
  const interactiveMetaItems = useMemo(
    () =>
      props.metaItems?.map((item) => {
        if (item.id !== "model" || !item.menuItems?.length) {
          return item;
        }

        const selectedModel =
          item.menuItems.find((option) => option.id === selectedModelId) ??
          item.menuItems[0];
        return {
          ...item,
          label: selectedModel.label,
          menuItems: item.menuItems.map((option) => ({
            ...option,
            selected: option.id === selectedModel.id,
            onSelect: () => {
              option.onSelect?.();
              setSelectedModelId(option.id);
            },
          })),
        };
      }),
    [props.metaItems, selectedModelId],
  );

  return <AwakenComposerBar {...props} metaItems={interactiveMetaItems} />;
}

export function AwakenComposerBarDemo() {
  const PromptComponent = AwakenUserPromptBlock as unknown as ComponentType<Record<string, unknown>>;
  const composerDefaultProps = AwakenComposerBarStates[0]?.props ?? {};
  const promptDefaultProps = AwakenUserPromptBlockStates[0]?.props ?? {};
  const dockComposerProps = {
    ...composerDefaultProps,
    ...AwakenComposerBarStates.find((state) => state.id === "dock-empty")?.props,
  } as ComposerDemoStateProps;

  return (
    <section className="ads-component-demo" aria-label="输入栏 AwakenComposerBar 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          输入栏 <code>AwakenComposerBar + AwakenComposerDock</code>
        </h2>
        <p>
          先用真实 Workbench 缩略场景说明底部停靠关系，再集中观测输入状态和提交后由
          <code>AwakenUserPromptBlock</code> 展示的用户提示。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state ads-component-demo-state--workbench-scene">
          <div className="ads-component-demo-state-heading">
            <div>
              <span className="ads-component-demo-state-eyebrow">真实位置</span>
              <h3>输入栏固定在工作台底部</h3>
            </div>
            <p>
              上方叙事会滚动；蓝色边界内的 <code>AwakenComposerDock</code>
              只负责把输入栏稳定停靠在底部。
            </p>
          </div>
          <div className="ads-component-demo-canvas">
            <div className="ads-workbench-scene ads-workbench-scene--composer">
              <span className="ads-workbench-scene-label ads-workbench-scene-label--stage">
                <b>1</b> AwakenRunStage · 整个中间工作区
              </span>
              <AwakenRunStage
                composer={(
                  <AwakenComposerDock>
                    <div className="ads-workbench-scene__dock-stack">
                      <span className="ads-workbench-scene-label ads-workbench-scene-label--focus">
                        <b>3</b> AwakenComposerDock · 固定底部容器
                      </span>
                      <InteractiveComposerDemoState {...dockComposerProps} />
                    </div>
                  </AwakenComposerDock>
                )}
              >
                <div className="ads-workbench-scene__narrative-context">
                  <span className="ads-workbench-scene-label">
                    <b>2</b> AwakenExecutionFeed · 上方滚动叙事
                  </span>
                  <AwakenExecutionFeed label="输入栏位置上下文">
                    <AwakenUserPromptBlock
                      text="请检查设计系统里的中间交互区组件。"
                      tokens={[{ id: "skill:design-review", type: "skill", label: "design-review" }]}
                    />
                    <AwakenAssistantMessage
                      markdown="已完成组件检查。你可以继续在下方输入栏追加任务。"
                      status="done"
                      elapsedMs={18000}
                    />
                  </AwakenExecutionFeed>
                </div>
              </AwakenRunStage>
            </div>
          </div>
        </article>
        {AwakenComposerBarStates.map((state) => (
          <article key={`composer:${state.id}`} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <InteractiveComposerDemoState
                {...({
                  ...composerDefaultProps,
                  ...state.props,
                } as ComposerDemoStateProps)}
              />
            </div>
          </article>
        ))}
        {AwakenUserPromptBlockStates.map((state) => (
          <article key={`prompt:${state.id}`} className="ads-component-demo-state">
            <h3>用户提示块 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <PromptComponent
                {...(promptDefaultProps as Record<string, unknown>)}
                {...(state.props as Record<string, unknown>)}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
