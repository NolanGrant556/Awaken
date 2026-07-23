import { useEffect, useMemo, useRef, useState } from "react";
import "./AwakenMemorySettings.demo.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenMemoryLightFlowLayout } from "../AwakenMemoryLightFlow";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenMemorySettings } from "./AwakenMemorySettings";
import { AwakenMemorySettingsSample, AwakenMemorySettingsSampleDetails } from "./AwakenMemorySettings.states";

export function AwakenMemorySettingsDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [switches, setSwitches] = useState(AwakenMemorySettingsSample.switches);
  const [libraryScope, setLibraryScope] = useState<"user" | "project">("user");
  const [libraryProjectId, setLibraryProjectId] = useState("");
  const [memories, setMemories] = useState(AwakenMemorySettingsSample.memories);
  const [selectedMemoryId, setSelectedMemoryId] = useState<string | null>(null);
  const originalThemeRef = useRef<string | null>(null);
  const memoryEnabled = switches.find((item) => item.id === "enabled")?.checked ?? false;

  const visibleMemories = useMemo(() => {
    return memories.filter((memory) => {
      if (libraryScope === "user") return !memory.projectId;
      return memory.projectId === libraryProjectId;
    });
  }, [libraryProjectId, libraryScope, memories]);

  const librarySummary = useMemo(() => {
    const userMemories = memories.filter((memory) => !memory.projectId);
    const projectMemories = memories.filter((memory) => memory.projectId);
    return {
      userTopicCount: userMemories.length,
      userUpdatedLabel: userMemories[0]?.updatedLabel,
      projectCount: new Set(projectMemories.map((memory) => memory.projectId)).size,
      projectTopicCount: projectMemories.length,
      projectUpdatedLabel: projectMemories[0]?.updatedLabel,
    };
  }, [memories]);

  const memoryProjects = useMemo(() => (AwakenMemorySettingsSample.memoryProjects ?? []).map((project) => ({
    ...project,
    topicCount: memories.filter((memory) => memory.projectId === project.id).length,
  })).filter((project) => project.topicCount > 0), [memories]);

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
            switches={switches.map((item) => ({
              ...item,
              disabled: item.id !== "enabled" && !memoryEnabled,
            }))}
            memories={visibleMemories}
            librarySummary={librarySummary}
            memoryProjects={memoryProjects}
            canStartDream={memoryEnabled}
            selectedMemoryId={selectedMemoryId}
            detail={selectedMemoryId ? AwakenMemorySettingsSampleDetails[selectedMemoryId] ?? null : null}
            onToggleSwitch={(id) => setSwitches((current) => {
              const memoryEnabled = current.find((item) => item.id === "enabled")?.checked ?? false;
              if (id !== "enabled" && !memoryEnabled) return current;
              if (id === "enabled") {
                return current.map((item) => ({ ...item, checked: !memoryEnabled }));
              }
              return current.map((item) => item.id === id ? { ...item, checked: !item.checked } : item);
            })}
            onOpenLibraryScope={(scope, projectId) => {
              setLibraryScope(scope);
              setLibraryProjectId(projectId ?? "");
            }}
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
