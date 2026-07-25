import "./AwakenSettings.demo.css";
import { useEffect, useRef, useState } from "react";
import { AwakenAppearanceSettings } from "../AwakenAppearanceSettings";
import { AwakenButton } from "../AwakenButton";
import { AwakenConfiguredModels } from "../AwakenConfiguredModels";
import { AwakenMemoryLightFlowLayout } from "../AwakenMemoryLightFlow";
import { AwakenMemorySettings } from "../AwakenMemorySettings";
import { AwakenMemorySettingsSample } from "../AwakenMemorySettings/AwakenMemorySettings.states";
import { AwakenModelProviderForm } from "../AwakenModelProviderForm";
import { AwakenModelSettings } from "../AwakenModelSettings";
import { AwakenPetsSettings } from "../AwakenPetsSettings";
import { AwakenPetsSettingsSample } from "../AwakenPetsSettings/AwakenPetsSettings.states";
import {
  AwakenSearchSettings,
  type AwakenSearchProviderItem,
} from "../AwakenSearchSettings";
import { AwakenSearchSettingsSample } from "../AwakenSearchSettings/AwakenSearchSettings.states";
import { AwakenSettingsNavItems } from "../AwakenSettingsNav";
import { AwakenSettingsProviderCard } from "../AwakenSettingsProviderCard";
import { AwakenModelSettingsTabs } from "../AwakenSettingsTabs";
import type { AwakenThemeChoiceValue } from "../AwakenThemeChoiceGroup";
import { AwakenSettings } from "./AwakenSettings";

const sectionTitles: Record<string, string> = {
  models: "模型",
  search: "搜索",
  memory: "记忆",
  appearance: "外观",
  pets: "宠物",
  acp: "ACP",
};

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

export function AwakenSettingsDemo() {
  const [previewTheme, setPreviewTheme] = useState<"light" | "dark">("light");
  const [activeSectionId, setActiveSectionId] = useState("memory");
  const [activeModelTabId, setActiveModelTabId] = useState("configure");
  const [themeChoice, setThemeChoice] = useState<AwakenThemeChoiceValue>("system");
  const [searchProviders, setSearchProviders] = useState<AwakenSearchProviderItem[]>(
    AwakenSearchSettingsSample.providers,
  );
  const [memorySwitches, setMemorySwitches] = useState(
    AwakenMemorySettingsSample.switches,
  );
  const [modelId, setModelId] = useState("deepseek-v4-pro");
  const [baseURL, setBaseURL] = useState("https://api.deepseek.com");
  const [apiKey, setApiKey] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);
  const [lastAction, setLastAction] = useState("当前预览与 Desktop 使用同一套设置页组件。");
  const originalThemeRef = useRef<string | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", previewTheme);
  }, [previewTheme]);

  useEffect(() => () => {
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  let content;
  if (activeSectionId === "search") {
    content = (
      <AwakenSearchSettings
        {...AwakenSearchSettingsSample}
        providers={searchProviders}
        onToggleProvider={(id) => setSearchProviders((current) => current.map((provider) => (
          provider.id === id ? { ...provider, enabled: !provider.enabled } : provider
        )))}
        onTestProvider={(id) => setLastAction(`已模拟测试 ${id} 连接。`)}
      />
    );
  } else if (activeSectionId === "memory") {
    content = (
      <AwakenMemoryLightFlowLayout theme={previewTheme} titleAs="h1">
        <AwakenMemorySettings
          {...AwakenMemorySettingsSample}
          switches={memorySwitches}
          onToggleSwitch={(id) => setMemorySwitches((current) => current.map((item) => (
            item.id === id ? { ...item, checked: !item.checked } : item
          )))}
          onStartDream={() => setLastAction("已触发立即整理（Gallery 模拟）。")}
        />
      </AwakenMemoryLightFlowLayout>
    );
  } else if (activeSectionId === "appearance") {
    content = (
      <AwakenAppearanceSettings
        value={themeChoice}
        onValueChange={(value) => {
          setThemeChoice(value);
          if (value !== "system") setPreviewTheme(value);
        }}
      />
    );
  } else if (activeSectionId === "pets") {
    content = (
      <AwakenPetsSettings
        {...AwakenPetsSettingsSample}
        onToggleOverlay={() => setLastAction("已触发唤醒宠物（Gallery 模拟）。")}
      />
    );
  } else if (activeSectionId === "acp") {
    // 设计系统只拥有 ACP 这一个导航入口，runner 列表和编辑表单由 Desktop 拥有。
    // 不占位就会落进下面的 else，在「ACP」标题下显示模型设置。
    content = (
      <p>ACP runner 的列表与编辑表单由 Desktop 拥有，Gallery 只观测这个导航入口。</p>
    );
  } else {
    content = (
      <AwakenModelSettings
        tabs={AwakenModelSettingsTabs}
        activeTabId={activeModelTabId}
        onSelectTab={setActiveModelTabId}
      >
        {activeModelTabId === "configure" ? (
          <section className="model-provider-list" aria-label="模型配置">
            <AwakenSettingsProviderCard name="DeepSeek" defaultExpanded>
              <AwakenModelProviderForm
                providerId="deepseek"
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
                onTest={() => setLastAction("已模拟测试 DeepSeek 连接。")}
                onSubmit={() => setLastAction("已模拟提交 DeepSeek 模型配置。")}
              />
            </AwakenSettingsProviderCard>
          </section>
        ) : (
          <AwakenConfiguredModels models={[configuredModel]} />
        )}
      </AwakenModelSettings>
    );
  }

  return (
    <section className="ads-component-demo aw-settings-demo" aria-label="设置页组件 AwakenSettings 演示">
      <header className="ads-component-demo-header">
        <h2>设置页框架 <code>AwakenSettings</code></h2>
        <p>全窗口左右分栏；Gallery 使用无副作用示例数据，Desktop 直接消费相同组件。</p>
      </header>
      <div className="aw-settings-demo__controls">
        <p>{lastAction}</p>
        <div className="aw-settings-demo__theme-toggle" role="group" aria-label="主题预览">
          <AwakenButton size="sm" variant={previewTheme === "light" ? "default" : "ghost"} onClick={() => setPreviewTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={previewTheme === "dark" ? "default" : "ghost"} onClick={() => setPreviewTheme("dark")}>深色</AwakenButton>
        </div>
      </div>
      <div className="aw-settings-demo__preview">
        <AwakenSettings
          presentation="embedded"
          contentLayout={activeSectionId === "memory" ? "full-bleed" : "standard"}
          navItems={AwakenSettingsNavItems}
          activeSectionId={activeSectionId}
          sectionTitle={sectionTitles[activeSectionId] ?? "设置"}
          onBack={() => setLastAction("已触发返回应用回调（Gallery 模拟）。")}
          onSelectSection={(id) => {
            setActiveSectionId(id);
            setLastAction(`已进入${sectionTitles[id]}设置。`);
          }}
        >
          {content}
        </AwakenSettings>
      </div>
    </section>
  );
}
