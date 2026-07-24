import "./AwakenProjectSkillsDialog.css";
import {
  useEffect,
  useRef,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenSkillCard } from "../AwakenSkillCard";
import type {
  AwakenSkillItem,
  AwakenSkillProject,
} from "../AwakenSkills/AwakenSkills.types";

export type AwakenProjectSkillsDialogDetailView = {
  mode: "preview" | "delete";
  title: ReactNode;
  titleVisuallyHidden?: boolean;
  bodyClassName?: string;
  bodyRef?: Ref<HTMLDivElement>;
  bodyTabIndex?: number;
  initialFocusRef?: RefObject<HTMLElement | null>;
  content: ReactNode;
  footer?: ReactNode;
  returnFocusSkillId?: string;
};

export type AwakenProjectSkillsDialogProps = {
  open: boolean;
  project: AwakenSkillProject | null;
  onOpenChange: (open: boolean) => void;
  onSkillPreview: (skill: AwakenSkillItem) => void;
  onSkillToggle: (skill: AwakenSkillItem, enabled: boolean) => void;
  detailView?: AwakenProjectSkillsDialogDetailView | null;
  onDetailClose?: () => void;
};

export function AwakenProjectSkillsDialog({
  open,
  project,
  onOpenChange,
  onSkillPreview,
  onSkillToggle,
  detailView = null,
  onDetailClose,
}: AwakenProjectSkillsDialogProps) {
  const listRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<string | null>(null);

  useEffect(() => {
    if (detailView?.returnFocusSkillId) {
      returnFocusRef.current = detailView.returnFocusSkillId;
      return;
    }
    if (detailView || !returnFocusRef.current) {
      return;
    }
    const target = returnFocusRef.current;
    returnFocusRef.current = null;
    const card = Array.from(
      listRef.current?.querySelectorAll<HTMLElement>("[data-skill-id]") ?? [],
    ).find((element) => element.dataset.skillId === target);
    card?.querySelector<HTMLElement>(".aw-skill-card__main")?.focus();
  }, [detailView, detailView?.returnFocusSkillId]);

  return (
    <AwakenDialog
      open={open && project !== null}
      title={detailView?.title ?? (project ? `${project.name} Skills` : "项目 Skills")}
      titleVisuallyHidden={detailView?.titleVisuallyHidden}
      size="large"
      className={[
        "aw-project-skills-dialog",
        detailView ? `aw-project-skills-dialog--${detailView.mode}` : "",
      ].filter(Boolean).join(" ")}
      bodyClassName={[
        detailView?.mode === "preview" ? "aw-skill-preview__body" : "",
        detailView?.bodyClassName ?? "",
      ].filter(Boolean).join(" ") || undefined}
      bodyRef={detailView?.bodyRef}
      bodyTabIndex={detailView?.bodyTabIndex}
      initialFocusRef={detailView?.initialFocusRef}
      footer={detailView?.footer}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && detailView) {
          onDetailClose?.();
          return;
        }
        onOpenChange(nextOpen);
      }}
    >
      {detailView ? detailView.content : project ? (
        <div ref={listRef} className="aw-project-skills-dialog__content">
          <div className="aw-project-skills-dialog__heading">
            {project.path ? (
              <span className="aw-project-skills-dialog__path">{project.path}</span>
            ) : null}
            <span className="aw-project-skills-dialog__count">
              {project.skills.length} Skills
            </span>
          </div>
          {project.skills.length > 0 ? (
            <div className="aw-project-skills-dialog__grid">
              {project.skills.map((skill) => (
                <AwakenSkillCard
                  key={skill.id}
                  data-skill-id={skill.id}
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
