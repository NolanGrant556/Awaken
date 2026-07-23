import { useState } from "react";
import { AwakenConfiguredModels } from "../AwakenConfiguredModels";
import { AwakenModelProviderForm } from "../AwakenModelProviderForm";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenSettingsProviderCard } from "../AwakenSettingsProviderCard";
import { AwakenModelSettingsTabs } from "../AwakenSettingsTabs";
import { AwakenModelSettings } from "./AwakenModelSettings";

const configuredModel = {
  id: "deepseek",
  displayName: "DeepSeek",
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
  const [activeTabId, setActiveTabId] = useState("configure");
  const [modelId, setModelId] = useState("deepseek-v4-pro");
  const [baseURL, setBaseURL] = useState("https://api.deepseek.com");
  const [apiKey, setApiKey] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);

  return (
    <section className="ads-component-demo" aria-label="模型设置 AwakenModelSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>模型设置 <code>AwakenModelSettings</code></h2>
        <p>模型配置与已配置模型两个现有页签，共用新的全窗口设置框架。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="models" sectionTitle="模型">
        <AwakenModelSettings
          tabs={AwakenModelSettingsTabs}
          activeTabId={activeTabId}
          onSelectTab={setActiveTabId}
        >
          {activeTabId === "configure" ? (
            <section className="model-provider-list" aria-label="模型配置">
              <AwakenSettingsProviderCard name="DeepSeek" defaultExpanded>
                <AwakenModelProviderForm
                  providerId="deepseek-demo"
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
                />
              </AwakenSettingsProviderCard>
            </section>
          ) : (
            <AwakenConfiguredModels models={[configuredModel]} />
          )}
        </AwakenModelSettings>
      </AwakenSettingsSectionPreview>
    </section>
  );
}
