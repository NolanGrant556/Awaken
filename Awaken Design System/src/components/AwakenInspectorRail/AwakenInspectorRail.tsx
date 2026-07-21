import "./AwakenInspectorRail.css";
import type { ReactNode } from "react";

export type AwakenInspectorRailItem = {
  id: string;
  label: string;
  badge?: string | number;
  disabled?: boolean;
  icon?: ReactNode;
  deltaAdded?: number;
  deltaRemoved?: number;
  tone?: "neutral" | "success" | "warning" | "error";
};

export type AwakenInspectorRailProps = {
  title?: string;
  status?: string;
  items: AwakenInspectorRailItem[];
  activeId?: string;
  onSelect?: (id: string) => void;
  action?: ReactNode;
  children?: ReactNode;
};

export function AwakenInspectorRail({
  title = "On Awaken",
  status,
  items,
  activeId,
  onSelect,
  action,
  children,
}: AwakenInspectorRailProps) {
  return (
    <aside className="light-inspector-rail" aria-label={title}>
      <div className="light-inspector-head">
        <h2>{title}</h2>
        {status || action ? (
          <div className="light-inspector-head-meta">
            {status ? <span>{status}</span> : null}
            {action}
          </div>
        ) : null}
      </div>
      <nav className="light-inspector-nav" aria-label="Inspector views">
        {items.map((item) => {
          const active = item.id === activeId;
          return (
            <button
              key={item.id}
              type="button"
              className={`light-inspector-tab light-inspector-tab-${
                item.tone ?? "neutral"
              }${active ? " is-active" : ""}`}
              disabled={item.disabled}
              aria-current={active ? "page" : undefined}
              onClick={() => onSelect?.(item.id)}
            >
              <span className="light-inspector-tab-label">
                {item.icon ? <span className="light-inspector-tab-icon">{item.icon}</span> : null}
                <span>{item.label}</span>
              </span>
              <span className="light-inspector-tab-meta">
                {item.deltaAdded ? (
                  <span className="light-inspector-delta light-inspector-delta-added">
                    +{item.deltaAdded}
                  </span>
                ) : null}
                {item.deltaRemoved ? (
                  <span className="light-inspector-delta light-inspector-delta-removed">
                    -{item.deltaRemoved}
                  </span>
                ) : null}
                {item.badge ? <strong>{item.badge}</strong> : null}
              </span>
            </button>
          );
        })}
      </nav>
      <div className="light-inspector-detail">{children}</div>
    </aside>
  );
}
