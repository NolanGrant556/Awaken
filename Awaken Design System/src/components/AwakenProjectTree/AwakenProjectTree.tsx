import "./AwakenProjectTree.css";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { createPortal } from "react-dom";
import {
  Check,
  ChevronRight,
  Folder,
  FolderOpen,
  MessageSquare,
  MoreHorizontal,
} from "../../icons";
import type { ProjectItem } from "../../types";
import { AwakenDropdown } from "../AwakenDropdown";
import {
  AwakenProjectHoverCard,
  type ProjectHoverCardAnchorProps,
} from "../AwakenProjectHoverCard";
import { AwakenSidebarThreadSpinner } from "../AwakenSidebarThreadSpinner";

export type AwakenProjectTreeProps = {
  items: ProjectItem[];
  activeId?: string;
  expandedIds?: string[];
  defaultExpandedIds?: string[];
  onExpandedChange?: (ids: string[]) => void;
  onSelect?: (id: string) => void;
  onReorder?: (ids: string[]) => void;
  reorderDisabled?: boolean;
  emptyLabel?: string;
};

type DropPosition = "before" | "after";

type DropTarget = {
  id: string;
  position: DropPosition;
};

type PointerSession = {
  pointerId: number;
  sourceId: string;
  startX: number;
  startY: number;
  currentX: number;
  currentY: number;
  active: boolean;
  activationTimer?: number;
};

const REORDER_HOLD_DELAY_MS = 160;
const REORDER_MOVE_THRESHOLD_PX = 6;

export function reorderProjectIds(
  ids: string[],
  sourceId: string,
  targetId: string,
  position: DropPosition,
): string[] {
  if (sourceId === targetId || !ids.includes(sourceId) || !ids.includes(targetId)) {
    return ids;
  }
  const next = ids.filter((id) => id !== sourceId);
  const targetIndex = next.indexOf(targetId);
  next.splice(position === "after" ? targetIndex + 1 : targetIndex, 0, sourceId);
  return next;
}

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

