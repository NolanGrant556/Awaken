import "./AwakenSkillCard.css";
import { AwakenIcon } from "../AwakenIcon";
import type { HTMLAttributes } from "react";
import { AwakenSwitch } from "../AwakenSwitch";
import type { AwakenSkillItem } from "../AwakenSkills/AwakenSkills.types";

export type AwakenSkillCardProps = Omit<
  HTMLAttributes<HTMLElement>,
  "children" | "onClick" | "onToggle"
> & {
  skill: AwakenSkillItem;
  onPreview?: () => void;
  onEnabledChange?: (enabled: boolean) => void;
};

export function AwakenSkillCard({
  skill,
  onPreview,
  onEnabledChange,
  className,
  ...props
}: AwakenSkillCardProps) {
  const previewable = (skill.previewable ?? true) && Boolean(onPreview);
  const cardBody = (
    <>
      <span className="aw-skill-card__icon" aria-hidden="true">
        <AwakenIcon concept="skill" size={20} />
      </span>
      <span className="aw-skill-card__copy">
        <span className="aw-skill-card__title-row">
          <span className="aw-skill-card__name">{skill.name}</span>
          {skill.sourceLabel ? (
            <span className="aw-skill-card__source">{skill.sourceLabel}</span>
          ) : null}
        </span>
        {skill.diagnostic ? (
          <span className="aw-skill-card__diagnostic">{skill.diagnostic}</span>
        ) : (
          <span className="aw-skill-card__description">{skill.description}</span>
        )}
      </span>
    </>
  );

  return (
    <article
      {...props}
      className={["aw-skill-card", className ?? ""].filter(Boolean).join(" ")}
      data-enabled={skill.enabled ? "true" : "false"}
      data-previewable={previewable ? "true" : "false"}
    >
      {previewable ? (
        <button
          type="button"
          className="aw-skill-card__main"
          aria-label={`查看 ${skill.name} 的 Skill 包`}
          onClick={onPreview}
        >
          {cardBody}
        </button>
      ) : (
        <div className="aw-skill-card__main aw-skill-card__main--static">
          {cardBody}
        </div>
      )}
      <div className="aw-skill-card__actions">
        <AwakenSwitch
          checked={skill.enabled}
          label={skill.name}
          busy={skill.busy}
          size="compact"
          disabled={skill.toggleDisabled || !onEnabledChange}
          onCheckedChange={onEnabledChange}
        />
      </div>
    </article>
  );
}
