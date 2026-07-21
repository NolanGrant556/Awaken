import "./AwakenComposerBar.css";
import type { ComponentType } from "react";
import { AwakenUserPromptBlock } from "../AwakenUserPromptBlock/AwakenUserPromptBlock";
import { AwakenUserPromptBlockStates } from "../AwakenUserPromptBlock/AwakenUserPromptBlock.states";
import { AwakenComposerBar } from "./AwakenComposerBar";
import { AwakenComposerBarStates } from "./AwakenComposerBar.states";

export function AwakenComposerBarDemo() {
  const ComposerComponent = AwakenComposerBar as unknown as ComponentType<Record<string, unknown>>;
  const PromptComponent = AwakenUserPromptBlock as unknown as ComponentType<Record<string, unknown>>;
  const composerDefaultProps = AwakenComposerBarStates[0]?.props ?? {};
  const promptDefaultProps = AwakenUserPromptBlockStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="输入栏 AwakenComposerBar 组件演示">
      <header className="ads-component-demo-header">
        <h2>输入栏 <code>AwakenComposerBar</code></h2>
        <p>
          集中观测中心入口、底部停靠输入，以及提交后由 <code>AwakenUserPromptBlock</code>
          展示的用户提示。Desktop 中输入栏仍属于 Composer，用户提示仍属于对应 Turn。
        </p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerBarStates.map((state) => (
          <article key={`composer:${state.id}`} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <ComposerComponent
                {...(composerDefaultProps as Record<string, unknown>)}
                {...(state.props as Record<string, unknown>)}
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
