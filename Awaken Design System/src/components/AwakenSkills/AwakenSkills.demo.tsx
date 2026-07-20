import "./AwakenSkills.demo.css";
import { useEffect, useMemo, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenProjectSkillFolder } from "../AwakenProjectSkillFolder";
import { AwakenProjectSkillsDialog } from "../AwakenProjectSkillsDialog";
import { AwakenSkillCard } from "../AwakenSkillCard";
import { AwakenSkillPreviewDialog } from "../AwakenSkillPreviewDialog";
import { AwakenSkillToggle } from "../AwakenSkillToggle";
import { AwakenSkills } from "./AwakenSkills";
import type {
  AwakenSkillGroup,
  AwakenSkillItem,
  AwakenSkillProject,
  AwakenSkillsView,
} from "./AwakenSkills.types";
import {
  AwakenSkillPreviewStates,
  AwakenSkillsAgentStates,
  AwakenSkillsAllStates,
  AwakenSkillsAwakenStates,
  AwakenSkillsGlobalStates,
  AwakenSkillsProjectStates,
} from "./AwakenSkills.states";

function applyEnabledState(
  item: AwakenSkillItem,
  enabledById: Record<string, boolean>,
): AwakenSkillItem {
  return { ...item, enabled: enabledById[item.id] ?? item.enabled };
}

function applyEnabledGroups(
  groups: AwakenSkillGroup[],
  enabledById: Record<string, boolean>,
): AwakenSkillGroup[] {
  return groups.map((group) => ({
    ...group,
    skills: group.skills.map((item) => applyEnabledState(item, enabledById)),
  }));
}

function applyEnabledProjects(
  projects: AwakenSkillProject[],
  enabledById: Record<string, boolean>,
): AwakenSkillProject[] {
  return projects.map((project) => ({
    ...project,
    skills: project.skills.map((item) => applyEnabledState(item, enabledById)),
  }));
}

