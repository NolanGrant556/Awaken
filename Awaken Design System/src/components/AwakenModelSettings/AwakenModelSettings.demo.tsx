import { useState } from "react";
import { AwakenConfiguredModels } from "../AwakenConfiguredModels";
import { AwakenModelProviderForm } from "../AwakenModelProviderForm";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenSettingsProviderCard, AwakenSettingsProviderModal } from "../AwakenSettingsProviderCard";
import { AwakenModelSettings } from "./AwakenModelSettings";

const accessTabs = [
  { id: "coding_plan", label: "Coding Plan" },
  { id: "api", label: "API" },
  { id: "agent_plan", label: "Agent Plan" },
];

const apiViews = [
  { id: "connections", label: "配置 API Key" },
  { id: "llm", label: "LLM 设置" },
];

const configuredModel = {
  id: "deepseek",
  displayName: "DeepSeek",
  brand: "deepseek" as const,
  modelId: "deepseek-v4-pro",
  enabled: true,
  contextWindow: 128_000,
  contextWindowInput: "128000",
  contextWindowMin: 8_000,
  contextWindowMax: 128_000,
  contextWindowStep: 1,
  contextFill: 100,
  contextAnchor: 56,
  contextAnchorValue: 76_800,
  contextAnchorLabel: "77K",
  contextEndLabel: "128K",
  compressionValue: 112_000,
  maxTokensInput: "8192",
};

export function AwakenModelSettingsDemo() {
  const [activeTabId, setActiveTabId] = useState("api");
  const [activeViewId, setActiveViewId] = useState("connections");
  const [modelId, setModelId] = useState("deepseek-v4-pro");
  const [baseURL, setBaseURL] = useState("https://api.deepseek.com");
  const [apiKey, setApiKey] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);
  const [providerOpen, setProviderOpen] = useState(false);

  return (
    <section className="ads-component-demo" aria-label="模型设置 AwakenModelSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>模型设置 <code>AwakenModelSettings</code></h2>
        <p>左侧区分接入类型，右侧独立管理连接与 LLM 设置。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="models" sectionTitle="模型">
        <AwakenModelSettings
          tabs={accessTabs}
          activeTabId={activeTabId}
          views={apiViews}
          activeViewId={activeViewId}
          title="API"
          description="管理供应商 API Key 与按量调用的模型。"
          onSelectTab={setActiveTabId}
          onSelectView={setActiveViewId}
        >
          {activeViewId === "connections" ? (
            <>
              <section className="model-provider-list" aria-label="模型配置">
                <AwakenSettingsProviderCard
                  name="DeepSeek"
                  brand="deepseek"
                  selected={providerOpen}
                  onSelect={() => setProviderOpen(true)}
                />
              </section>
              <AwakenSettingsProviderModal
                open={providerOpen}
                name="DeepSeek"
                brand="deepseek"
                status="connected"
                caption="已配置 deepseek-v4-pro"
                onClose={() => setProviderOpen(false)}
              >
                <AwakenModelProviderForm
                  providerId="deepseek-demo"
                  presentation="modal"
                  modelMode="select"
                  models={["deepseek-v4-pro", "deepseek-v4-flash"]}
                  modelId={modelId}
                  baseURL={baseURL}
                  apiKey={apiKey}
                  apiKeyPlaceholder="输入 DEEPSEEK_API_KEY"
                  apiDocsURL="https://platform.deepseek.com/api_keys"
                  showApiKey={showApiKey}
                  onModelChange={setModelId}
                  onBaseURLChange={setBaseURL}
                  onApiKeyChange={setApiKey}
                  onShowApiKeyChange={setShowApiKey}
                  onCancel={() => setProviderOpen(false)}
                />
              </AwakenSettingsProviderModal>
            </>
          ) : (
            <AwakenConfiguredModels models={[configuredModel]} />
          )}
        </AwakenModelSettings>
      </AwakenSettingsSectionPreview>
    </section>
  );
}
