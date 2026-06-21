import { ToolActionRow, type ToolActionStatus } from "./ToolActionRow";

export type ExploringActionItem = {
  id: string;
  title: string;
  detail?: string;
  status?: ToolActionStatus;
};

type ExploringGroupProps = {
  title?: string;
  status?: "active" | "done";
  actions: ExploringActionItem[];
  onSelectAction?: (action: ExploringActionItem) => void;
};

export function ExploringGroup({
  title = "Exploring",
  status = "active",
  actions,
  onSelectAction,
}: ExploringGroupProps) {
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
            <ToolActionRow
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
