import "./AwakenProjectFoldersEditor.css";
import { AwakenIcon } from "../AwakenIcon";
import { useState } from "react";
import { AwakenDropdown } from "../AwakenDropdown";
import type { ProjectFolderRow, ProjectFolderRowAction } from "../../types";

export type AwakenProjectFoldersEditorProps = {
  folders: ProjectFolderRow[];
  addLabel?: string;
  addDisabled?: boolean;
  emptyHint?: string;
  primaryBadgeLabel?: string;
  makePrimaryLabel?: string;
  removeLabel?: string;
  busyPath?: string;
  onAddFolder?: () => void;
};

// 路径来自用户机器，Windows 上分隔符是反斜杠，只切 `/` 会把整条路径当成末段名字。
// 切不出任何一段时（`/` 或 `C:\` 这类根目录）退回原路径，不返回空字符串。
function folderName(path: string): string {
  const segments = path.split(/[\\/]+/).filter(Boolean);
  return segments[segments.length - 1] ?? path;
}

function FolderRow({
  folder,
  primaryBadgeLabel,
  makePrimaryLabel,
  removeLabel,
  busy,
}: {
  folder: ProjectFolderRow;
  primaryBadgeLabel: string;
  makePrimaryLabel: string;
  removeLabel: string;
  busy: boolean;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const actions = folder.actions ?? [];
  const label = folder.label ?? folderName(folder.path);

  return (
    <li
      className="aw-project-folders-editor__row"
      data-primary={folder.isPrimary ? "true" : "false"}
      data-busy={busy ? "true" : "false"}
    >
      <span className="aw-project-folders-editor__row-icon" aria-hidden="true">
        <AwakenIcon concept="project" size={16} />
      </span>
      {/* 只展示末段名字；完整路径留在原生 title 上，用于区分同名文件夹。 */}
      <span className="aw-project-folders-editor__row-label" title={folder.path}>
        {label}
      </span>
      <span className="aw-project-folders-editor__row-actions">
        {folder.isPrimary ? (
          <span className="aw-project-folders-editor__badge">{primaryBadgeLabel}</span>
        ) : folder.onMakePrimary ? (
          <button
            type="button"
            className="aw-project-folders-editor__make-primary"
            disabled={busy}
            onClick={() => folder.onMakePrimary?.(folder.path)}
          >
            {makePrimaryLabel}
          </button>
        ) : null}
        {actions.length > 0 ? (
          <AwakenDropdown
            open={menuOpen}
            onOpenChange={setMenuOpen}
            placement="bottom-end"
            offset={4}
            trigger={
              <button
                type="button"
                className="aw-project-folders-editor__row-menu-trigger"
                aria-label={`${label} 更多操作`}
                title={`${label} 更多操作`}
                disabled={busy}
              >
                <AwakenIcon concept="more-actions" size={16} />
              </button>
            }
          >
            <div className="aw-project-folders-editor__menu" role="menu">
              {actions.map((action: ProjectFolderRowAction) => (
                <button
                  key={action.id}
                  type="button"
                  role="menuitem"
                  className={[
                    "aw-project-folders-editor__menu-item",
                    action.tone === "danger"
                      ? "aw-project-folders-editor__menu-item--danger"
                      : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                  disabled={action.disabled || busy}
                  onClick={() => {
                    setMenuOpen(false);
                    action.onSelect?.(folder.path);
                  }}
                >
                  {action.label}
                </button>
              ))}
            </div>
          </AwakenDropdown>
        ) : null}
        {folder.onRemove ? (
          <button
            type="button"
            className="aw-project-folders-editor__remove"
            aria-label={`${removeLabel}：${label}`}
            title={removeLabel}
            disabled={busy}
            onClick={() => folder.onRemove?.(folder.path)}
          >
            <AwakenIcon concept="close" size={16} />
          </button>
        ) : null}
      </span>
    </li>
  );
}

export function AwakenProjectFoldersEditor({
  folders,
  addLabel = "添加文件夹",
  addDisabled = false,
  emptyHint,
  primaryBadgeLabel = "主文件夹",
  makePrimaryLabel = "设为主文件夹",
  removeLabel = "移出列表",
  busyPath,
  onAddFolder,
}: AwakenProjectFoldersEditorProps) {
  return (
    <div className="aw-project-folders-editor">
      {folders.length === 0 && emptyHint ? (
        <p className="aw-project-folders-editor__empty">{emptyHint}</p>
      ) : null}
      {folders.length > 0 ? (
        <ul className="aw-project-folders-editor__list">
          {folders.map((folder) => (
            <FolderRow
              key={folder.path}
              folder={folder}
              primaryBadgeLabel={primaryBadgeLabel}
              makePrimaryLabel={makePrimaryLabel}
              removeLabel={removeLabel}
              busy={busyPath === folder.path}
            />
          ))}
        </ul>
      ) : null}
      {onAddFolder ? (
        <button
          type="button"
          className="aw-project-folders-editor__add"
          disabled={addDisabled}
          onClick={onAddFolder}
        >
          <span className="aw-project-folders-editor__add-icon" aria-hidden="true">
            <AwakenIcon concept="add" size={16} />
          </span>
          <span>{addLabel}</span>
        </button>
      ) : null}
    </div>
  );
}
