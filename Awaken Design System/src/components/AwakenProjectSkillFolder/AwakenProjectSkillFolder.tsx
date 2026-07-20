import "./AwakenProjectSkillFolder.css";
import type { ButtonHTMLAttributes } from "react";

export type AwakenProjectSkillFolderProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "children" | "title"
> & {
  projectName: string;
  skillCount?: number;
  open?: boolean;
  onOpen?: () => void;
};

export function AwakenProjectSkillFolder({
  projectName,
  skillCount,
  open = false,
  onOpen,
  disabled,
  className,
  onClick,
  "aria-label": ariaLabel,
  "aria-expanded": ariaExpanded,
  ...props
}: AwakenProjectSkillFolderProps) {
  const countLabel = skillCount === undefined
    ? undefined
    : `${skillCount} ${skillCount === 1 ? "Skill" : "Skills"}`;

  return (
    <button
      type="button"
      aria-label={ariaLabel ?? `打开 ${projectName} 的项目 Skills`}
      aria-expanded={ariaExpanded ?? open}
      data-open={open ? "true" : "false"}
      disabled={disabled}
      {...props}
      className={[
        "aw-project-skill-folder",
        className ?? "",
      ].filter(Boolean).join(" ")}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && !disabled) {
          onOpen?.();
        }
      }}
    >
      <span className="aw-project-skill-folder__stage">
        <span
          className="aw-project-skill-folder__layer aw-project-skill-folder__layer--back"
          aria-hidden="true"
        />
        <span
          className="aw-project-skill-folder__layer aw-project-skill-folder__layer--paper-rear"
          aria-hidden="true"
        />
        <span
          className="aw-project-skill-folder__layer aw-project-skill-folder__layer--paper-front"
          aria-hidden="true"
        >
          <span className="aw-project-skill-folder__sheet-lines" />
        </span>
        <span className="aw-project-skill-folder__face">
          <span className="aw-project-skill-folder__copy">
            <span className="aw-project-skill-folder__name">{projectName}</span>
            {countLabel ? (
              <span className="aw-project-skill-folder__count">{countLabel}</span>
            ) : null}
          </span>
        </span>
      </span>
    </button>
  );
}
