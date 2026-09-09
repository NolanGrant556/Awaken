import { useMemo, useRef, useState } from "react";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenPopover } from "../AwakenPopover";
import type { AwakenScheduledTaskProjectOption } from "./AwakenScheduledTaskEditorDialog";

export type AwakenScheduledTaskProjectPickerProps = {
  value: string;
  projects: AwakenScheduledTaskProjectOption[];
  invalid?: boolean;
  disabled?: boolean;
  onChange: (projectId: string) => void;
};

export function AwakenScheduledTaskProjectPicker({
  value,
  projects,
  invalid,
  disabled,
  onChange,
}: AwakenScheduledTaskProjectPickerProps) {
  const searchRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selected = projects.find((project) => project.projectId === value);
  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const filteredProjects = useMemo(
    () => normalizedQuery
      ? projects.filter((project) =>
          `${project.name} ${project.projectId}`
            .toLocaleLowerCase("zh-CN")
            .includes(normalizedQuery),
        )
      : projects,
    [normalizedQuery, projects],
  );

  return (
    <AwakenPopover
      viewportBounded
      initialFocusRef={searchRef}
      open={open}
      onOpenChange={(nextOpen) => {
        if (disabled) return;
        setOpen(nextOpen);
        if (nextOpen) {
          setQuery("");
        }
      }}
      placement="bottom-start"
      offset={6}
      matchTriggerWidth
      trigger={
        <button
          type="button"
          className="aw-scheduled-task-editor__picker-trigger aw-scheduled-task-editor__project-trigger"
          aria-label="运行项目"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          disabled={disabled}
        >
          <AwakenIcon concept="project" size={16} />
          <span data-placeholder={selected ? undefined : "true"}>
            <strong>{selected?.name ?? (value ? "当前项目不可用" : "请选择项目")}</strong>
            <small>{selected ? "任务将在此项目中运行" : "选择任务的运行位置"}</small>
          </span>
          <AwakenIcon concept="dropdown" size={16} />
        </button>
      }
      content={
        <div
          className="aw-scheduled-task-editor__project-popover"
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              setOpen(false);
            }
          }}
        >
          <div className="aw-scheduled-task-editor__project-popover-head">
            <strong>选择项目</strong>
            <span>{projects.length} 个项目</span>
          </div>
          <label className="aw-scheduled-task-editor__project-search">
            <AwakenIcon concept="search" size={16} />
            <input
              ref={searchRef}
              type="search"
              aria-label="搜索项目"
              placeholder="搜索项目"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "ArrowDown") {
                  event.preventDefault();
                  event.currentTarget
                    .closest(".aw-scheduled-task-editor__project-popover")
                    ?.querySelector<HTMLButtonElement>(
                      ".aw-scheduled-task-editor__project-list button",
                    )
                    ?.focus();
                }
              }}
            />
          </label>
          <div
            className="aw-scheduled-task-editor__project-list"
            role="listbox"
            aria-label="运行项目列表"
            onKeyDown={(event) => {
              if (!(event.target instanceof HTMLButtonElement)) return;
              if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
              const buttons = Array.from(
                event.currentTarget.querySelectorAll<HTMLButtonElement>("button"),
              );
              const currentIndex = buttons.indexOf(event.target);
              const nextIndex = event.key === "Home"
                ? 0
                : event.key === "End"
                  ? buttons.length - 1
                  : Math.min(
                      buttons.length - 1,
                      Math.max(0, currentIndex + (event.key === "ArrowDown" ? 1 : -1)),
                    );
              event.preventDefault();
              buttons[nextIndex]?.focus();
            }}
          >
            {filteredProjects.map((project) => (
              <button
                type="button"
                role="option"
                aria-selected={value === project.projectId}
                key={project.projectId}
                onClick={() => {
                  onChange(project.projectId);
                  setOpen(false);
                }}
              >
                <span className="aw-scheduled-task-editor__project-option-icon">
                  <AwakenIcon concept="project" size={16} />
                </span>
                <span>{project.name}</span>
                {value === project.projectId ? (
                  <AwakenIcon concept="step-done" size={16} />
                ) : null}
              </button>
            ))}
            {filteredProjects.length === 0 ? (
              <p className="aw-scheduled-task-editor__project-empty">没有匹配的项目</p>
            ) : null}
          </div>
        </div>
      }
    />
  );
}
