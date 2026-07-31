import "./AwakenTooltip.css";
import { useState, type ReactNode } from "react";
import {
  autoUpdate,
  flip,
  FloatingPortal,
  offset,
  shift,
  useDismiss,
  useFloating,
  useFocus,
  useHover,
  useInteractions,
  useRole,
  type Placement,
} from "@floating-ui/react";

export type AwakenTooltipProps = {
  label: ReactNode;
  children: ReactNode;
  placement?: Placement;
  disabled?: boolean;
  /** When false, only pointer hover opens the tip; keyboard focus does not. Default true. */
  openOnFocus?: boolean;
};

export function AwakenTooltip({
  label,
  children,
  placement = "top",
  disabled = false,
  openOnFocus = true,
}: AwakenTooltipProps) {
  const [open, setOpen] = useState(false);
  const isOpen = open && !disabled;

  const { refs, floatingStyles, context } = useFloating({
    open: isOpen,
    onOpenChange: setOpen,
    placement,
    whileElementsMounted: autoUpdate,
    middleware: [offset(8), flip({ padding: 8 }), shift({ padding: 8 })],
  });

  const hover = useHover(context, { move: false, enabled: !disabled });
  const focus = useFocus(context, { enabled: !disabled && openOnFocus });
  const dismiss = useDismiss(context, { escapeKey: true });
  const role = useRole(context, { role: "tooltip" });
  const { getReferenceProps, getFloatingProps } = useInteractions([
    hover,
    focus,
    dismiss,
    role,
  ]);

  return (
    <>
      <span
        ref={refs.setReference}
        {...getReferenceProps()}
        style={{ display: "inline-flex", minWidth: 0 }}
      >
        {children}
      </span>
      {isOpen ? (
        <FloatingPortal>
          <div
            ref={refs.setFloating}
            {...getFloatingProps()}
            style={{
              ...floatingStyles,
              zIndex: "var(--aw-z-tooltip)",
              maxWidth: 260,
              padding: "6px 8px",
              border: "1px solid var(--aw-border-main)",
              borderRadius: "var(--aw-radius-control)",
              background: "var(--aw-bg-white)",
              color: "var(--aw-text-primary)",
              boxShadow: "var(--aw-shadow-popover)",
              fontSize: "var(--aw-font-size-xs)",
              fontWeight: "var(--aw-font-weight-semibold)",
              lineHeight: "var(--aw-line-height-xs)",
              overflowWrap: "anywhere",
            }}
          >
            {label}
          </div>
        </FloatingPortal>
      ) : null}
    </>
  );
}
