import { useState } from "react";
import { AwakenSettingsProviderCard, AwakenSettingsProviderModal } from "./AwakenSettingsProviderCard";

export function AwakenSettingsProviderCardDemo() {
  const [modalOpen, setModalOpen] = useState(false);

  return (
    <section className="ads-component-demo" aria-label="模型供应商卡 AwakenSettingsProviderCard 组件演示">
      <header className="ads-component-demo-header">
        <h2>模型供应商卡 <code>AwakenSettingsProviderCard</code></h2>
        <p>用官方产品 Logo、名称和轻量供应商色展示固定三列连接入口；编辑与连接状态在居中弹窗完成。</p>
      </header>
      <div className="model-provider-list">
        <AwakenSettingsProviderCard name="DeepSeek" brand="deepseek" selected={modalOpen} onSelect={() => setModalOpen(true)} />
        <AwakenSettingsProviderCard name="MiniMax" brand="minimax" onSelect={() => undefined} />
        <AwakenSettingsProviderCard name="GLM" brand="glm" onSelect={() => undefined} />
      </div>
      <AwakenSettingsProviderModal open={modalOpen} name="DeepSeek" brand="deepseek" status="connected" caption="已配置 deepseek-v4-pro" onClose={() => setModalOpen(false)}>
        <p style={{ margin: 0, color: "var(--aw-text-secondary)", fontSize: 12 }}>供应商表单由 consumer 组合。</p>
      </AwakenSettingsProviderModal>
    </section>
  );
}
