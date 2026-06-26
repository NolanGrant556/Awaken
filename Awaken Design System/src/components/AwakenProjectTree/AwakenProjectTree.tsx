import "./AwakenProjectTree.css";
import { useState, type CSSProperties, type MouseEvent } from "react";
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

  const rowHeight = depth === 0 ? 32 : 28;
  const fontSize =
    depth === 0 ? "var(--aw-font-size-sm)" : "var(--aw-font-size-xs)";
  const lineHeight =
    depth === 0 ? "var(--aw-line-height-sm)" : "var(--aw-line-height-xs)";
  const idleColor =
    depth === 0 ? "var(--aw-text-secondary)" : "var(--aw-text-tertiary)";
  const baseColor = isActive ? "var(--aw-text-primary)" : idleColor;
  const baseBg = isActive ? "var(--aw-bg-fill-dark)" : "transparent";

  const rowFrameStyle: CSSProperties = {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto auto",
    alignItems: "center",
    width: "100%",
    height: rowHeight,
    gap: 3,
    border: 0,
    borderRadius: "var(--aw-radius-sm)",
    background: baseBg,
    color: baseColor,
    transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast)",
  };

  const rowButtonStyle: CSSProperties = {
    display: "grid",
    gridTemplateColumns: "20px minmax(0, 1fr) auto",
    alignItems: "center",
    width: "100%",
    height: rowHeight,
    minWidth: 0,
    gap: 9,
    padding: "0 0 0 10px",
    border: 0,
    borderRadius: "var(--aw-radius-sm)",
    background: "transparent",
    color: "inherit",
    fontSize,
    lineHeight,
    textAlign: "left",
    cursor: isDisabled ? "not-allowed" : "pointer",
    opacity: isDisabled ? 0.6 : 1,
  };

  const utilityButtonStyle: CSSProperties = {
    display: "inline-grid",
    placeItems: "center",
    width: 22,
    height: 22,
    padding: 0,
    border: 0,
    borderRadius: "var(--aw-radius-sm)",
    background: "transparent",
    color: "var(--aw-text-tertiary)",
    cursor: isDisabled ? "not-allowed" : "pointer",
    opacity: isDisabled ? 0.6 : 1,
  };

  const labelStyle: CSSProperties = {
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  };

  const timeStyle: CSSProperties = {
    color: "var(--aw-text-tertiary)",
    fontSize: "var(--aw-font-size-xs)",
    lineHeight: "var(--aw-line-height-xs)",
    whiteSpace: "nowrap",
  };

  const handleEnter = (e: MouseEvent<HTMLDivElement>) => {
    if (!isDisabled) {
      e.currentTarget.style.background = isActive
        ? "var(--aw-bg-fill-dark)"
        : "var(--aw-bg-fill-light)";
    }
  };
  const handleLeave = (e: MouseEvent<HTMLDivElement>) => {
    e.currentTarget.style.background = baseBg;
  };
  const handleDown = (e: MouseEvent<HTMLDivElement>) => {
    if (!isDisabled) e.currentTarget.style.background = "var(--aw-bg-fill-dark)";
  };
  const handleUp = (e: MouseEvent<HTMLDivElement>) => {
    if (!isDisabled) {
      e.currentTarget.style.background = isActive
        ? "var(--aw-bg-fill-dark)"
        : "var(--aw-bg-fill-light)";
    }
  };

  const handleClick = () => {
    if (isDisabled) return;
    onSelect?.(item.id);
  };

  const handleToggle = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    if (isDisabled || !hasChildren) return;
    onToggle(item.id);
  };

  const childrenContainerStyle: CSSProperties = {
    display: "grid",
    gap: 1,
    padding: "3px 0 4px 24px",
  };

  return (
    <div className="aw-project-group">
      <div
        style={rowFrameStyle}
        onMouseEnter={handleEnter}
        onMouseLeave={handleLeave}
        onMouseDown={handleDown}
        onMouseUp={handleUp}
      >
        <button
          type="button"
          disabled={isDisabled}
          onClick={handleClick}
          title={item.path ?? item.label}
          aria-current={isActive ? "true" : undefined}
          style={rowButtonStyle}
        >
          <span style={{ display: "inline-flex", width: 20, height: 20, alignItems: "center", justifyContent: "center", flexShrink: 0, color: "var(--aw-text-tertiary)" }}>
            {kind === "project" ? (
              <AwakenIcon size={16}>
                <path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h4" />
              </AwakenIcon>
            ) : null}
          </span>
          <span style={labelStyle}>{item.label}</span>
          {item.timeLabel ? <span style={timeStyle}>{item.timeLabel}</span> : <span />}
        </button>
        {item.action ? (
          <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center" }}>
            {item.action}
          </span>
        ) : null}
        <span style={{ display: "inline-flex", width: 22, height: 22, alignItems: "center", justifyContent: "center", flexShrink: 0 }}>
          {isLoading ? (
            <span style={spinnerStyle} aria-label="加载中" />
          ) : hasChildren ? (
            <button
              type="button"
              aria-label={isExpanded ? `折叠 ${item.label}` : `展开 ${item.label}`}
              aria-expanded={isExpanded}
              disabled={isDisabled}
              style={utilityButtonStyle}
              onClick={handleToggle}
            >
            <span
              style={{
                display: "inline-flex",
                color: "inherit",
                transform: isExpanded ? "rotate(90deg)" : "none",
                transition: "transform 140ms ease",
              }}
            >
              <AwakenIcon size={14}>
                <path d="M9 6l6 6-6 6" />
              </AwakenIcon>
            </span>
            </button>
          ) : null}
        </span>
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