export function AwakenProjectTree({
  items,
  activeId,
  expandedIds,
  defaultExpandedIds = [],
  onExpandedChange,
  onSelect,
  onReorder,
  reorderDisabled = false,
  emptyLabel = "暂无项目",
}: AwakenProjectTreeProps) {
  const [internalExpanded, setInternalExpanded] = useState<string[]>(defaultExpandedIds);
  const [dragSourceId, setDragSourceId] = useState<string>();
  const [dragPoint, setDragPoint] = useState<{ x: number; y: number }>();
  const [dropTarget, setDropTarget] = useState<DropTarget>();
  const pointerSessionRef = useRef<PointerSession | undefined>(undefined);
  const dropTargetRef = useRef<DropTarget | undefined>(undefined);
  const suppressClickRef = useRef<string | undefined>(undefined);
  const itemsRef = useRef(items);
  const onReorderRef = useRef(onReorder);
  const reorderDisabledRef = useRef(reorderDisabled);
  const isControlled = expandedIds !== undefined;
  const expanded = isControlled ? expandedIds : internalExpanded;

  itemsRef.current = items;
  onReorderRef.current = onReorder;
  reorderDisabledRef.current = reorderDisabled;

  const setExpanded = (next: string[]) => {
    if (!isControlled) setInternalExpanded(next);
    onExpandedChange?.(next);
  };

  const toggle = (id: string) => {
    setExpanded(
      expanded.includes(id) ? expanded.filter((x) => x !== id) : [...expanded, id],
    );
  };

  const resetPointerSession = useCallback(() => {
    const session = pointerSessionRef.current;
    if (session?.activationTimer !== undefined) {
      window.clearTimeout(session.activationTimer);
    }
    pointerSessionRef.current = undefined;
    document.body.classList.remove("aw-project-reordering");
    setDragSourceId(undefined);
    setDragPoint(undefined);
    dropTargetRef.current = undefined;
    setDropTarget(undefined);
  }, []);

  const activatePointerSession = useCallback(() => {
    const session = pointerSessionRef.current;
    if (!session || session.active || reorderDisabledRef.current) return;
    session.active = true;
    suppressClickRef.current = session.sourceId;
    document.body.classList.add("aw-project-reordering");
    setDragSourceId(session.sourceId);
    setDragPoint({ x: session.currentX, y: session.currentY });
  }, []);

  useEffect(() => {
    const handlePointerMove = (event: PointerEvent) => {
      const session = pointerSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) return;
      const distance = Math.hypot(
        event.clientX - session.startX,
        event.clientY - session.startY,
      );
      session.currentX = event.clientX;
      session.currentY = event.clientY;
      if (!session.active && distance >= REORDER_MOVE_THRESHOLD_PX) {
        activatePointerSession();
      }
      if (!session.active) return;
      event.preventDefault();
      setDragPoint({ x: event.clientX, y: event.clientY });
      const row = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest<HTMLElement>(".aw-project-row[data-aw-project-id]");
      const targetId = row?.dataset.awProjectId;
      if (!row || !targetId || targetId === session.sourceId) {
        dropTargetRef.current = undefined;
        setDropTarget(undefined);
        return;
      }
      const bounds = row.getBoundingClientRect();
      const nextTarget: DropTarget = {
        id: targetId,
        position: event.clientY < bounds.top + bounds.height / 2 ? "before" : "after",
      };
      dropTargetRef.current = nextTarget;
      setDropTarget(nextTarget);
    };

    const finishPointerSession = (event: PointerEvent) => {
      const session = pointerSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) return;
      if (session.active) {
        event.preventDefault();
        const target = dropTargetRef.current;
        if (target) {
          const ids = itemsRef.current.map((item) => item.id);
          const next = reorderProjectIds(ids, session.sourceId, target.id, target.position);
          if (next.some((id, index) => id !== ids[index])) {
            onReorderRef.current?.(next);
          }
        }
        window.setTimeout(() => {
          if (suppressClickRef.current === session.sourceId) {
            suppressClickRef.current = undefined;
          }
        }, 0);
      }
      resetPointerSession();
    };

    const cancelPointerSession = (event: PointerEvent) => {
      const session = pointerSessionRef.current;
      if (!session || session.pointerId !== event.pointerId) return;
      suppressClickRef.current = undefined;
      resetPointerSession();
    };

    window.addEventListener("pointermove", handlePointerMove, { passive: false });
    window.addEventListener("pointerup", finishPointerSession);
    window.addEventListener("pointercancel", cancelPointerSession);
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", finishPointerSession);
      window.removeEventListener("pointercancel", cancelPointerSession);
      resetPointerSession();
    };
  }, [activatePointerSession, resetPointerSession]);

  const beginPointerSession = (
    event: ReactPointerEvent<HTMLButtonElement>,
    item: ProjectItem,
  ) => {
    if (
      event.button !== 0 ||
      item.disabled ||
      reorderDisabled ||
      !onReorder ||
      items.length < 2
    ) {
      return;
    }
    const session: PointerSession = {
      pointerId: event.pointerId,
      sourceId: item.id,
      startX: event.clientX,
      startY: event.clientY,
      currentX: event.clientX,
      currentY: event.clientY,
      active: false,
    };
    session.activationTimer = window.setTimeout(
      activatePointerSession,
      REORDER_HOLD_DELAY_MS,
    );
    pointerSessionRef.current = session;
  };

  const consumeSuppressedClick = (id: string): boolean => {
    if (suppressClickRef.current !== id) return false;
    suppressClickRef.current = undefined;
    return true;
  };

  const handleReorderKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    id: string,
  ) => {
    if (!event.altKey || reorderDisabled || !onReorder) return;
    const direction = event.key === "ArrowUp" ? -1 : event.key === "ArrowDown" ? 1 : 0;
    if (direction === 0) return;
    const ids = items.map((item) => item.id);
    const index = ids.indexOf(id);
    const targetIndex = index + direction;
    if (index < 0 || targetIndex < 0 || targetIndex >= ids.length) return;
    event.preventDefault();
    const next = [...ids];
    [next[index], next[targetIndex]] = [next[targetIndex], next[index]];
    onReorder(next);
  };

  if (!items || items.length === 0) {
    return <div style={emptyStyle}>{emptyLabel}</div>;
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
          reorderable={Boolean(onReorder) && !reorderDisabled && items.length > 1}
          dragSourceId={dragSourceId}
          dropTarget={dropTarget}
          onPointerDown={beginPointerSession}
          onReorderKeyDown={handleReorderKeyDown}
          consumeSuppressedClick={consumeSuppressedClick}
        />
      ))}
      {dragSourceId && dragPoint && typeof document !== "undefined"
        ? createPortal(
            <div
              className="aw-project-drag-preview"
              style={{ left: dragPoint.x + 12, top: dragPoint.y + 10 }}
              aria-hidden="true"
            >
              <Folder size={19} strokeWidth={1.75} />
              <span>
                {items.find((item) => item.id === dragSourceId)?.label ?? dragSourceId}
              </span>
            </div>,
            document.body,
          )
        : null}
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
  reorderable?: boolean;
  dragSourceId?: string;
  dropTarget?: DropTarget;
  onPointerDown?: (event: ReactPointerEvent<HTMLButtonElement>, item: ProjectItem) => void;
  onReorderKeyDown?: (event: KeyboardEvent<HTMLButtonElement>, id: string) => void;
  consumeSuppressedClick?: (id: string) => boolean;
};

