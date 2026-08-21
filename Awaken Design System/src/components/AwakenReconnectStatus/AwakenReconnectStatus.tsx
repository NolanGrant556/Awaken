import "./AwakenReconnectStatus.css";
import { useId, useState, type ReactNode } from "react";
import { AwakenIcon } from "../AwakenIcon";

export type AwakenReconnectStatusProps = {
  title: ReactNode;
  detail?: string;
  details?: string[];
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

function visibleDetails(detail?: string, details?: string[]): string[] {
  const values = [
    ...(details ?? []),
    ...(detail && !(details ?? []).includes(detail) ? [detail] : []),
  ]
    .map((value) => value.trim())
    .filter(Boolean);
  return values.filter((value, index) => values.indexOf(value) === index);
}

export function AwakenReconnectStatus({
  title,
  detail,
  details,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
}: AwakenReconnectStatusProps) {
  const panelId = useId();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : uncontrolledOpen;
  const detailLines = visibleDetails(detail, details);
  const expandable = detailLines.length > 0;

  const toggleOpen = () => {
    if (!expandable) {
      return;
    }
    const nextOpen = !open;
    if (!isControlled) {
      setUncontrolledOpen(nextOpen);
    }
    onOpenChange?.(nextOpen);
  };

  return (
    <div
      className={`aw-reconnect-status${open && expandable ? " aw-reconnect-status-open" : ""}`}
    >
      {expandable ? (
        <button
          type="button"
          className="aw-reconnect-status-trigger"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={toggleOpen}
        >
          <span className="aw-reconnect-status-icon" aria-hidden="true">
            <AwakenIcon concept="network" size={16} />
          </span>
          <strong className="aw-reconnect-status-title">{title}</strong>
          <span className="aw-reconnect-status-chevron" aria-hidden="true">
            <AwakenIcon concept="disclose" size={16} />
          </span>
        </button>
      ) : (
        <div className="aw-reconnect-status-row">
          <span className="aw-reconnect-status-icon" aria-hidden="true">
            <AwakenIcon concept="network" size={16} />
          </span>
          <strong className="aw-reconnect-status-title">{title}</strong>
        </div>
      )}
      {expandable && open ? (
        <div id={panelId} className="aw-reconnect-status-panel">
          {detailLines.map((line) => (
            <p key={line} className="aw-reconnect-status-detail">
              {line}
            </p>
          ))}
        </div>
      ) : null}
    </div>
  );
}