export function AwakenSkillsDemo() {
  const [activeView, setActiveView] = useState<AwakenSkillsView>("awaken");
  const [searchValue, setSearchValue] = useState("");
  const [openProjectId, setOpenProjectId] = useState<string | null>(null);
  const [returnProjectId, setReturnProjectId] = useState<string | null>(null);
  const [previewSkill, setPreviewSkill] = useState<AwakenSkillItem | null>(null);
  const [isolatedProjectOpen, setIsolatedProjectOpen] = useState(false);
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [enabledById, setEnabledById] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(AwakenSkillsAllStates.map((item) => [item.id, item.enabled]))
  );
  const originalThemeRef = useRef<string | null>(null);

  const awakenSkills = useMemo(
    () => AwakenSkillsAwakenStates.map((item) => applyEnabledState(item, enabledById)),
    [enabledById],
  );
  const projects = useMemo(
    () => applyEnabledProjects(AwakenSkillsProjectStates, enabledById),
    [enabledById],
  );
  const globalGroups = useMemo(
    () => applyEnabledGroups(AwakenSkillsGlobalStates, enabledById),
    [enabledById],
  );
  const agentGroups = useMemo(
    () => applyEnabledGroups(AwakenSkillsAgentStates, enabledById),
    [enabledById],
  );
  const preview = previewSkill ? AwakenSkillPreviewStates[previewSkill.id] : undefined;
  const componentSampleSkill = awakenSkills[0];
  const componentSampleProject = projects[3] ?? projects[0] ?? null;

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

  function updateSkill(skill: AwakenSkillItem, enabled: boolean): void {
    setEnabledById((current) => ({ ...current, [skill.id]: enabled }));
  }

  function handlePreview(skill: AwakenSkillItem): void {
    if (skill.projectId && openProjectId) {
      setReturnProjectId(openProjectId);
      setOpenProjectId(null);
    } else {
      setReturnProjectId(null);
    }
    setPreviewSkill(skill);
  }

  return (
    <section className="ads-component-demo aw-skills-demo" aria-label="「技能」组件库 AwakenSkills 演示">
      <header className="ads-component-demo-header">
        <h2>「技能」组件库 <code>AwakenSkills</code></h2>
        <p>先查看完整组合，再逐个识别和操作可由 Awaken 单独复用的小组件。</p>
      </header>

      <div className="aw-skills-demo__controls">
        <p>组合组件与下面的小组件共享同一组模拟状态，便于观察组合关系。</p>
        <div className="aw-skills-demo__theme-toggle" role="group" aria-label="主题预览">
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

      <section className="aw-skills-demo__composition" aria-labelledby="aw-skills-composition-title">
        <header className="aw-skills-demo__section-heading">
          <h3 id="aw-skills-composition-title">完整组合</h3>
          <p><code>AwakenSkills</code> 组合来源切换、搜索、卡片、项目文件夹和项目窗口。</p>
        </header>
        <AwakenSkills
          activeView={activeView}
          searchValue={searchValue}
          awakenSkills={awakenSkills}
          projects={projects}
          globalGroups={globalGroups}
          agentGroups={agentGroups}
          openProjectId={openProjectId}
          onViewChange={(view) => {
            setActiveView(view);
            setSearchValue("");
            setOpenProjectId(null);
          }}
          onSearchChange={setSearchValue}
          onProjectOpenChange={setOpenProjectId}
          onSkillPreview={handlePreview}
          onSkillToggle={updateSkill}
        />
      </section>

      <section className="aw-skills-demo__parts" aria-labelledby="aw-skills-parts-title">
        <header className="aw-skills-demo__section-heading">
          <h3 id="aw-skills-parts-title">可独立组合的小组件</h3>
          <p>每一项都有独立目录、Props 和状态 Demo，并集中在当前组件库入口中查看。</p>
        </header>

        <div className="aw-skills-demo__part-list">
          <article className="aw-skills-demo__part">
            <header>
              <h4>Skill 卡片</h4>
              <code>AwakenSkillCard</code>
              <p>承载名称、说明、来源、预览入口与开关。</p>
            </header>
            <div className="aw-skills-demo__part-canvas">
              {componentSampleSkill ? (
                <AwakenSkillCard
                  skill={componentSampleSkill}
                  onPreview={() => handlePreview(componentSampleSkill)}
                  onEnabledChange={(enabled) => updateSkill(componentSampleSkill, enabled)}
                />
              ) : null}
            </div>
          </article>

          <article className="aw-skills-demo__part">
            <header>
              <h4>Skill 开关</h4>
              <code>AwakenSkillToggle</code>
              <p>只负责受控启停状态，可脱离卡片独立使用。</p>
            </header>
            <div className="aw-skills-demo__part-canvas aw-skills-demo__toggle-row">
              {componentSampleSkill ? (
                <>
                  <AwakenSkillToggle
                    checked={componentSampleSkill.enabled}
                    label={componentSampleSkill.name}
                    onCheckedChange={(enabled) => updateSkill(componentSampleSkill, enabled)}
                  />
                  <span>{componentSampleSkill.enabled ? "已启用" : "已停用"}</span>
                </>
              ) : null}
            </div>
          </article>

          <article className="aw-skills-demo__part aw-skills-demo__part--folder">
            <header>
              <h4>项目 Skill 文件夹</h4>
              <code>AwakenProjectSkillFolder</code>
              <p>项目级入口；窗口打开期间持续保持展开。</p>
            </header>
            <div className="aw-skills-demo__part-canvas aw-skills-demo__folder-canvas">
              {componentSampleProject ? (
                <AwakenProjectSkillFolder
                  projectName={componentSampleProject.name}
                  skillCount={componentSampleProject.skills.length}
                  open={isolatedProjectOpen}
                  onOpen={() => setIsolatedProjectOpen(true)}
                />
              ) : null}
            </div>
          </article>

          <article className="aw-skills-demo__part">
            <header>
              <h4>项目 Skills 窗口</h4>
              <code>AwakenProjectSkillsDialog</code>
              <p>接收一个项目，并使用统一 Skill 卡片展示内容。</p>
            </header>
            <div className="aw-skills-demo__part-canvas">
              <AwakenButton onClick={() => setIsolatedProjectOpen(true)}>打开项目 Skills 窗口</AwakenButton>
            </div>
          </article>

          <article className="aw-skills-demo__part">
            <header>
              <h4>Skill 预览窗口</h4>
              <code>AwakenSkillPreviewDialog</code>
              <p>展示完整 frontmatter 和安全 Markdown 正文。</p>
            </header>
            <div className="aw-skills-demo__part-canvas">
              <AwakenButton
                onClick={() => {
                  if (componentSampleSkill) {
                    handlePreview(componentSampleSkill);
                  }
                }}
              >
                打开 SKILL.md 预览
              </AwakenButton>
            </div>
          </article>
        </div>
      </section>

      <AwakenProjectSkillsDialog
        open={isolatedProjectOpen}
        project={componentSampleProject}
        onOpenChange={setIsolatedProjectOpen}
        onSkillPreview={(skill) => {
          setIsolatedProjectOpen(false);
          handlePreview(skill);
        }}
        onSkillToggle={updateSkill}
      />

      <AwakenSkillPreviewDialog
        open={previewSkill !== null}
        title={previewSkill ? `${previewSkill.name} 的 SKILL.md` : "SKILL.md 预览"}
        contentKey={previewSkill?.id}
        frontmatterRaw={preview?.frontmatterRaw}
        markdown={preview?.markdown}
        onOpenChange={(open) => {
          if (open) {
            return;
          }
          setPreviewSkill(null);
          if (returnProjectId) {
            setOpenProjectId(returnProjectId);
            setReturnProjectId(null);
          }
        }}
      />
    </section>
  );
}
