import "./AwakenDialog.css";
import { AwakenIcon } from "../AwakenIcon";
import {
  useEffect,
  useId,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { AwakenIconButton } from "../AwakenIconButton";

export type AwakenDialogProps = {
  open: boolean;
  title: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  onOpenChange: (open: boolean) => void;
  ariaLabelledBy?: string;
  size?: "default" | "large";
  titleVisuallyHidden?: boolean;
  initialFocusRef?: RefObject<HTMLElement | null>;
  className?: string;
  bodyClassName?: string;
  bodyRef?: Ref<HTMLDivElement>;
  bodyTabIndex?: number;
  modal?: boolean;
};

const focusableSelector = [
  "a[href]",
  "button:not([disabled])",
  "textarea:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

export function AwakenDialog({
  open,
  title,
  children,
  footer,
  onOpenChange,
  ariaLabelledBy,
  size = "default",
  titleVisuallyHidden = false,
  initialFocusRef,
  className,
  bodyClassName,
  bodyRef,
  bodyTabIndex,
  modal = true,
}: AwakenDialogProps) {
  const generatedTitleId = useId();
  const titleId = ariaLabelledBy ?? generatedTitleId;
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || typeof document === "undefined") {
      return;
    }

    const previousActiveElement = document.activeElement;
    const dialog = dialogRef.current;
    const firstFocusable = dialog?.querySelector<HTMLElement>(focusableSelector);
    (initialFocusRef?.current ?? firstFocusable ?? dialog)?.focus();

    return () => {
      if (
        previousActiveElement instanceof HTMLElement &&
        previousActiveElement.isConnected
      ) {
        previousActiveElement.focus();
      }
    };
  }, [initialFocusRef, open]);

  useEffect(() => {
    if (!open || typeof document === "undefined") {
      return;
    }

    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onOpenChange(false);
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onOpenChange, open]);

  if (!open) {
    return null;
  }

  const dialog = (
    <div
      className="aw-dialog-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onOpenChange(false);
        }
      }}
    >
      <div
        ref={dialogRef}
        className={[
          "aw-dialog",
          size === "large" ? "aw-dialog--large" : "",
          className ?? "",
        ].filter(Boolean).join(" ")}
        role="dialog"
        aria-modal={modal ? "true" : "false"}
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={(event: ReactKeyboardEvent<HTMLDivElement>) => {
          if (event.key !== "Tab") {
            return;
          }
          const focusable = Array.from(
            event.currentTarget.querySelectorAll<HTMLElement>(focusableSelector),
          );
          if (focusable.length === 0) {
            event.preventDefault();
            return;
          }
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first.focus();
          }
        }}
      >
        <header
          className={[
            "aw-dialog__head",
            titleVisuallyHidden ? "aw-dialog__head--title-visually-hidden" : "",
          ].filter(Boolean).join(" ")}
        >
          <h2
            className={[
              "aw-dialog__title",
              titleVisuallyHidden ? "aw-dialog__title--visually-hidden" : "",
            ].filter(Boolean).join(" ")}
            id={titleId}
          >
            {title}
          </h2>
          <AwakenIconButton
            label="关闭"
            size="sm"
            showTooltip={false}
            onClick={() => onOpenChange(false)}
          >
            <AwakenIcon concept="close" size={16} />
          </AwakenIconButton>
        </header>
        <div
          ref={bodyRef}
          className={["aw-dialog__body", bodyClassName ?? ""].filter(Boolean).join(" ")}
          tabIndex={bodyTabIndex}
        >
          {children}
        </div>
        {footer ? <footer className="aw-dialog__foot">{footer}</footer> : null}
      </div>
    </div>
  );

  if (typeof document === "undefined") {
    return dialog;
  }

  return createPortal(dialog, document.body);
}
