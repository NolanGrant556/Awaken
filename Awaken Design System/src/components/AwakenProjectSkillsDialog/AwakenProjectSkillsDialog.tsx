import "./AwakenProjectSkillsDialog.css";
import { AwakenIcon } from "../AwakenIcon";
import {
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenPopover } from "../AwakenPopover";
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

function folderName(path: string): string {
  const normalizedPath = path.replace(/[\\/]+$/, "");
  return normalizedPath.split(/[\\/]/).filter(Boolean).at(-1) ?? path;
}

function skillBelongsToFolder(
  skill: AwakenSkillItem,
  folder: NonNullable<AwakenSkillProject["folders"]>[number],
): boolean {
  if (skill.folderPath) {
    return skill.folderPath === folder.path;
  }
  if (skill.projectId === folder.path) {
    return true;
  }
  return skill.sourceLabel === (folder.label || folderName(folder.path));
}

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
  const [folderPopoverOpen, setFolderPopoverOpen] = useState(false);
  const [selectedFolderPath, setSelectedFolderPath] = useState<string | null>(null);

  const projectFolders = project?.folders && project.folders.length > 0
    ? project.folders
    : project?.path
      ? [{ path: project.path, label: folderName(project.path), isPrimary: true }]
      : [];
  const primaryFolder = projectFolders.find((folder) => folder.isPrimary) ?? projectFolders[0];
  const selectedFolder = selectedFolderPath
    ? projectFolders.find((folder) => folder.path === selectedFolderPath) ?? null
    : null;
  const visibleSkills = selectedFolder
    ? project?.skills.filter((skill) => skillBelongsToFolder(skill, selectedFolder)) ?? []
    : project?.skills ?? [];

  useEffect(() => {
    setFolderPopoverOpen(false);
    setSelectedFolderPath(null);
  }, [detailView, open, project?.id]);

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
            {primaryFolder ? (
              <AwakenPopover
                open={folderPopoverOpen}
                onOpenChange={setFolderPopoverOpen}
                placement="bottom-start"
                offset={8}
                trigger={
                  <button
                    type="button"
                    className="aw-project-skills-dialog__folder-summary"
                    aria-label={`筛选 ${projectFolders.length} 个项目文件夹`}
                    aria-expanded={folderPopoverOpen}
                    aria-haspopup="dialog"
                  >
                    <AwakenIcon concept="folder-group" size={16} className="aw-project-skills-dialog__folder-summary-icon" />
                    <span className="aw-project-skills-dialog__folder-summary-count">
                      {projectFolders.length} 个项目文件夹
                    </span>
                    <span
                      className="aw-project-skills-dialog__folder-summary-separator"
                      aria-hidden="true"
                    >
                      ·
                    </span>
                    <span className="aw-project-skills-dialog__folder-summary-primary">
                      {selectedFolder
                        ? `当前：${selectedFolder.label || folderName(selectedFolder.path)}`
                        : `主：${primaryFolder.label || folderName(primaryFolder.path)}`}
                    </span>
                    <AwakenIcon concept="dropdown" size={14} className="aw-project-skills-dialog__folder-summary-chevron" />
                  </button>
                }
                content={
                  <section
                    className="aw-project-skills-dialog__folder-popover"
                    aria-label="项目文件夹"
                  >
                    <header className="aw-project-skills-dialog__folder-popover-heading">
                      <span>项目文件夹</span>
                      <span
                        className="aw-project-skills-dialog__folder-popover-count"
                        aria-label={`${projectFolders.length} 个文件夹`}
                      >
                        {projectFolders.length}
                      </span>
                    </header>
                    <div className="aw-project-skills-dialog__folder-list">
                      <button
                        type="button"
                        className="aw-project-skills-dialog__folder-item"
                        data-selected={selectedFolder ? "false" : "true"}
                        aria-label="查看全部文件夹的 Skills"
                        aria-pressed={!selectedFolder}
                        onClick={() => {
                          setSelectedFolderPath(null);
                          setFolderPopoverOpen(false);
                        }}
                      >
                        <span className="aw-project-skills-dialog__folder-icon">
                          <AwakenIcon concept="folder-group" size={16} />
                        </span>
                        <span className="aw-project-skills-dialog__folder-copy">
                          <span className="aw-project-skills-dialog__folder-name-row">
                            <span className="aw-project-skills-dialog__folder-name">
                              全部文件夹
                            </span>
                          </span>
                        </span>
                        <span className="aw-project-skills-dialog__folder-meta">
                          <span>{project.skills.length}</span>
                          {!selectedFolder ? (
                            <AwakenIcon concept="step-done" size={14} />
                          ) : null}
                        </span>
                      </button>
                      {projectFolders.map((folder) => {
                        const label = folder.label || folderName(folder.path);
                        const folderSkillCount = project.skills.filter((skill) =>
                          skillBelongsToFolder(skill, folder)
                        ).length;
                        const selected = selectedFolder?.path === folder.path;
                        return (
                          <button
                            type="button"
                            key={folder.path}
                            className="aw-project-skills-dialog__folder-item"
                            data-primary={folder.isPrimary ? "true" : "false"}
                            data-selected={selected ? "true" : "false"}
                            aria-label={`查看 ${label} 的 Skills`}
                            aria-pressed={selected}
                            onClick={() => {
                              setSelectedFolderPath(folder.path);
                              setFolderPopoverOpen(false);
                            }}
                          >
                            <span className="aw-project-skills-dialog__folder-icon">
                              <AwakenIcon concept="project" size={16} />
                            </span>
                            <span className="aw-project-skills-dialog__folder-copy">
                              <span className="aw-project-skills-dialog__folder-name-row">
                                <span className="aw-project-skills-dialog__folder-name">
                                  {label}
                                </span>
                                {folder.isPrimary ? (
                                  <span className="aw-project-skills-dialog__primary-badge">
                                    主文件夹
                                  </span>
                                ) : null}
                              </span>
                            </span>
                            <span className="aw-project-skills-dialog__folder-meta">
                              <span>{folderSkillCount}</span>
                              {selected ? (
                                <AwakenIcon concept="step-done" size={14} />
                              ) : null}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </section>
                }
              />
            ) : null}
            <span className="aw-project-skills-dialog__count">
              {visibleSkills.length} Skills
            </span>
          </div>
          {visibleSkills.length > 0 ? (
            <div className="aw-project-skills-dialog__grid">
              {visibleSkills.map((skill) => (
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
            <div className="aw-project-skills-dialog__empty">
              {selectedFolder ? "这个文件夹还没有 Skills" : "这个项目还没有 Skills"}
            </div>
          )}
        </div>
      ) : null}
    </AwakenDialog>
  );
}
