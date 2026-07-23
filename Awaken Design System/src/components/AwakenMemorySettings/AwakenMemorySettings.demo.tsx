import { useEffect, useMemo, useRef, useState } from "react";
import "./AwakenMemorySettings.demo.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenMemoryLightFlowLayout } from "../AwakenMemoryLightFlow";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenMemorySettings, type AwakenMemoryFilterValue } from "./AwakenMemorySettings";
import { AwakenMemorySettingsSample, AwakenMemorySettingsSampleDetails } from "./AwakenMemorySettings.states";

export function AwakenMemorySettingsDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [switches, setSwitches] = useState(AwakenMemorySettingsSample.switches);
  const [filters, setFilters] = useState(AwakenMemorySettingsSample.filters);
  const [query, setQuery] = useState("");
  const [memories, setMemories] = useState(AwakenMemorySettingsSample.memories);
  const [selectedMemoryId, setSelectedMemoryId] = useState<string | null>(null);
  const originalThemeRef = useRef<string | null>(null);

  const visibleMemories = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    const values = Object.fromEntries(filters.map((filter) => [filter.id, filter.value]));
    return memories.filter((memory) => {
      if (values.scope === "project") return false;
      if (values.type && memory.kind !== values.type) return false;
      if (values.status && memory.status !== values.status) return false;
      if (normalizedQuery && !`${memory.name}\n${memory.description}`.toLocaleLowerCase().includes(normalizedQuery)) return false;
      return true;
    });
  }, [filters, memories, query]);

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
            filters={filters}
            query={query}
            memories={visibleMemories}
            selectedMemoryId={selectedMemoryId}
            detail={selectedMemoryId ? AwakenMemorySettingsSampleDetails[selectedMemoryId] ?? null : null}
            onToggleSwitch={(id) => setSwitches((current) => current.map((item) => (
              item.id === id ? { ...item, checked: !item.checked } : item
            )))}
            onQueryChange={setQuery}
            onFilterChange={(id: string, value: AwakenMemoryFilterValue) => setFilters((current) => current.map((filter) => filter.id === id ? { ...filter, value } : filter))}
            onOpenMemory={setSelectedMemoryId}
            onCloseMemory={() => setSelectedMemoryId(null)}
            onDeleteMemory={(id) => {
              setMemories((current) => current.filter((memory) => memory.id !== id));
              setSelectedMemoryId(null);
            }}
          />
        </AwakenMemoryLightFlowLayout>
      </AwakenSettingsSectionPreview>
    </section>
  );
}
