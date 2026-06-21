import { useId, useState, type ReactNode } from "react";
import { ChevronRight } from "lucide-react";

type DisclosureProps = {
  title: ReactNode;
  children: ReactNode;
  meta?: ReactNode;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

export function Disclosure({
  title,
  children,
  meta,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
}: DisclosureProps) {
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
      style={{
        display: "grid",
        gap: open ? "8px" : 0,
        minWidth: 0,
      }}
    >
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggleOpen}
        style={{
          display: "grid",
          gridTemplateColumns: "auto minmax(0, 1fr) auto",
          alignItems: "center",
          gap: "8px",
          width: "100%",
          minWidth: 0,
          padding: 0,
          border: 0,
          background: "transparent",
          color: "inherit",
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        <ChevronRight
          size={15}
          strokeWidth={2}
          style={{
            flex: "0 0 auto",
            color: "var(--aw-text-subtle)",
            transform: open ? "rotate(90deg)" : "rotate(0deg)",
            transition: "transform 140ms ease",
          }}
        />
        <strong
          style={{
            minWidth: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            lineHeight: "var(--aw-line-height-sm)",
            overflowWrap: "anywhere",
          }}
        >
          {title}
        </strong>
        {meta ? (
          <span
            style={{
              flex: "0 0 auto",
              color: "var(--aw-text-subtle)",
              fontSize: "var(--aw-font-size-xs)",
              lineHeight: "var(--aw-line-height-xs)",
              whiteSpace: "nowrap",
            }}
          >
            {meta}
          </span>
        ) : null}
      </button>
      {open ? (
        <div id={panelId} style={{ minWidth: 0 }}>
          {children}
        </div>
      ) : null}
    </div>
  );
}