function ProjectNode({
  item,
  depth,
  activeId,
  expanded,
  onToggle,
  onSelect,
  reorderable = false,
  dragSourceId,
  dropTarget,
  onPointerDown,
  onReorderKeyDown,
  consumeSuppressedClick,
}: NodeProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const childItems = item.children ?? [];
  const hasChildren = childItems.length > 0;
  const isExpanded = expanded.includes(item.id);
  const isActive = item.id === activeId;
  const selection = item.selection;
  const isDisabled = item.disabled || Boolean(selection?.disabled);
  const isLoading = item.isLoading;
  const kind = item.kind ?? (depth === 0 || hasChildren ? "project" : "thread");
  const isProject = kind === "project";
  const isExpandable = isProject || hasChildren;
  const showTimeLabel = !isProject && item.timeLabel;
  const showInlineChevron = isExpandable;
  const isTopLevel = depth === 0;
  const isDragging = isTopLevel && dragSourceId === item.id;
  const dropPosition = isTopLevel && dropTarget?.id === item.id
    ? dropTarget.position
    : undefined;

  const rowHeight = isProject ? 36 : 32;
  const fontSize =
    isProject ? "var(--aw-font-size-base)" : "var(--aw-font-size-sm)";
  const lineHeight =
    isProject ? "var(--aw-line-height-base)" : "var(--aw-line-height-sm)";
  const idleColor =
    isProject ? "var(--aw-text-secondary)" : "var(--aw-text-tertiary)";
  const baseColor = isActive ? "var(--aw-text-primary)" : idleColor;

  const nodeClassName = [
    "aw-project-node",
    `aw-project-node--${kind}`,
    hasChildren ? "aw-project-node--has-children" : "",
    isExpanded ? "aw-project-node--expanded" : "",
    isActive ? "aw-project-node--active" : "",
    selection ? "aw-project-node--selection" : "",
    selection?.checked ? "aw-project-node--selected" : "",
    isDisabled ? "aw-project-node--disabled" : "",
    menuOpen ? "aw-project-node--menu-open" : "",
    reorderable && isTopLevel ? "aw-project-node--reorderable" : "",
    isDragging ? "aw-project-node--dragging" : "",
    dropPosition ? `aw-project-node--drop-${dropPosition}` : "",
  ].filter(Boolean).join(" ");

  const rowStyle = {
    "--aw-project-row-height": `${rowHeight}px`,
    "--aw-project-row-font-size": fontSize,
    "--aw-project-row-line-height": lineHeight,
    "--aw-project-row-color": baseColor,
  } as CSSProperties;

  const rowButtonStyle: CSSProperties = {
    fontSize,
    lineHeight,
    fontWeight: isProject
      ? "var(--aw-font-weight-medium)"
      : "var(--aw-font-weight-regular)",
    cursor: isDisabled ? "not-allowed" : "pointer",
    opacity: isDisabled ? 0.6 : 1,
  };

  const handleClick = () => {
    if (isDisabled) return;
    if (consumeSuppressedClick?.(item.id)) return;
    if (selection) {
      selection.onChange?.(!selection.checked);
      return;
    }
    if (isExpandable) {
      onToggle(item.id);
    }
    onSelect?.(item.id);
  };

  const childrenContainerStyle: CSSProperties = {
    display: "grid",
    gap: 1,
    padding: "3px 0 4px 8px",
  };

  // render prop 而不是包一层容器：行的激活、拖拽、菜单展开样式都挂在
  // `.aw-project-node > .aw-project-row` 直接子选择器上，多一层会静默打断它们。
  const renderRow = (anchor?: ProjectHoverCardAnchorProps) => (
      <div
        className="aw-project-row"
        style={rowStyle}
        data-aw-project-id={isTopLevel ? item.id : undefined}
        {...anchor}
      >
        <button
          type="button"
          className="aw-project-row-main"
          disabled={item.disabled}
          onClick={handleClick}
          onPointerDown={isTopLevel ? (event) => onPointerDown?.(event, item) : undefined}
          onKeyDown={isTopLevel ? (event) => onReorderKeyDown?.(event, item.id) : undefined}
          title={selection?.disabledReason ?? (item.hoverCard && isTopLevel ? undefined : item.path ?? item.label)}
          role={selection ? "checkbox" : undefined}
          aria-checked={selection ? selection.checked : undefined}
          aria-disabled={isDisabled || undefined}
          aria-label={selection ? `${selection.checked ? "取消选择" : "选择"} ${item.label}` : undefined}
          aria-current={!selection && isActive ? "true" : undefined}
          aria-expanded={isExpandable ? isExpanded : undefined}
          aria-grabbed={isDragging || undefined}
          aria-keyshortcuts={reorderable && isTopLevel ? "Alt+ArrowUp Alt+ArrowDown" : undefined}
          style={rowButtonStyle}
        >
          <span className="aw-project-row-icon">
            {selection ? (
              <span className="aw-project-row-checkbox" aria-hidden="true">
                <Check size={12} strokeWidth={2.2} />
              </span>
            ) : isProject ? (
              isExpanded ? (
                <FolderOpen size={20} strokeWidth={1.75} aria-hidden="true" />
              ) : (
                <Folder size={20} strokeWidth={1.75} aria-hidden="true" />
              )
            ) : isTopLevel ? (
              <MessageSquare size={17} strokeWidth={1.75} aria-hidden="true" />
            ) : null}
          </span>
          <span className="aw-project-row-label-wrap">
            <span className="aw-project-row-label">{item.label}</span>
            {showInlineChevron ? (
              <span className="aw-project-inline-chevron" aria-hidden="true">
                <ChevronRight size={16} strokeWidth={1.85} aria-hidden="true" />
              </span>
            ) : null}
          </span>
          {isLoading ? <AwakenSidebarThreadSpinner /> : null}
          {showTimeLabel ? <span className="aw-project-row-meta">{item.timeLabel}</span> : null}
        </button>
        {!selection && (item.action || item.menuItems?.length) ? (
          <span className="aw-project-row-actions">
            {item.action}
            {item.menuItems?.length ? (
              <AwakenDropdown
                open={menuOpen}
                onOpenChange={setMenuOpen}
                placement="bottom-end"
                offset={4}
                trigger={
                  <button
                    type="button"
                    className="aw-project-row-menu-trigger"
                    aria-label={`${item.label} 更多操作`}
                    title={`${item.label} 更多操作`}
                    disabled={isDisabled}
                  >
                    <MoreHorizontal size={16} strokeWidth={1.9} />
                  </button>
                }
              >
                <div className="aw-project-row-menu" role="menu">
                  {item.menuItems.map((menuItem) => (
                    <button
                      key={menuItem.id}
                      type="button"
                      className={[
                        "aw-project-row-menu-item",
                        menuItem.tone === "danger" ? "aw-project-row-menu-item--danger" : "",
                      ].filter(Boolean).join(" ")}
                      role="menuitem"
                      disabled={menuItem.disabled}
                      onClick={(event) => {
                        event.stopPropagation();
                        setMenuOpen(false);
                        menuItem.onSelect?.();
                      }}
                    >
                      {menuItem.label}
                    </button>
                  ))}
                </div>
              </AwakenDropdown>
            ) : null}
          </span>
        ) : null}
      </div>
  );

  return (
    <div className={nodeClassName}>
      {item.hoverCard && isTopLevel && !isDisabled ? (
        <AwakenProjectHoverCard card={item.hoverCard} disabled={isDragging || menuOpen}>
          {(anchor) => renderRow(anchor)}
        </AwakenProjectHoverCard>
      ) : (
        renderRow()
      )}

      {isExpandable && isExpanded ? (
        <div style={childrenContainerStyle}>
          {hasChildren ? childItems.map((child) => (
            <ProjectNode
              key={child.id}
              item={child}
              depth={depth + 1}
              activeId={activeId}
              expanded={expanded}
              onToggle={onToggle}
              onSelect={onSelect}
              dragSourceId={dragSourceId}
              dropTarget={dropTarget}
              consumeSuppressedClick={consumeSuppressedClick}
            />
          )) : (
            <div className="aw-project-empty-children">暂无会话</div>
          )}
        </div>
      ) : null}
    </div>
  );
}
