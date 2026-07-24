import type { ButtonHTMLAttributes } from "react";
import { AwakenFolder } from "../AwakenFolder";

export type AwakenProjectSkillFolderProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "children" | "title"
> & {
  projectName: string;
  skillCount?: number;
  countLabel?: string;
  size?: "default" | "compact";
  open?: boolean;
  onOpen?: () => void;
};

export function AwakenProjectSkillFolder({
  projectName,
  skillCount,
  countLabel,
  size = "default",
  open = false,
  onOpen,
  disabled,
  className,
  onClick,
  "aria-label": ariaLabel,
  "aria-expanded": ariaExpanded,
  ...props
}: AwakenProjectSkillFolderProps) {
  const resolvedCountLabel = countLabel ?? (skillCount === undefined
    ? undefined
    : `${skillCount} ${skillCount === 1 ? "Skill" : "Skills"}`);

  return <AwakenFolder label={projectName} meta={resolvedCountLabel} size={size} open={open} showNativeTitle={false} onOpen={onOpen} disabled={disabled} className={className} onClick={onClick} aria-label={ariaLabel ?? `打开 ${projectName} 的项目 Skills`} aria-expanded={ariaExpanded ?? open} {...props} />;
}
