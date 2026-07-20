import "./AwakenProjectSkillsDialog.css";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenSkillCard } from "../AwakenSkillCard";
import type {
  AwakenSkillItem,
  AwakenSkillProject,
} from "../AwakenSkills/AwakenSkills.types";

export type AwakenProjectSkillsDialogProps = {
  open: boolean;
  project: AwakenSkillProject | null;
  onOpenChange: (open: boolean) => void;
  onSkillPreview: (skill: AwakenSkillItem) => void;
  onSkillToggle: (skill: AwakenSkillItem, enabled: boolean) => void;
};

export function AwakenProjectSkillsDialog({
  open,
  project,
  onOpenChange,
  onSkillPreview,
  onSkillToggle,
}: AwakenProjectSkillsDialogProps) {
  return (
    <AwakenDialog
      open={open && project !== null}
      title={project ? `${project.name} Skills` : "项目 Skills"}
      size="large"
      className="aw-project-skills-dialog"
      onOpenChange={onOpenChange}
    >
      {project ? (
        <div className="aw-project-skills-dialog__content">
          <div className="aw-project-skills-dialog__heading">
            <div>
              <strong>{project.name}</strong>
              {project.path ? <span>{project.path}</span> : null}
            </div>
            <span>{project.skills.length} Skills</span>
          </div>
          {project.skills.length > 0 ? (
            <div className="aw-project-skills-dialog__grid">
              {project.skills.map((skill) => (
                <AwakenSkillCard
                  key={skill.id}
                  skill={skill}
                  onPreview={(skill.previewable ?? true) ? () => onSkillPreview(skill) : undefined}
                  onEnabledChange={(enabled) => onSkillToggle(skill, enabled)}
                />
              ))}
            </div>
          ) : (
            <div className="aw-project-skills-dialog__empty">这个项目还没有 Skills</div>
          )}
        </div>
      ) : null}
    </AwakenDialog>
  );
}
