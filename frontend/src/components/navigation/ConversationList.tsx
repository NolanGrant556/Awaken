import type { CSSProperties, MouseEvent } from "react";
import type { ConversationItem } from "../../types";

type ConversationListProps = {
  items: ConversationItem[];
  activeId?: string;
  onSelect?: (id: string) => void;
  onContextMenu?: (id: string, event: MouseEvent<HTMLElement>) => void;
};

const listStyle: CSSProperties = {
  display: "grid",
  gap: 2,
};

const rowBase: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "minmax(0, 1fr) auto",
  alignItems: "center",
  width: "100%",
  minHeight: 34,
  gap: 8,
  padding: "0 10px",
  border: 0,
  borderRadius: "var(--aw-radius-sm)",
  background: "transparent",
  color: "var(--aw-text-primary)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color 160ms ease, color 160ms ease",
};

const titleStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-sm)",
  fontWeight: "var(--aw-font-weight-semibold)",
  lineHeight: "var(--aw-line-height-sm)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const timeStyle: CSSProperties = {
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
  whiteSpace: "nowrap",
};

const emptyStyle: CSSProperties = {
  padding: "10px 12px",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
};

export function ConversationList({ items, activeId, onSelect, onContextMenu }: ConversationListProps) {
  if (!items || items.length === 0) {
    return <div style={emptyStyle}>暂无对话</div>;
  }

  return (
    <div className="aw-conversation-list" style={listStyle}>
      {items.map((item) => {
        const isActive = item.id === activeId;
        const isDisabled = item.disabled;
        const baseBg = isActive ? "var(--aw-bg-fill-main)" : "transparent";

        const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) {
            e.currentTarget.style.background = isActive
              ? "var(--aw-bg-fill-main)"
              : "var(--aw-bg-fill-light)";
          }
        };
        const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
          e.currentTarget.style.background = baseBg;
        };
        const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) e.currentTarget.style.background = "var(--aw-bg-fill-dark)";
        };
        const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) {
            e.currentTarget.style.background = isActive
              ? "var(--aw-bg-fill-main)"
              : "var(--aw-bg-fill-light)";
          }
        };

        return (
          <button
            key={item.id}
            type="button"
            disabled={isDisabled}
            onClick={() => !isDisabled && onSelect?.(item.id)}
            onContextMenu={(e) => onContextMenu?.(item.id, e)}
            title={item.label}
            style={{
              ...rowBase,
              background: baseBg,
              cursor: isDisabled ? "not-allowed" : "pointer",
              opacity: isDisabled ? 0.6 : 1,
            }}
            onMouseEnter={handleEnter}
            onMouseLeave={handleLeave}
            onMouseDown={handleDown}
            onMouseUp={handleUp}
          >
            <span style={{ minWidth: 0 }}>
              <span style={titleStyle}>{item.label}</span>
            </span>
            <span style={timeStyle}>{item.timeLabel}</span>
          </button>
        );
      })}
    </div>
  );
}
