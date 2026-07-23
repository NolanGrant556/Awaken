import { useEffect, useRef, useState } from "react";
import "./AwakenMemorySettings.demo.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenMemoryLightFlowLayout } from "../AwakenMemoryLightFlow";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenMemorySettings } from "./AwakenMemorySettings";
import { AwakenMemorySettingsSample } from "./AwakenMemorySettings.states";

export function AwakenMemorySettingsDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [switches, setSwitches] = useState(AwakenMemorySettingsSample.switches);
  const originalThemeRef = useRef<string | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", theme);
  }, [theme]);

  useEffect(() => () => {
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  return (
    <section className="ads-component-demo" aria-label="记忆设置 AwakenMemorySettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>记忆设置 <code>AwakenMemorySettings</code></h2>
        <p>记忆光流与现有记忆功能组合成独立设置页；不读取真实记忆数据。</p>
      </header>
      <div className="aw-memory-settings-demo__controls">
        <span>主题预览</span>
        <div className="aw-memory-settings-demo__theme-toggle" role="group" aria-label="主题预览">
          <AwakenButton size="sm" variant={theme === "light" ? "default" : "ghost"} onClick={() => setTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={theme === "dark" ? "default" : "ghost"} onClick={() => setTheme("dark")}>深色</AwakenButton>
        </div>
      </div>
      <AwakenSettingsSectionPreview activeSectionId="memory" sectionTitle="记忆" fullBleed>
        <AwakenMemoryLightFlowLayout theme={theme} titleAs="h1">
          <AwakenMemorySettings
            {...AwakenMemorySettingsSample}
            switches={switches}
            onToggleSwitch={(id) => setSwitches((current) => current.map((item) => (
              item.id === id ? { ...item, checked: !item.checked } : item
            )))}
          />
        </AwakenMemoryLightFlowLayout>
      </AwakenSettingsSectionPreview>
    </section>
  );
}
