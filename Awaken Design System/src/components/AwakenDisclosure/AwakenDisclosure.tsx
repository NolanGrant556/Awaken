import "./AwakenDisclosure.css";
import { useId, useState, type ReactNode } from "react";
import { ChevronRight } from "lucide-react";

export type AwakenDisclosureProps = {
  title: ReactNode;
  children: ReactNode;
  meta?: ReactNode;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

export function AwakenDisclosure({
  title,
  children,
  meta,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
}: AwakenDisclosureProps) {
  const panelId = useId();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : uncontrolledOpen;

  const toggleOpen = () => {
    const nextOpen = !open;
    if (!isControlled) {
      setUncontrolledOpen(nextOpen);
    }
    onOpenChange?.(nextOpen);
  };

  return (
    <div
      className={`aw-disclosure${open ? " aw-disclosure-open" : ""}`}
    >
      <button
        type="button"
        className="aw-disclosure-trigger"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggleOpen}
      >
        <strong className="aw-disclosure-title">
          {title}
        </strong>
        {meta ? (
          <span className="aw-disclosure-meta">
            {meta}
          </span>
        ) : null}
        <span className="aw-disclosure-chevron" aria-hidden="true">
          <ChevronRight size={15} strokeWidth={2} />
        </span>
      </button>
      {open ? (
        <div id={panelId} className="aw-disclosure-panel">
          {children}
        </div>
      ) : null}
    </div>
  );
}
