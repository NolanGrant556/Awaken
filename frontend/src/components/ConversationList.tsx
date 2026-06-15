import type { ConversationItem } from "../types";

type ConversationListProps = {
  items: ConversationItem[];
  activeItemId?: string;
  onSelectItem?: (id: string) => void;
};

export function ConversationList({ items, activeItemId, onSelectItem }: ConversationListProps) {
  return (
    <div className="sidebar-section">
      <div className="section-title">对话</div>
      <div className="sidebar-list">
        {items.map((item) => (
          <button
            key={item.id}
            className={`conversation-row ${activeItemId === item.id ? "is-active" : ""}`}
            onClick={() => onSelectItem?.(item.id)}
          >
            <span className="row-label">{item.label}</span>
            <span className="row-time">{item.timeLabel}</span>
          </button>
        ))}
      </div>
    </div>
  );
}
