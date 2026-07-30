import type { ComponentType } from "react";
import { AwakenMediaGenerationCard } from "./AwakenMediaGenerationCard";
import { AwakenMediaGenerationCardStates } from "./AwakenMediaGenerationCard.states";
import { AwakenMediaPreviewOverlay } from "../AwakenMediaPreviewOverlay";

export function AwakenMediaGenerationCardDemo() {
  const Component = AwakenMediaGenerationCard as unknown as ComponentType<Record<string, unknown>>;
  return (
    <section className="ads-component-demo" aria-label="媒体生成卡片演示">
      <header className="ads-component-demo-header">
        <h2>媒体生成卡片 <code>AwakenMediaGenerationCard</code></h2>
        <p>每张图片或每个视频都是独立紧凑卡片，并按媒体真实宽高比展示。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenMediaGenerationCardStates.map((state) => (
          <article className="ads-component-demo-state" key={state.id}>
            <h3>{state.name}</h3>
            <p>{state.description}</p>
            <div className="ads-component-demo-canvas">
              <Component {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
        <article className="ads-component-demo-state">
          <h3>图片放大预览</h3>
          <p>单击独立媒体卡片后，在 RunStage overlay 内查看，并可调用本地系统操作。</p>
          <div
            className="ads-component-demo-canvas"
            style={{ position: "relative", height: 560, overflow: "hidden" }}
          >
            <AwakenMediaPreviewOverlay
              item={{
                id: "preview-image",
                kind: "image",
                title: "生成的主视觉.png",
                src: "data:image/svg+xml;charset=utf-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='900' height='1200'%3E%3Cdefs%3E%3ClinearGradient id='g' x1='0' y1='0' x2='1' y2='1'%3E%3Cstop stop-color='%23c9d8ff'/%3E%3Cstop offset='1' stop-color='%232b1f55'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect width='900' height='1200' fill='url(%23g)'/%3E%3C/svg%3E",
              }}
              position={0}
              count={1}
              onClose={() => undefined}
              onOpenExternal={() => undefined}
              onReveal={() => undefined}
              revealLabel="在 Finder 中显示"
            />
          </div>
        </article>
      </div>
    </section>
  );
}
