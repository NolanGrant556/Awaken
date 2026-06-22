import "./AwakenExploringGroup.css";
import { AwakenToolActionRow, type ToolActionStatus } from "../AwakenToolActionRow";

export type ExploringActionItem = {
  id: string;
  title: string;
  detail?: string;
  status?: ToolActionStatus;
};

export type AwakenExploringGroupProps = {
  title?: string;
  status?: "active" | "done";
  actions: ExploringActionItem[];
  onSelectAction?: (action: ExploringActionItem) => void;
};

export function AwakenExploringGroup({
  title = "Exploring",
  status = "active",
  actions,
  onSelectAction,
}: AwakenExploringGroupProps) {
  return (
    <article className="narrative-item narrative-kind-exploring">
      <div className="narrative-item-header">
        <h2>{title}</h2>
        <span>{status}</span>
      </div>
      <ul
        className="exploring-action-list"
        style={{
          display: "grid",
          gap: "8px",
          margin: 0,
          padding: 0,
          listStyle: "none",
        }}
      >
        {actions.map((action) => (
          <li key={action.id} style={{ minWidth: 0 }}>
            <AwakenToolActionRow
              title={action.title}
              detail={action.detail}
              status={action.status}
              onSelect={onSelectAction ? () => onSelectAction(action) : undefined}
            />
          </li>
        ))}
      </ul>
    </article>
  );
}
