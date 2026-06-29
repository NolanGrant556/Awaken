import "./AwakenProjectTree.css";
import { useState, type CSSProperties } from "react";
import type { ProjectItem } from "../../types";
import { AwakenIcon } from "../AwakenIcon";

export type AwakenProjectTreeProps = {
  items: ProjectItem[];
  activeId?: string;
  expandedIds?: string[];
  defaultExpandedIds?: string[];
  onExpandedChange?: (ids: string[]) => void;
  onSelect?: (id: string) => void;
};

const listStyle: CSSProperties = {
  display: "grid",
  gap: 3,
};

const emptyStyle: CSSProperties = {
  padding: "10px 12px",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
};

const spinnerStyle: CSSProperties = {
  display: "inline-block",
  width: 14,
  height: 14,
  border: "2px solid var(--aw-border-main)",
  borderTopColor: "var(--aw-text-tertiary)",
  borderRadius: "999px",
  animation: "spin 900ms linear infinite",
  flexShrink: 0,
};

export function AwakenProjectTree({
  items,
  activeId,
  expandedIds,
  defaultExpandedIds = [],
  onExpandedChange,
  onSelect,
}: AwakenProjectTreeProps) {
  const [internalExpanded, setInternalExpanded] = useState<string[]>(defaultExpandedIds);
  const isControlled = expandedIds !== undefined;
  const expanded = isControlled ? expandedIds : internalExpanded;

  const setExpanded = (next: string[]) => {
    if (!isControlled) setInternalExpanded(next);
    onExpandedChange?.(next);
  };

  const toggle = (id: string) => {
    setExpanded(
      expanded.includes(id) ? expanded.filter((x) => x !== id) : [...expanded, id],
    );
  };

  if (!items || items.length === 0) {
    return <div style={emptyStyle}>暂无项目</div>;
  }

  return (
    <div className="aw-project-list" style={listStyle}>
      {items.map((item) => (
        <ProjectNode
          key={item.id}
          item={item}
          depth={0}
          activeId={activeId}
          expanded={expanded}
          onToggle={toggle}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

type NodeProps = {
  item: ProjectItem;
  depth: number;
  activeId?: string;
  expanded: string[];
  onToggle: (id: string) => void;
  onSelect?: (id: string) => void;
};

function ProjectNode({ item, depth, activeId, expanded, onToggle, onSelect }: NodeProps) {
  const hasChildren = !!item.children && item.children.length > 0;
  const isExpanded = expanded.includes(item.id);
  const isActive = item.id === activeId;
  const isDisabled = item.disabled;
  const isLoading = item.isLoading;
  const kind = item.kind ?? (depth === 0 || hasChildren ? "project" : "thread");
  const isProject = kind === "project";
  const showTimeLabel = !isProject && item.timeLabel;
  const showInlineChevron = isProject && hasChildren;

  const rowHeight = depth === 0 ? 32 : 28;
  const fontSize =
    depth === 0 ? "var(--aw-font-size-sm)" : "var(--aw-font-size-xs)";
  const lineHeight =
    depth === 0 ? "var(--aw-line-height-sm)" : "var(--aw-line-height-xs)";
  const idleColor =
    depth === 0 ? "var(--aw-text-secondary)" : "var(--aw-text-tertiary)";
  const baseColor = isActive ? "var(--aw-text-primary)" : idleColor;
  const showDescription = !isProject && item.description;

  const nodeClassName = [
    "aw-project-node",
    `aw-project-node--${kind}`,
    hasChildren ? "aw-project-node--has-children" : "",
    isExpanded ? "aw-project-node--expanded" : "",
    isActive ? "aw-project-node--active" : "",
    isDisabled ? "aw-project-node--disabled" : "",
  ].filter(Boolean).join(" ");

  const rowStyle = {
    "--aw-project-row-height": showDescription ? "auto" : `${rowHeight}px`,
    "--aw-project-row-font-size": fontSize,
    "--aw-project-row-line-height": lineHeight,
    "--aw-project-row-color": baseColor,
  } as CSSProperties;

  const rowButtonStyle: CSSProperties = {
    fontSize,
    lineHeight,
    cursor: isDisabled ? "not-allowed" : "pointer",
    opacity: isDisabled ? 0.6 : 1,
    alignItems: showDescription ? "flex-start" : "center",
    padding: showDescription ? "3px 0 3px 10px" : undefined,
  };

  const handleClick = () => {
    if (isDisabled) return;
    if (isProject && hasChildren) {
      onToggle(item.id);
    }
    onSelect?.(item.id);
  };

  const childrenContainerStyle: CSSProperties = {
    display: "grid",
    gap: 1,
    padding: "3px 0 4px 24px",
  };

  return (
    <div className={nodeClassName}>
      <div className="aw-project-row" style={rowStyle}>
        <button
          type="button"
          className="aw-project-row-main"
          disabled={isDisabled}
          onClick={handleClick}
          title={item.path ?? item.label}
          aria-current={isActive ? "true" : undefined}
          aria-expanded={hasChildren ? isExpanded : undefined}
          style={rowButtonStyle}
        >
          <span className="aw-project-row-icon">
            {isProject ? (
              <AwakenIcon size={16}>
                <g className="aw-project-folder-icon" data-expanded={isExpanded ? "true" : "false"}>
                  <path d="M3 8.5h7l2 2h9v7.5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                  <path
                    className="aw-project-folder-icon__lid"
                    d="M3 8.5V6a2 2 0 0 1 2-2h4l2 2h4"
                  />
                </g>
              </AwakenIcon>
            ) : null}
          </span>
          <span className="aw-project-row-text">
            <span className="aw-project-row-label">{item.label}</span>
            {showDescription ? (
              <span className="aw-project-row-description">{item.description}</span>
            ) : null}
          </span>
          {showInlineChevron ? (
            <span className="aw-project-inline-chevron" aria-hidden="true">
              <AwakenIcon size={14}>
                <path d="M9 6l6 6-6 6" />
              </AwakenIcon>
            </span>
          ) : null}
          {isLoading ? <span style={spinnerStyle} aria-label="加载中" /> : null}
          {showTimeLabel ? <span className="aw-project-row-meta">{item.timeLabel}</span> : null}
        </button>
        {item.action ? (
          <span className="aw-project-row-actions">
            {item.action}
          </span>
        ) : null}
      </div>

      {hasChildren && isExpanded ? (
        <div style={childrenContainerStyle}>
          {item.children!.map((child) => (
            <ProjectNode
              key={child.id}
              item={child}
              depth={depth + 1}
              activeId={activeId}
              expanded={expanded}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
