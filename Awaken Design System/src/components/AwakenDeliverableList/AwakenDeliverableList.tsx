import "./AwakenDeliverableList.css";
import { useId, useState } from "react";
import {
  AwakenDeliverableCard,
  type AwakenDeliverableCardProps,
} from "../AwakenDeliverableCard";

export type AwakenDeliverableListItem = AwakenDeliverableCardProps & {
  id: string;
};

export type AwakenDeliverableListProps = {
  items: AwakenDeliverableListItem[];
  terminalStatus?: "finished" | "error" | "cancelled";
  initiallyVisibleCount?: number;
};

const terminalMessage: Record<"error" | "cancelled", string> = {
  error: "任务最终失败，但以下文件已成功生成",
  cancelled: "任务已停止，但以下文件已成功生成",
};

export function AwakenDeliverableList({
  items,
  terminalStatus = "finished",
  initiallyVisibleCount = 3,
}: AwakenDeliverableListProps) {
  const [expanded, setExpanded] = useState(false);
  const listId = useId();
  const hasMore = items.length > initiallyVisibleCount;
  const visibleItems = expanded || !hasMore ? items : items.slice(0, initiallyVisibleCount);
  const hiddenCount = Math.max(0, items.length - initiallyVisibleCount);

  if (items.length === 0) {
    return null;
  }

  return (
    <section className="aw-deliverable-list" aria-label="任务交付文件">
      {terminalStatus === "error" || terminalStatus === "cancelled" ? (
        <p className={`aw-deliverable-list-notice aw-deliverable-list-notice-${terminalStatus}`}>
          {terminalMessage[terminalStatus]}
        </p>
      ) : null}
      <div className="aw-deliverable-list-items" id={listId}>
        {visibleItems.map((item) => (
          <div
            key={item.id}
            className={`aw-deliverable-list-item aw-deliverable-list-item-${item.fileType}`}
          >
            <AwakenDeliverableCard {...item} />
          </div>
        ))}
      </div>
      {hasMore ? (
        <button
          type="button"
          className="aw-deliverable-list-toggle"
          aria-expanded={expanded}
          aria-controls={listId}
          onClick={() => setExpanded((current) => !current)}
        >
          {expanded ? "收起" : `展开其余 ${hiddenCount} 个交付文件`}
        </button>
      ) : null}
    </section>
  );
}
