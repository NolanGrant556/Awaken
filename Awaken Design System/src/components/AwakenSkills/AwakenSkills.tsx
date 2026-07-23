import "./AwakenSkills.css";
import { Search } from "lucide-react";
import type { HTMLAttributes } from "react";
import { AwakenFolder } from "../AwakenFolder";
import { AwakenProjectSkillsDialog } from "../AwakenProjectSkillsDialog";
import { AwakenSkillCard } from "../AwakenSkillCard";
import type {
  AwakenSkillGroup,
  AwakenSkillItem,
  AwakenSkillProject,
  AwakenSkillsView,
} from "./AwakenSkills.types";

const viewOptions: Array<{ id: AwakenSkillsView; label: string }> = [
  { id: "awaken", label: "Awaken Skills" },
  { id: "project", label: "项目 Skills" },
  { id: "global", label: "全局 Skills" },
  { id: "agents", label: "Agents Skills" },
];

function normalizedIncludes(value: string | undefined, query: string): boolean {
  return Boolean(value?.toLocaleLowerCase().includes(query));
}

function filterSkills(skills: AwakenSkillItem[], query: string): AwakenSkillItem[] {
  if (!query) {
    return skills;
  }
  return skills.filter((skill) =>
    normalizedIncludes(skill.name, query)
    || normalizedIncludes(skill.description, query)
    || normalizedIncludes(skill.sourceLabel, query)
    || normalizedIncludes(skill.diagnostic, query));
}

function filterGroups(groups: AwakenSkillGroup[], query: string): AwakenSkillGroup[] {
  return groups.flatMap((group) => {
    const groupMatches = normalizedIncludes(group.label, query)
      || normalizedIncludes(group.path, query);
    const visibleSkills = groupMatches ? group.skills : filterSkills(group.skills, query);
    return visibleSkills.length > 0 ? [{ ...group, skills: visibleSkills }] : [];
  });
}

function filterProjects(projects: AwakenSkillProject[], query: string): AwakenSkillProject[] {
  return projects.flatMap((project) => {
    const projectMatches = normalizedIncludes(project.name, query)
      || normalizedIncludes(project.path, query);
    const visibleSkills = projectMatches ? project.skills : filterSkills(project.skills, query);
    return visibleSkills.length > 0 ? [{ ...project, skills: visibleSkills }] : [];
  });
}

type SkillGridProps = {
  skills: AwakenSkillItem[];
  emptyMessage: string;
  onPreview: (skill: AwakenSkillItem) => void;
  onToggle: (skill: AwakenSkillItem, enabled: boolean) => void;
};

function SkillGrid({
  skills,
  emptyMessage,
  onPreview,
  onToggle,
}: SkillGridProps) {
  if (skills.length === 0) {
    return <div className="aw-skills__empty">{emptyMessage}</div>;
  }

  return (
    <div className="aw-skills__card-grid">
      {skills.map((skill) => (
        <AwakenSkillCard
          key={skill.id}
          skill={skill}
          onPreview={(skill.previewable ?? true) ? () => onPreview(skill) : undefined}
          onEnabledChange={(enabled) => onToggle(skill, enabled)}
        />
      ))}
    </div>
  );
}

export type AwakenSkillsProps = Omit<
  HTMLAttributes<HTMLDivElement>,
  "children" | "onChange"
> & {
  activeView: AwakenSkillsView;
  searchValue: string;
  awakenSkills: AwakenSkillItem[];
  projects: AwakenSkillProject[];
  globalGroups: AwakenSkillGroup[];
  agentGroups: AwakenSkillGroup[];
  openProjectId?: string | null;
  onViewChange: (view: AwakenSkillsView) => void;
  onSearchChange: (value: string) => void;
  onSkillPreview: (skill: AwakenSkillItem) => void;
  onSkillToggle: (skill: AwakenSkillItem, enabled: boolean) => void;
  onProjectOpenChange: (projectId: string | null) => void;
};

