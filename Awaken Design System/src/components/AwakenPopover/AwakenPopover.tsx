import "./AwakenPopover.css";
import { useState, useCallback, useRef, useEffect } from "react";
import type { ReactNode } from "react";
import {
  useFloating,
  autoUpdate,
  offset,
  flip,
  shift,
  useClick,
  useDismiss,
  useRole,
  useInteractions,
  FloatingPortal,
  FloatingFocusManager,
  type Placement,
} from "@floating-ui/react";

export type AwakenPopoverProps = {
  trigger: ReactNode;
  content: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  placement?: Placement;
  offset?: number;
  matchTriggerWidth?: boolean;
  role?: "dialog" | "menu" | "listbox" | "tree" | "grid";
  /** Keep the floating surface beside its trigger so clipping ancestors can bound placement. */
  portalled?: boolean;
};

export function AwakenPopover({
  trigger,
  content,
  open: controlledOpen,
  onOpenChange,
  placement = "bottom-start",
  offset: offsetValue = 6,
  matchTriggerWidth = false,
  role = "dialog",
  portalled = true,
}: AwakenPopoverProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);

  const isControlled = controlledOpen !== undefined;
  const isOpen = isControlled ? controlledOpen : uncontrolledOpen;

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!isControlled) {
        setUncontrolledOpen(nextOpen);
      }
      onOpenChange?.(nextOpen);
    },
    [isControlled, onOpenChange]
  );

  const { refs, floatingStyles, context } = useFloating({
    open: isOpen,
    onOpenChange: handleOpenChange,
    placement,
    whileElementsMounted: autoUpdate,
    middleware: [
      offset(offsetValue),
      flip({ padding: 8 }),
      shift({ padding: 8 }),
    ],
  });

  const click = useClick(context);
  const dismiss = useDismiss(context, { escapeKey: true, outsidePress: true });
  const floatingRole = useRole(context, { role });

  const { getReferenceProps, getFloatingProps } = useInteractions([
    click,
    dismiss,
    floatingRole,
  ]);

  const triggerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (matchTriggerWidth && refs.floating.current && triggerRef.current) {
      const width = triggerRef.current.offsetWidth;
      refs.floating.current.style.width = `${width}px`;
    }
  }, [matchTriggerWidth, refs.floating, isOpen]);

  const floatingSurface = isOpen ? (
    <FloatingFocusManager context={context} modal={false}>
      <div
        ref={refs.setFloating}
        className="aw-popover__floating"
        style={floatingStyles}
        {...getFloatingProps()}
      >
        {content}
      </div>
    </FloatingFocusManager>
  ) : null;

  return (
    <>
      <div ref={triggerRef} style={{ display: "inline-flex" }}>
        <div ref={refs.setReference} {...getReferenceProps()}>
          {trigger}
        </div>
      </div>
      {floatingSurface
        ? portalled
          ? <FloatingPortal>{floatingSurface}</FloatingPortal>
          : floatingSurface
        : null}
    </>
  );
}
