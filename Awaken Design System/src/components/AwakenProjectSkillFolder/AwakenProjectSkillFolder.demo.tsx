import "./AwakenProjectSkillFolder.demo.css";
import { useEffect, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenProjectSkillFolder } from "./AwakenProjectSkillFolder";
import { AwakenProjectSkillFolderStates } from "./AwakenProjectSkillFolder.states";

export function AwakenProjectSkillFolderDemo() {
  const [activeProjectId, setActiveProjectId] = useState<string | null>(null);
  const [theme, setTheme] = useState<"light" | "dark">("dark");
  const originalThemeRef = useRef<string | null>(null);
  const activeProject = AwakenProjectSkillFolderStates.find(
    (state) => state.id === activeProjectId,
  ) ?? null;

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", theme);
  }, [theme]);

  useEffect(() => () => {
    if (typeof document === "undefined" || originalThemeRef.current === null) {
      return;
    }
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  return (
    <section className="ads-component-demo" aria-label="项目 Skill 文件夹 AwakenProjectSkillFolder 组件演示">
      <header className="ads-component-demo-header">
        <h2>项目 Skill 文件夹 <code>AwakenProjectSkillFolder</code></h2>
        <p>以白色叠层档案夹展示项目，悬停展开内容页，点击进入该项目的 Skills。</p>
      </header>

      <div className="aw-project-skill-folder-demo__surface">
        <div className="aw-project-skill-folder-demo__toolbar">
          <p className="aw-project-skill-folder-demo__hint">
            将鼠标放到任一文件夹上查看动效，点击可打开项目 Skills 预览窗口。
          </p>
          <div className="aw-project-skill-folder-demo__theme-toggle" role="group" aria-label="主题预览">
            <AwakenButton
              size="sm"
              variant={theme === "light" ? "default" : "ghost"}
              aria-pressed={theme === "light"}
              onClick={() => setTheme("light")}
            >
              浅色
            </AwakenButton>
            <AwakenButton
              size="sm"
              variant={theme === "dark" ? "default" : "ghost"}
              aria-pressed={theme === "dark"}
              onClick={() => setTheme("dark")}
            >
              暗色
            </AwakenButton>
          </div>
        </div>
        <div className="aw-project-skill-folder-demo__grid">
          {AwakenProjectSkillFolderStates.map((state) => (
            <AwakenProjectSkillFolder
              key={state.id}
              {...state.props}
              open={activeProjectId === state.id}
              onOpen={() => setActiveProjectId(state.id)}
            />
          ))}
        </div>
      </div>

      <AwakenDialog
        open={activeProject !== null}
        title={activeProject ? `${activeProject.props.projectName} Skills` : "项目 Skills"}
        size="large"
        onOpenChange={(open) => {
          if (!open) {
            setActiveProjectId(null);
          }
        }}
      >
        {activeProject ? (
          <>
            <div className="aw-project-skill-folder-demo__dialog-heading">
              <span>该项目包含的 Skills</span>
              <span>{activeProject.props.skillCount ?? activeProject.skills.length} Skills</span>
            </div>
            <div className="aw-project-skill-folder-demo__skill-grid">
              {activeProject.skills.map((skill) => (
                <article className="aw-project-skill-folder-demo__skill" key={skill}>
                  <strong>{skill}</strong>
                  <span>点击后由 Desktop 打开只读 Skill 包预览</span>
                </article>
              ))}
            </div>
          </>
        ) : null}
      </AwakenDialog>
    </section>
  );
}
