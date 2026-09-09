import "./AwakenProjectHoverCard.css";
import { AwakenIcon } from "../AwakenIcon";
import { useEffect, useState, type ReactNode } from "react";
import {
  autoUpdate,
  flip,
  FloatingPortal,
  offset,
  safePolygon,
  shift,
  size,
  useDismiss,
  useFloating,
  useHover,
  useInteractions,
  useRole,
  type Placement,
} from "@floating-ui/react";
import type { ProjectHoverCard } from "../../types";

export type ProjectHoverCardAnchorProps = {
  ref: (node: HTMLElement | null) => void;
} & Record<string, unknown>;

export type AwakenProjectHoverCardProps = {
  card: ProjectHoverCard;
  /**
   * render prop 而不是包裹元素：项目行的激活、拖拽、菜单展开样式都依赖
   * `.aw-project-node > .aw-project-row` 直接子选择器，插一层容器会静默打断它们。
   */
  children: (anchor: ProjectHoverCardAnchorProps) => ReactNode;
  placement?: Placement;
  disabled?: boolean;
};

/**
 * 悬停在项目行上时展示这个项目的文件夹组成。用 safePolygon 让指针可以从行斜着
 * 移进卡片而不关闭，否则文件夹行根本点不到。
 */
export function AwakenProjectHoverCard({
  card,
  children,
  placement = "right-start",
  disabled = false,
}: AwakenProjectHoverCardProps) {
  const [open, setOpen] = useState(false);
  const isOpen = open && !disabled;

  /**
   * 转入 disabled 时把内部状态一起清掉。只靠 isOpen 遮住的话，指针没离开行时
   * disabled 一解除卡片会立刻弹回来，而且绕过了 useHover 的开启延迟。
   */
  useEffect(() => {
    if (disabled) {
      setOpen(false);
    }
  }, [disabled]);

  const { refs, floatingStyles, context, isPositioned } = useFloating({
    open: isOpen,
    onOpenChange: setOpen,
    placement,
    strategy: "fixed",
    whileElementsMounted: autoUpdate,
    middleware: [
      offset(8),
      flip({ padding: 8 }),
      shift({ padding: 8 }),
      size({
        padding: 8,
        apply({ availableWidth, availableHeight, elements }) {
          Object.assign(elements.floating.style, {
            maxWidth: `${Math.max(0, availableWidth)}px`,
            maxHeight: `${Math.max(0, availableHeight)}px`,
          });
        },
      }),
    ],
  });

  const hover = useHover(context, {
    enabled: !disabled,
    delay: { open: 420, close: 120 },
    handleClose: safePolygon({ blockPointerEvents: false }),
  });
  const dismiss = useDismiss(context, { escapeKey: true });
  const role = useRole(context, { role: "dialog" });
  const { getReferenceProps, getFloatingProps } = useInteractions([
    hover,
    dismiss,
    role,
  ]);

  return (
    <>
      {children({
        ref: refs.setReference,
        ...getReferenceProps(),
      })}
      {isOpen ? (
        <FloatingPortal>
          <div
            ref={refs.setFloating}
            {...getFloatingProps()}
            style={{ ...floatingStyles, visibility: isPositioned ? undefined : "hidden" }}
            className="aw-project-hover-card"
          >
            <div className="aw-project-hover-card__header">
              <span className="aw-project-hover-card__header-icon" aria-hidden="true">
                <AwakenIcon concept="project" size={16} />
              </span>
              <span className="aw-project-hover-card__title">{card.title}</span>
            </div>

            {card.summary ? (
              <div className="aw-project-hover-card__summary">
                <span aria-hidden="true">
                  <AwakenIcon concept="conversation" size={16} />
                </span>
                <span>{card.summary}</span>
              </div>
            ) : null}

            <div className="aw-project-hover-card__divider" role="presentation" />

            {card.folders.length > 0 ? (
              <ul className="aw-project-hover-card__folders">
                {card.folders.map((folder) => (
                  <li key={folder.path}>
                    <button
                      type="button"
                      className="aw-project-hover-card__folder"
                      aria-label={
                        card.revealHint
                          ? `${card.revealHint}${folder.label ?? folder.path}`
                          : folder.label ?? folder.path
                      }
                      onClick={() => card.onRevealFolder?.(folder.path)}
                    >
                      <span aria-hidden="true">
                        <AwakenIcon concept="project" size={16} />
                      </span>
                      <span className="aw-project-hover-card__folder-label">
                        {folder.label ?? folder.path}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="aw-project-hover-card__empty">
                {card.emptyFoldersLabel ?? "这个项目还没有文件夹"}
              </p>
            )}

            {card.actionLabel && card.onAction ? (
              <>
                <div className="aw-project-hover-card__divider" role="presentation" />
                <button
                  type="button"
                  className="aw-project-hover-card__action"
                  onClick={() => card.onAction?.()}
                >
                  <span aria-hidden="true">
                    <AwakenIcon concept="settings" size={16} />
                  </span>
                  <span>{card.actionLabel}</span>
                </button>
              </>
            ) : null}
          </div>
        </FloatingPortal>
      ) : null}
    </>
  );
}
