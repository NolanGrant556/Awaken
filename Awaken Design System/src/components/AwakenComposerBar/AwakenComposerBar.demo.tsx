import "./AwakenComposerBar.css";
import { useMemo, useRef, useState } from "react";
import type { ComponentProps, ComponentType } from "react";
import { AwakenAssistantMessage } from "../AwakenAssistantMessage/AwakenAssistantMessage";
import { AwakenComposerDock } from "../AwakenComposerDock/AwakenComposerDock";
import { AwakenComposerInput } from "../AwakenComposerInput/AwakenComposerInput";
import { AwakenComposerInputStates } from "../AwakenComposerInput/AwakenComposerInput.states";
import { AwakenContextButton } from "../AwakenContextButton/AwakenContextButton";
import { AwakenContextButtonStates } from "../AwakenContextButton/AwakenContextButton.states";
import { AwakenContextUsagePanel } from "../AwakenContextUsagePanel/AwakenContextUsagePanel";
import { AwakenContextUsagePanelStates } from "../AwakenContextUsagePanel/AwakenContextUsagePanel.states";
import { AwakenExecutionFeed } from "../AwakenExecutionFeed/AwakenExecutionFeed";
import { AwakenRunStage } from "../AwakenRunStage/AwakenRunStage";
import { AwakenUserPromptBlock } from "../AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenUserPromptBlockStates } from "../AwakenUserPromptBlock/AwakenUserPromptBlock.states";
import {
  AwakenComposerBar,
  type ComposerBarMetaItem,
} from "./AwakenComposerBar";
import { AwakenComposerBarStates } from "./AwakenComposerBar.states";

type ComposerDemoStateProps = ComponentProps<typeof AwakenComposerBar>;

const contextUsageDemoProps = AwakenContextUsagePanelStates.find(
  (state) => state.id === "zero-middle-category",
)?.props as ComponentProps<typeof AwakenContextUsagePanel>;

function InteractiveComposerDemoState(props: ComposerDemoStateProps) {
  const modelItem = props.metaItems?.find((item) => item.id === "model");
  const initialModelId =
    modelItem?.menuItems?.find((item) => item.selected)?.id ??
    modelItem?.menuItems?.[0]?.id ??
    null;
  const [selectedModelId, setSelectedModelId] = useState(initialModelId);
  const [effortByModelId, setEffortByModelId] = useState<
    Record<string, "low" | "high" | "max">
  >(() =>
    Object.fromEntries(
      modelItem?.menuItems?.flatMap((option) =>
        option.effort ? [[option.id, option.effort.selectedLevel]] : [],
      ) ?? [],
    ),
  );
  const initialProjectId =
    props.projectPicker?.options.find((option) => option.selected)?.id ?? null;
  const [selectedProjectId, setSelectedProjectId] = useState(initialProjectId);
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
          label: selectedModel.effort
            ? `${selectedModel.label} · ${effortByModelId[selectedModel.id] ?? selectedModel.effort.selectedLevel}`
            : selectedModel.label,
          menuItems: item.menuItems.map((option) => ({
            ...option,
            selected: option.id === selectedModel.id,
            onSelect: () => {
              option.onSelect?.();
              setSelectedModelId(option.id);
            },
            effort: option.effort
              ? {
                  ...option.effort,
                  selectedLevel:
                    effortByModelId[option.id] ?? option.effort.selectedLevel,
                  onChange: (level: "low" | "high" | "max") => {
                    option.effort?.onChange(level);
                    setSelectedModelId(option.id);
                    setEffortByModelId((current) => ({
                      ...current,
                      [option.id]: level,
                    }));
                  },
                }
              : undefined,
          })),
        };
      }),
    [effortByModelId, props.metaItems, selectedModelId],
  );
  const interactiveProjectPicker = useMemo(() => {
    if (!props.projectPicker) {
      return undefined;
    }
    const globalProjectOptionId =
      props.projectPicker.options.find((option) => option.icon === null)?.id ??
      null;
    return {
      ...props.projectPicker,
      options: props.projectPicker.options.map((option) => ({
        ...option,
        selected: option.id === selectedProjectId,
        onSelect: () => {
          option.onSelect?.();
          setSelectedProjectId(option.id);
        },
      })),
      clearAction: props.projectPicker.clearAction
        ? {
            ...props.projectPicker.clearAction,
            onClick: () => {
              props.projectPicker?.clearAction?.onClick();
              setSelectedProjectId(globalProjectOptionId);
            },
          }
        : undefined,
    };
  }, [props.projectPicker, selectedProjectId]);

  return (
    <AwakenComposerBar
      {...props}
      metaItems={interactiveMetaItems}
      projectPicker={interactiveProjectPicker}
    />
  );
}

