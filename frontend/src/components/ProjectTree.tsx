import { Folder, FolderCog } from "lucide-react";
import type { ProjectItem } from "../types";

type ProjectTreeProps = {
  items: ProjectItem[];
  activeItemId?: string;
  onSelectItem?: (id: string) => void;
};

export function ProjectTree({ items, activeItemId, onSelectItem }: ProjectTreeProps) {
  return (
    <div className="sidebar-section project-section">
      <div className="section-title">项目</div>
      <div className="project-tree">
        {items.map((item) => (
          <ProjectNode key={item.id} item={item} activeItemId={activeItemId} onSelectItem={onSelectItem} />
        ))}
      </div>
    </div>
  );
}

type ProjectNodeProps = {
  item: ProjectItem;
  activeItemId?: string;
  onSelectItem?: (id: string) => void;
};

function ProjectNode({ item, activeItemId, onSelectItem }: ProjectNodeProps) {
  const hasChildren = Boolean(item.children?.length);

  return (
    <div className="project-node">
      <button
        className={`project-row ${activeItemId === item.id ? "is-active" : ""} ${hasChildren ? "is-folder" : ""}`}
        onClick={() => onSelectItem?.(item.id)}
      >
        <span className="project-label">
          {hasChildren ? <Folder size={18} /> : <FolderCog size={18} />}
          <span>{item.label}</span>
        </span>
        {item.timeLabel ? <span className="row-time">{item.timeLabel}</span> : null}
        {item.isLoading ? <span className="loading-ring" aria-label="加载中" /> : null}
      </button>
      {item.children?.length ? (
        <div className="project-children">
          {item.children.map((child) => (
            <button
              key={child.id}
              className={`project-row project-child ${activeItemId === child.id ? "is-active" : ""}`}
              onClick={() => onSelectItem?.(child.id)}
            >
              <span className="child-label">{child.label}</span>
              {child.timeLabel ? <span className="row-time">{child.timeLabel}</span> : null}
              {child.isLoading ? <span className="loading-ring" aria-label="加载中" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
