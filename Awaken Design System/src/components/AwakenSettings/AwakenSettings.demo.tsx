import "./AwakenSettings.demo.css";
import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenConfiguredModels } from "../AwakenConfiguredModels";
import { AwakenIconButton } from "../AwakenIconButton";
import { AwakenMemorySettings } from "../AwakenMemorySettings";
import { AwakenMemorySettingsSample } from "../AwakenMemorySettings/AwakenMemorySettings.states";
import { AwakenModelProviderForm } from "../AwakenModelProviderForm";
import { AwakenPetsSettings } from "../AwakenPetsSettings";
import { AwakenPetsSettingsSample } from "../AwakenPetsSettings/AwakenPetsSettings.states";
import { AwakenSearchSettings, type AwakenSearchProviderItem } from "../AwakenSearchSettings";
import { AwakenSearchSettingsSample } from "../AwakenSearchSettings/AwakenSearchSettings.states";
import { AwakenSettingsNav, AwakenSettingsNavItems } from "../AwakenSettingsNav";
import { AwakenSettingsProviderCard } from "../AwakenSettingsProviderCard";
import { AwakenModelSettingsTabs, AwakenSettingsTabs } from "../AwakenSettingsTabs";
import { AwakenAppearanceSettings, AwakenThemeChoiceGroup, type AwakenThemeChoiceValue } from "../AwakenThemeChoiceGroup";
import { AwakenSettings } from "./AwakenSettings";

const sectionTitles: Record<string, string> = { models: "模型", search: "搜索", memory: "记忆", appearance: "外观", pets: "宠物" };
type ProviderDemo = {
  id: string;
  name: string;
  models: string[];
  modelId: string;
  baseURL: string;
  keyEnvName: string;
  apiDocsURL: string;
  allowCustomModel?: boolean;
};