function WorkbenchComposerScene({
  composerProps,
}: {
  composerProps: ComposerDemoStateProps;
}) {
  const contextUsagePanelId = "awaken-composer-demo-context-usage";
  const contextTriggerRef = useRef<HTMLButtonElement>(null);
  const [contextUsagePanelOpen, setContextUsagePanelOpen] = useState(false);
  const sceneMetaItems = useMemo<ComposerBarMetaItem[]>(
    () => [
      ...(composerProps.metaItems?.filter((item) => item.id !== "context") ?? []),
      {
        id: "context",
        label: "上下文窗口",
        value: "92K / 240K",
        progress: 92_100 / 240_000,
        onClick: () => setContextUsagePanelOpen((current) => !current),
        buttonRef: contextTriggerRef,
        ariaHaspopup: "dialog",
        ariaExpanded: contextUsagePanelOpen,
        ariaControls: contextUsagePanelId,
      },
    ],
    [composerProps.metaItems, contextUsagePanelOpen],
  );
  const closeContextUsagePanel = () => {
    setContextUsagePanelOpen(false);
    contextTriggerRef.current?.focus();
  };

  return (
    <div className="ads-workbench-scene ads-workbench-scene--composer">
      <AwakenRunStage
        overlay={
          contextUsagePanelOpen ? (
            <div className="ads-workbench-scene__context-overlay">
              <AwakenContextUsagePanel
                {...contextUsageDemoProps}
                id={contextUsagePanelId}
                onClose={closeContextUsagePanel}
              />
            </div>
          ) : null
        }
        composer={
          <AwakenComposerDock>
            <div className="ads-workbench-scene__dock-stack">
              <span className="ads-workbench-scene-label ads-workbench-scene-label--focus">
                固定底部输入栏 · 运行中可继续追加任务
              </span>
              <InteractiveComposerDemoState
                {...composerProps}
                metaItems={sceneMetaItems}
              />
            </div>
          </AwakenComposerDock>
        }
      >
        <div className="ads-workbench-scene__narrative-context">
          <span className="ads-workbench-scene-label">
            历史消息与最终输出 · 仅此区域滚动
          </span>
          <AwakenExecutionFeed label="运行中的历史消息与最终输出">
            <AwakenUserPromptBlock
              text="请检查设计系统里的中间交互区组件。"
              tokens={[
                {
                  id: "skill:design-review",
                  type: "skill",
                  label: "design-review",
                },
              ]}
            />
            <AwakenAssistantMessage
              markdown="已完成组件检查：输入框已归入输入栏组件族，上下文用量可从下方入口打开。"
              status="done"
              elapsedMs={18000}
            />
          </AwakenExecutionFeed>
        </div>
      </AwakenRunStage>
    </div>
  );
}

export function AwakenComposerBarDemo() {
  const PromptComponent = AwakenUserPromptBlock as unknown as ComponentType<
    Record<string, unknown>
  >;
  const composerDefaultProps = AwakenComposerBarStates[0]?.props ?? {};
  const inputDefaultProps = AwakenComposerInputStates[0]?.props ?? {};
  const contextButtonDefaultProps = AwakenContextButtonStates[0]?.props ?? {};
  const promptDefaultProps = AwakenUserPromptBlockStates[0]?.props ?? {};
  const dockComposerProps = {
    ...composerDefaultProps,
    ...AwakenComposerBarStates.find((state) => state.id === "dock-empty")?.props,
  } as ComposerDemoStateProps;

  return (
    <section className="ads-component-demo" aria-label="输入栏 AwakenComposerBar 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          输入栏 <code>AwakenComposerBar + AwakenComposerDock + AwakenSendButton</code>
        </h2>
        <p>
          集中观测输入栏组合、底层输入框、添加上下文图标按钮、发送/停止按钮及其工具提示，以及提交后的用户提示。center 空态还展示与侧栏同源的项目低栏，可切换 mock 项目并查看创建入口；运行态中，历史消息与最终输出只在上方滚动，输入栏固定在最底部；有真实快照时可悬停上下文圆环查看用量摘要，或点击打开用量浮层。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state ads-component-demo-state--workbench-scene">
          <div className="ads-component-demo-state-heading">
            <div>
              <span className="ads-component-demo-state-eyebrow">运行态真实位置</span>
              <h3>输出在上方滚动，输入栏固定在最底部</h3>
            </div>
            <p>
              点击输入栏底部的上下文圆环后，
              <code>AwakenContextUsagePanel</code> 通过 <code>RunStage.overlay</code>
              出现在输入栏上方，不进入消息流，也不会改变上下顺序。
            </p>
          </div>
          <div className="ads-component-demo-canvas">
            <WorkbenchComposerScene composerProps={dockComposerProps} />
          </div>
        </article>
        {AwakenComposerBarStates.map((state) => (
          <article key={`composer:${state.id}`} className="ads-component-demo-state">
            <h3>完整输入栏 · {state.label}</h3>
            {state.id === "running" ? (
              <p>运行中的停止按钮由 <code>AwakenSendButton</code> 提供，并继续允许取消当前运行。</p>
            ) : state.id === "long-draft" ? (
              <p>可发送状态在完整输入栏内观测；悬停发送按钮可查看真实工具提示。</p>
            ) : null}
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
        {AwakenComposerInputStates.map((state) => (
          <article key={`input:${state.id}`} className="ads-component-demo-state">
            <h3>底层输入框 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenComposerInput
                {...({
                  ...inputDefaultProps,
                  ...state.props,
                } as ComponentProps<typeof AwakenComposerInput>)}
              />
            </div>
          </article>
        ))}
        {AwakenContextButtonStates.map((state) => (
          <article key={`context-button:${state.id}`} className="ads-component-demo-state">
            <h3>添加上下文按钮 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenContextButton
                {...({
                  ...contextButtonDefaultProps,
                  ...state.props,
                } as ComponentProps<typeof AwakenContextButton>)}
              />
            </div>
          </article>
        ))}
        {AwakenUserPromptBlockStates.map((state) => (
          <article key={`prompt:${state.id}`} className="ads-component-demo-state">
            <h3>提交后的用户提示 · {state.label}</h3>
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