export function AwakenSkills({
  activeView,
  searchValue,
  awakenSkills,
  projects,
  globalGroups,
  agentGroups,
  openProjectId = null,
  onViewChange,
  onSearchChange,
  onSkillPreview,
  onSkillToggle,
  onProjectOpenChange,
  className,
  ...props
}: AwakenSkillsProps) {
  const query = searchValue.trim().toLocaleLowerCase();
  const visibleAwakenSkills = filterSkills(awakenSkills, query);
  const visibleProjects = filterProjects(projects, query);
  const visibleGlobalGroups = filterGroups(globalGroups, query);
  const visibleAgentGroups = filterGroups(agentGroups, query);
  const activeProject = visibleProjects.find((project) => project.id === openProjectId) ?? null;

  return (
    <div
      {...props}
      className={["aw-skills", className ?? ""].filter(Boolean).join(" ")}
      data-view={activeView}
    >
      <div className="aw-skills__toolbar">
        <div className="aw-skills__tabs" role="tablist" aria-label="Skills 视图">
          {viewOptions.map((view) => (
            <button
              key={view.id}
              type="button"
              role="tab"
              aria-selected={activeView === view.id}
              className="aw-skills__tab"
              data-active={activeView === view.id ? "true" : "false"}
              onClick={() => onViewChange(view.id)}
            >
              {view.label}
            </button>
          ))}
        </div>
        <label className="aw-skills__search">
          <Search size={15} strokeWidth={1.8} aria-hidden="true" />
          <span className="aw-skills__sr-only">搜索 Skills</span>
          <input
            value={searchValue}
            placeholder="搜索 Skills…"
            onChange={(event) => onSearchChange(event.target.value)}
          />
        </label>
      </div>

      <div className="aw-skills__content">
        {activeView === "awaken" ? (
          <SkillGrid
            skills={visibleAwakenSkills}
            emptyMessage="内置 Skills 未就绪，重启 Awaken 后重试"
            onPreview={onSkillPreview}
            onToggle={onSkillToggle}
          />
        ) : null}

        {activeView === "project" ? (
          visibleProjects.length > 0 ? (
            <div className="aw-skills__folder-grid">
              {visibleProjects.map((project) => (
                <AwakenFolder
                  key={project.id}
                  label={project.name}
                  meta={`${project.skills.length} ${project.skills.length === 1 ? "Skill" : "Skills"}`}
                  open={openProjectId === project.id}
                  onOpen={() => onProjectOpenChange(project.id)}
                />
              ))}
            </div>
          ) : (
            <div className="aw-skills__empty">暂无项目 Skills</div>
          )
        ) : null}

        {activeView === "global" ? (
          visibleGlobalGroups.length > 0 ? (
            <div className="aw-skills__sections">
              {visibleGlobalGroups.map((group) => (
                <section className="aw-skills__section" key={group.id}>
                  <header className="aw-skills__section-heading">
                    <div>
                      <h3>{group.label}</h3>
                      {group.path ? <p>{group.path}</p> : null}
                    </div>
                    <span>{group.skills.length} Skills</span>
                  </header>
                  <SkillGrid
                    skills={group.skills}
                    emptyMessage="暂无全局 Skills"
                    onPreview={onSkillPreview}
                    onToggle={onSkillToggle}
                  />
                </section>
              ))}
            </div>
          ) : (
            <div className="aw-skills__empty">暂无全局 Skills</div>
          )
        ) : null}

        {activeView === "agents" ? (
          visibleAgentGroups.length > 0 ? (
            <div className="aw-skills__sections">
              {visibleAgentGroups.map((group) => (
                <section className="aw-skills__section" key={group.id}>
                  <header className="aw-skills__section-heading">
                    <div>
                      <h3>{group.label}</h3>
                      {group.path ? <p>{group.path}</p> : null}
                    </div>
                    <span>{group.skills.length} Skills</span>
                  </header>
                  <SkillGrid
                    skills={group.skills}
                    emptyMessage="暂无 Agents Skills"
                    onPreview={onSkillPreview}
                    onToggle={onSkillToggle}
                  />
                </section>
              ))}
            </div>
          ) : (
            <div className="aw-skills__empty">暂无 Agents Skills</div>
          )
        ) : null}
      </div>

      <AwakenProjectSkillsDialog
        open={activeProject !== null}
        project={activeProject}
        onOpenChange={(open) => {
          if (!open) {
            onProjectOpenChange(null);
          }
        }}
        onSkillPreview={onSkillPreview}
        onSkillToggle={onSkillToggle}
      />
    </div>
  );
}
