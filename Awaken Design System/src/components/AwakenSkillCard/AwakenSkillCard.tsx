import "./AwakenSkillCard.css";
import { FileText, Wrench } from "lucide-react";
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
  const previewHint = previewable ? "可预览 SKILL.md" : "当前来源不提供预览";
  const cardBody = (
    <>
      <span className="aw-skill-card__icon" aria-hidden="true">
        <Wrench size={20} strokeWidth={1.8} />
      </span>
      <span className="aw-skill-card__copy">
        <span className="aw-skill-card__name">{skill.name}</span>
        <span className="aw-skill-card__description">{skill.description}</span>
        {skill.diagnostic ? (
          <span className="aw-skill-card__diagnostic">{skill.diagnostic}</span>
        ) : (
          <span className="aw-skill-card__meta">
            {skill.sourceLabel ? <span>{skill.sourceLabel}</span> : null}
            <span className="aw-skill-card__preview-hint">
              <FileText size={12} strokeWidth={1.8} aria-hidden="true" />
              {previewHint}
            </span>
          </span>
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
          aria-label={`查看 ${skill.name} 的 SKILL.md`}
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
          disabled={skill.toggleDisabled || !onEnabledChange}
          onCheckedChange={onEnabledChange}
        />
      </div>
    </article>
  );
}