const providerSamples: ProviderDemo[] = [
  { id: "deepseek", name: "DeepSeek", models: ["deepseek-v4-pro", "deepseek-v4-flash"], modelId: "deepseek-v4-pro", baseURL: "https://api.deepseek.com", keyEnvName: "DEEPSEEK_API_KEY", apiDocsURL: "https://platform.deepseek.com/api_keys" },
  { id: "kimi-cn", name: "Kimi CN", models: ["kimi-k2.6", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.5", "moonshot-v1-128k", "moonshot-v1-32k", "moonshot-v1-8k"], modelId: "kimi-k2.6", baseURL: "https://api.moonshot.cn/v1", keyEnvName: "MOONSHOT_API_KEY", apiDocsURL: "https://platform.kimi.com/console/api-keys" },
  { id: "kimi-coding-plan", name: "Kimi Coding Plan", models: ["k3", "kimi-for-coding", "kimi-for-coding-highspeed"], modelId: "kimi-for-coding", baseURL: "https://api.kimi.com/coding/v1", keyEnvName: "KIMI_CODE_API_KEY", apiDocsURL: "https://www.kimi.com/code/console" },
  { id: "volcengine", name: "火山 Coding Plan", models: ["ark-code-latest", "doubao-seed-code", "doubao-seed-2.0-code", "doubao-seed-2.0-pro", "doubao-seed-2.0-lite", "minimax-m2.5", "minimax-m2.7", "minimax-m3", "glm-4.7", "glm-5.2", "glm-latest", "deepseek-v3.2", "deepseek-v4-flash", "deepseek-v4-pro", "kimi-k2.5", "kimi-k2.6", "kimi-k2.7-code"], modelId: "ark-code-latest", baseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", keyEnvName: "ARK_API_KEY", apiDocsURL: "https://www.volcengine.com/docs/82379/1541594", allowCustomModel: true },
];

type ConfiguredDemoDraft = {
  enabled: boolean;
  contextWindow: number;
  contextWindowInput: string;
  maxTokensInput: string;
};

const configuredModelInitial: Record<string, ConfiguredDemoDraft> = {
  deepseek: { enabled: true, contextWindow: 128_000, contextWindowInput: "128000", maxTokensInput: "8192" },
  kimi: { enabled: true, contextWindow: 262_144, contextWindowInput: "262144", maxTokensInput: "16384" },
};

export function AwakenSettingsDemo() {
  const [previewTheme, setPreviewTheme] = useState<"light" | "dark">("light");
  const [activeSectionId, setActiveSectionId] = useState("models");
  const [activeModelTabId, setActiveModelTabId] = useState("configure");
  const [expandedProvider, setExpandedProvider] = useState<string | null>(null);
  const [providerDrafts, setProviderDrafts] = useState(() => Object.fromEntries(
    providerSamples.map((provider) => [provider.id, {
      modelId: provider.modelId,
      baseURL: provider.baseURL,
      apiKey: "",
      supportsVision: provider.id === "volcengine",
      supportsNativePDF: false,
      supportsVideo: false,
    }]),
  ));
  const [showProviderApiKey, setShowProviderApiKey] = useState(false);
  const [configuredModels, setConfiguredModels] = useState(configuredModelInitial);
  const [themeChoice, setThemeChoice] = useState<AwakenThemeChoiceValue>("system");
  const [searchProviders, setSearchProviders] = useState<AwakenSearchProviderItem[]>(AwakenSearchSettingsSample.providers);
  const [memorySwitches, setMemorySwitches] = useState(AwakenMemorySettingsSample.switches);
  const [lastAction, setLastAction] = useState("当前完整组合直接使用 Awaken Desktop 消费的设置组件。");
  const originalThemeRef = useRef<string | null>(null);
  const configuredHasChanges = JSON.stringify(configuredModels) !== JSON.stringify(configuredModelInitial);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    root.setAttribute("data-aw-theme", previewTheme);
  }, [previewTheme]);

  useEffect(() => () => {
    if (originalThemeRef.current) document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    else document.documentElement.removeAttribute("data-aw-theme");
  }, []);

  let content;
  if (activeSectionId === "search") {
    content = <AwakenSearchSettings providers={searchProviders} onTestProvider={(id) => setLastAction(`已模拟测试 ${id} 连接。`)} onToggleProvider={(id) => setSearchProviders((current) => current.map((provider) => provider.id === id ? { ...provider, enabled: !provider.enabled } : provider))} />;
  } else if (activeSectionId === "memory") {
    content = <AwakenMemorySettings {...AwakenMemorySettingsSample} switches={memorySwitches} onToggleSwitch={(id) => setMemorySwitches((current) => current.map((item) => item.id === id ? { ...item, checked: !item.checked } : item))} onStartDream={() => setLastAction("已触发立即整理（Gallery 模拟）。")} />;
  } else if (activeSectionId === "appearance") {
    content = <AwakenAppearanceSettings value={themeChoice} onValueChange={(value) => { setThemeChoice(value); setPreviewTheme(value === "dark" ? "dark" : "light"); }} />;
  } else if (activeSectionId === "pets") {
    content = <AwakenPetsSettings {...AwakenPetsSettingsSample} onToggleOverlay={() => setLastAction("已触发唤醒宠物（Gallery 模拟）。")} />;
  } else if (activeModelTabId === "configured") {
    content = (
      <AwakenConfiguredModels
        models={[
          { id: "deepseek", displayName: "DeepSeek", modelId: "deepseek-v4-pro", contextWindowMax: 128_000 },
          { id: "kimi", displayName: "Kimi CN", modelId: "kimi-k2.6", contextWindowMax: 262_144 },
        ].map((model) => {
          const draft = configuredModels[model.id];
          const anchorValue = Math.round(model.contextWindowMax * 0.6);
          return {
            ...model,
            enabled: draft.enabled,
            contextWindow: draft.contextWindow,
            contextWindowInput: draft.contextWindowInput,
            contextWindowMin: 8_000,
            contextWindowStep: 1,
            contextFill: ((draft.contextWindow - 8_000) / (model.contextWindowMax - 8_000)) * 100,
            contextAnchor: ((anchorValue - 8_000) / (model.contextWindowMax - 8_000)) * 100,
            contextAnchorValue: anchorValue,
            contextAnchorLabel: `${Math.round(anchorValue / 1_000)}K`,
            contextEndLabel: model.contextWindowMax >= 1_000_000 ? `${model.contextWindowMax / 1_000_000}M` : `${Math.round(model.contextWindowMax / 1_000)}K`,
            compressionValue: Math.max(0, draft.contextWindow - 16_000),
            maxTokensInput: draft.maxTokensInput,
          };
        })}
        hasChanges={configuredHasChanges}
        onEnabledChange={(id, enabled) => setConfiguredModels((current) => ({ ...current, [id]: { ...current[id], enabled } }))}
        onContextRangeChange={(id, value) => setConfiguredModels((current) => ({ ...current, [id]: { ...current[id], contextWindow: value, contextWindowInput: String(value) } }))}
        onContextInputChange={(id, value) => setConfiguredModels((current) => ({ ...current, [id]: { ...current[id], contextWindowInput: value, contextWindow: Number(value) || current[id].contextWindow } }))}
        onContextInputCommit={(id) => setConfiguredModels((current) => {
          const value = Math.max(8_000, Number(current[id].contextWindowInput) || current[id].contextWindow);
          return { ...current, [id]: { ...current[id], contextWindow: value, contextWindowInput: String(value) } };
        })}
        onContextAnchorSelect={(id) => setConfiguredModels((current) => {
          const max = id === "deepseek" ? 128_000 : 262_144;
          const value = Math.round(max * 0.6);
          return { ...current, [id]: { ...current[id], contextWindow: value, contextWindowInput: String(value) } };
        })}
        onMaxTokensInputChange={(id, value) => setConfiguredModels((current) => ({ ...current, [id]: { ...current[id], maxTokensInput: value } }))}
        onReset={() => { setConfiguredModels(configuredModelInitial); setLastAction("已取消模型参数修改（Gallery 模拟）。"); }}
        onSave={() => setLastAction("已模拟保存模型参数。")}
        onUpdateApiKey={(id) => { setActiveModelTabId("configure"); setExpandedProvider(id === "kimi" ? "kimi-cn" : id); setLastAction("已进入供应商配置以更新 API key。"); }}
        onDelete={(id) => setLastAction(`已模拟删除 ${id} 模型配置。`)}
      />
    );
  } else {
    content = (
      <section className="model-provider-list" aria-label="模型配置">
        {providerSamples.map((provider) => {
          const providerDraft = providerDrafts[provider.id];
          return (
            <AwakenSettingsProviderCard key={provider.id} name={provider.name} expanded={expandedProvider === provider.id} onExpandedChange={(expanded) => { setExpandedProvider(expanded ? provider.id : null); setShowProviderApiKey(false); }}>
              <AwakenModelProviderForm
                providerId={provider.id}
                modelMode={provider.allowCustomModel ? "custom" : "select"}
                models={provider.models}
                modelId={providerDraft.modelId}
                baseURL={providerDraft.baseURL}
                apiKey={providerDraft.apiKey}
                apiKeyPlaceholder={`输入 ${provider.keyEnvName}`}
                apiDocsURL={provider.apiDocsURL}
                showApiKey={showProviderApiKey}
                editableCapabilities={provider.allowCustomModel}
                supportsVision={providerDraft.supportsVision}
                supportsNativePDF={providerDraft.supportsNativePDF}
                supportsVideo={providerDraft.supportsVideo}
                onModelChange={(modelId) => setProviderDrafts((current) => ({ ...current, [provider.id]: { ...current[provider.id], modelId } }))}
                onBaseURLChange={(baseURL) => setProviderDrafts((current) => ({ ...current, [provider.id]: { ...current[provider.id], baseURL } }))}
                onApiKeyChange={(apiKey) => setProviderDrafts((current) => ({ ...current, [provider.id]: { ...current[provider.id], apiKey } }))}
                onShowApiKeyChange={setShowProviderApiKey}
                onCapabilityChange={(capability, checked) => setProviderDrafts((current) => ({ ...current, [provider.id]: { ...current[provider.id], [capability]: checked, ...(capability === "supportsVision" && !checked ? { supportsVideo: false } : {}), ...(capability === "supportsVideo" && checked ? { supportsVision: true } : {}) } }))}
                onTest={() => setLastAction(`已模拟测试 ${provider.name} 连接。`)}
                onSubmit={() => setLastAction(`已模拟提交 ${provider.name} 模型配置。`)}
              />
            </AwakenSettingsProviderCard>
          );
        })}
      </section>
    );
  }

  return (
    <section className="ads-component-demo aw-settings-demo" aria-label="「设置」组件库 AwakenSettings 演示">
      <header className="ads-component-demo-header"><h2>「设置」组件库 <code>AwakenSettings</code></h2><p>这里展示的外壳与设置内容组件由 Awaken Desktop 直接复用；Gallery 只替换为无副作用的示例数据。</p></header>
      <div className="aw-settings-demo__controls"><p>{lastAction}</p><div className="aw-settings-demo__theme-toggle" role="group" aria-label="主题预览"><AwakenButton size="sm" variant={previewTheme === "light" ? "default" : "ghost"} aria-pressed={previewTheme === "light"} onClick={() => setPreviewTheme("light")}>浅色</AwakenButton><AwakenButton size="sm" variant={previewTheme === "dark" ? "default" : "ghost"} aria-pressed={previewTheme === "dark"} onClick={() => setPreviewTheme("dark")}>暗色</AwakenButton></div></div>

      <section className="aw-settings-demo__composition" aria-labelledby="aw-settings-composition-title">
        <header className="aw-settings-demo__section-heading"><h3 id="aw-settings-composition-title">完整组合</h3><p>弹窗头、两栏布局、内容滚动和底部关闭按钮均按当前 Desktop 设置页呈现。</p></header>
        <div className="aw-dialog aw-settings-demo__dialog-preview">
          <header className="aw-dialog__head"><h2 className="aw-dialog__title">设置</h2><AwakenIconButton label="关闭" size="sm" showTooltip={false} onClick={() => setLastAction("已触发关闭设置回调（Gallery 模拟）。")}><X size={15} strokeWidth={1.9} /></AwakenIconButton></header>
          <div className="aw-dialog__body">
            <AwakenSettings navItems={AwakenSettingsNavItems} activeSectionId={activeSectionId} sectionTitle={sectionTitles[activeSectionId] ?? "设置"} tabs={activeSectionId === "models" ? AwakenModelSettingsTabs : undefined} activeTabId={activeModelTabId} onSelectSection={(id) => { setActiveSectionId(id); setLastAction(`已进入${sectionTitles[id]}设置。`); }} onSelectTab={setActiveModelTabId}>
              {content}
            </AwakenSettings>
          </div>
          <footer className="aw-dialog__foot"><AwakenButton onClick={() => setLastAction("已触发关闭设置回调（Gallery 模拟）。")}>关闭</AwakenButton></footer>
        </div>
      </section>

      <section className="aw-settings-demo__parts" aria-labelledby="aw-settings-parts-title">
        <header className="aw-settings-demo__section-heading"><h3 id="aw-settings-parts-title">可独立组合的小组件</h3><p>以下小组件与完整组合使用同一实现，不单独占用左侧组件目录。</p></header>
        <div className="aw-settings-demo__part-list">
          <article className="aw-settings-demo__part"><header><h4>设置导航</h4><code>AwakenSettingsNav</code><p>五个一级设置分区。</p></header><div className="aw-settings-demo__part-canvas aw-settings-demo__part-canvas--nav"><AwakenSettingsNav items={AwakenSettingsNavItems} activeId={activeSectionId} onSelect={setActiveSectionId} /></div></article>
          <article className="aw-settings-demo__part"><header><h4>模型页签</h4><code>AwakenSettingsTabs</code><p>模型配置与已配置模型切换。</p></header><div className="aw-settings-demo__part-canvas"><AwakenSettingsTabs items={AwakenModelSettingsTabs} activeId={activeModelTabId} onSelect={setActiveModelTabId} /></div></article>
          <article className="aw-settings-demo__part"><header><h4>供应商配置</h4><code>AwakenSettingsProviderCard + AwakenModelProviderForm</code><p>包含真实折叠卡、模型选择、密钥与提交交互。</p></header><div className="aw-settings-demo__part-canvas"><AwakenSettingsProviderCard name="DeepSeek" defaultExpanded><AwakenModelProviderForm providerId="deepseek-part" modelMode="select" models={providerSamples[0].models} modelId={providerDrafts.deepseek.modelId} baseURL={providerDrafts.deepseek.baseURL} apiKey={providerDrafts.deepseek.apiKey} apiKeyPlaceholder="输入 DEEPSEEK_API_KEY" apiDocsURL={providerSamples[0].apiDocsURL} showApiKey={showProviderApiKey} onModelChange={(modelId) => setProviderDrafts((current) => ({ ...current, deepseek: { ...current.deepseek, modelId } }))} onBaseURLChange={(baseURL) => setProviderDrafts((current) => ({ ...current, deepseek: { ...current.deepseek, baseURL } }))} onApiKeyChange={(apiKey) => setProviderDrafts((current) => ({ ...current, deepseek: { ...current.deepseek, apiKey } }))} onShowApiKeyChange={setShowProviderApiKey} onTest={() => setLastAction("已模拟测试 DeepSeek 连接。")} onSubmit={() => setLastAction("已模拟提交 DeepSeek 模型配置。")} /></AwakenSettingsProviderCard></div></article>
          <article className="aw-settings-demo__part"><header><h4>已配置模型</h4><code>AwakenConfiguredModels</code><p>可直接调整启用状态、上下文长度与输出上限。</p></header><div className="aw-settings-demo__part-canvas aw-settings-demo__part-canvas--wide"><AwakenConfiguredModels models={[{ id: "deepseek", displayName: "DeepSeek", modelId: "deepseek-v4-pro", enabled: configuredModels.deepseek.enabled, contextWindow: configuredModels.deepseek.contextWindow, contextWindowInput: configuredModels.deepseek.contextWindowInput, contextWindowMin: 8_000, contextWindowMax: 128_000, contextWindowStep: 1, contextFill: ((configuredModels.deepseek.contextWindow - 8_000) / 120_000) * 100, contextAnchor: 60, contextAnchorValue: 76_800, contextAnchorLabel: "77K", contextEndLabel: "128K", compressionValue: Math.max(0, configuredModels.deepseek.contextWindow - 16_000), maxTokensInput: configuredModels.deepseek.maxTokensInput }]} hasChanges={configuredHasChanges} onEnabledChange={(_, enabled) => setConfiguredModels((current) => ({ ...current, deepseek: { ...current.deepseek, enabled } }))} onContextRangeChange={(_, value) => setConfiguredModels((current) => ({ ...current, deepseek: { ...current.deepseek, contextWindow: value, contextWindowInput: String(value) } }))} onContextInputChange={(_, value) => setConfiguredModels((current) => ({ ...current, deepseek: { ...current.deepseek, contextWindowInput: value, contextWindow: Number(value) || current.deepseek.contextWindow } }))} onMaxTokensInputChange={(_, value) => setConfiguredModels((current) => ({ ...current, deepseek: { ...current.deepseek, maxTokensInput: value } }))} onReset={() => setConfiguredModels(configuredModelInitial)} onSave={() => setLastAction("已模拟保存模型参数。")}/></div></article>
          <article className="aw-settings-demo__part"><header><h4>主题选择</h4><code>AwakenThemeChoiceGroup</code><p>跟随系统、浅色与黑色。</p></header><div className="aw-settings-demo__part-canvas"><AwakenThemeChoiceGroup value={themeChoice} onValueChange={setThemeChoice} /></div></article>
          <article className="aw-settings-demo__part"><header><h4>搜索设置</h4><code>AwakenSearchSettings</code><p>真实搜索渠道卡片与开关。</p></header><div className="aw-settings-demo__part-canvas"><AwakenSearchSettings {...AwakenSearchSettingsSample} /></div></article>
          <article className="aw-settings-demo__part"><header><h4>记忆设置</h4><code>AwakenMemorySettings</code><p>四开关、过滤器、列表和详情。</p></header><div className="aw-settings-demo__part-canvas"><AwakenMemorySettings {...AwakenMemorySettingsSample} /></div></article>
          <article className="aw-settings-demo__part"><header><h4>宠物设置</h4><code>AwakenPetsSettings</code><p>选择、目录与浮窗动作。</p></header><div className="aw-settings-demo__part-canvas"><AwakenPetsSettings {...AwakenPetsSettingsSample} /></div></article>
        </div>
      </section>
    </section>
  );
}
