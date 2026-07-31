import "./AwakenComposerInput.css";
import { useRef, useEffect, useCallback, type ClipboardEvent } from "react";

const DEFAULT_ACCEPTED_IMAGE_MIME_TYPES = ["image/png", "image/jpeg", "image/webp"] as const;

export type AwakenComposerInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  submitOnEnter?: boolean;
  onKeyDown?: (event: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  ariaLabel?: string;
  placeholder?: string;
  minRows?: number;
  maxRows?: number;
  disabled?: boolean;
  focusRequest?: number;
  focusOffset?: number;
  onSelectionChange?: (selection: ComposerInputSelection) => void;
  onPasteImages?: (files: File[]) => void;
};

export type ComposerInputSelection = {
  anchor: number;
  focus: number;
  caretRect?: ComposerInputCaretRect;
};

export type ComposerInputCaretRect = {
  left: number;
  top: number;
  bottom: number;
};

export function textareaCaretRect(
  textarea: HTMLTextAreaElement,
  offset: number,
): ComposerInputCaretRect {
  const ownerDocument = textarea.ownerDocument;
  const ownerWindow = ownerDocument.defaultView;
  const textareaRect = textarea.getBoundingClientRect();
  if (!ownerWindow) {
    return { left: textareaRect.left, top: textareaRect.top, bottom: textareaRect.bottom };
  }

  const computed = ownerWindow.getComputedStyle(textarea);
  const mirror = ownerDocument.createElement("div");
  const marker = ownerDocument.createElement("span");
  const copiedProperties = [
    "borderLeftWidth",
    "borderRightWidth",
    "borderTopWidth",
    "borderBottomWidth",
    "fontFamily",
    "fontSize",
    "fontStyle",
    "fontWeight",
    "letterSpacing",
    "lineHeight",
    "paddingLeft",
    "paddingRight",
    "paddingTop",
    "paddingBottom",
    "textAlign",
    "textIndent",
    "textTransform",
    "wordSpacing",
  ] as const;
  for (const property of copiedProperties) {
    mirror.style[property] = computed[property];
  }
  Object.assign(mirror.style, {
    position: "fixed",
    visibility: "hidden",
    pointerEvents: "none",
    boxSizing: computed.boxSizing,
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    overflow: "hidden",
    width: `${textareaRect.width}px`,
    minHeight: `${textareaRect.height}px`,
    left: `${textareaRect.left - textarea.scrollLeft}px`,
    top: `${textareaRect.top - textarea.scrollTop}px`,
  });
  mirror.textContent = textarea.value.slice(0, Math.max(0, Math.min(offset, textarea.value.length)));
  marker.textContent = "\u200b";
  mirror.append(marker);
  ownerDocument.body.append(mirror);
  const markerRect = marker.getBoundingClientRect();
  mirror.remove();
  const lineHeight = Number.parseFloat(computed.lineHeight) || 20;
  return {
    left: markerRect.left,
    top: markerRect.top,
    bottom: markerRect.top + Math.max(markerRect.height, lineHeight),
  };
}

export function pastedImageFilesFromClipboardData(
  clipboardData?: Pick<DataTransfer, "items" | "files"> | null,
  acceptedMimeTypes: readonly string[] = DEFAULT_ACCEPTED_IMAGE_MIME_TYPES,
): File[] {
  if (!clipboardData) {
    return [];
  }

  const files: File[] = [];
  const accepted = new Set(acceptedMimeTypes);
  const items = Array.from(clipboardData.items ?? []);
  for (const item of items) {
    if (item.kind !== "file" || !accepted.has(item.type)) {
      continue;
    }
    const file = item.getAsFile();
    if (file) {
      files.push(file);
    }
  }

  if (files.length > 0) {
    return files;
  }

  return Array.from(clipboardData.files ?? []).filter((file) => accepted.has(file.type));
}

export type ComposerInputKeyState = {
  key: string;
  shiftKey?: boolean;
  isComposing?: boolean;
};

export function shouldSubmitComposerInputKey({
  key,
  shiftKey,
  isComposing,
}: ComposerInputKeyState): boolean {
  return key === "Enter" && !shiftKey && !isComposing;
}

export function AwakenComposerInput({
  value,
  onChange,
  onSubmit,
  submitOnEnter = true,
  onKeyDown,
  ariaLabel = "输入任务",
  placeholder = "",
  minRows = 2,
  maxRows = 6,
  disabled,
  focusRequest = 0,
  focusOffset,
  onSelectionChange,
  onPasteImages,
}: AwakenComposerInputProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange(e.target.value);
      const anchor = e.target.selectionStart ?? e.target.value.length;
      const focus = e.target.selectionEnd ?? e.target.value.length;
      onSelectionChange?.({
        anchor,
        focus,
        caretRect: anchor === focus ? textareaCaretRect(e.target, focus) : undefined,
      });
    },
    [onChange, onSelectionChange],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      onKeyDown?.(e);
      if (e.defaultPrevented) {
        return;
      }
      if (
        !submitOnEnter ||
        !shouldSubmitComposerInputKey({
          key: e.key,
          shiftKey: e.shiftKey,
          isComposing: e.nativeEvent.isComposing,
        })
      ) {
        return;
      }

      e.preventDefault();
      onSubmit?.();
    },
    [onKeyDown, onSubmit, submitOnEnter],
  );

  const handlePaste = useCallback(
    (event: ClipboardEvent<HTMLTextAreaElement>) => {
      if (!onPasteImages) {
        return;
      }
      const files = pastedImageFilesFromClipboardData(event.clipboardData);
      if (files.length === 0) {
        return;
      }
      event.preventDefault();
      onPasteImages(files);
    },
    [onPasteImages],
  );

  const emitSelection = useCallback(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    onSelectionChange?.({
      anchor: textarea.selectionStart ?? textarea.value.length,
      focus: textarea.selectionEnd ?? textarea.value.length,
      caretRect:
        textarea.selectionStart === textarea.selectionEnd
          ? textareaCaretRect(textarea, textarea.selectionEnd ?? textarea.value.length)
          : undefined,
    });
  }, [onSelectionChange]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    textarea.style.height = "auto";
    const lineHeight = Number.parseFloat(window.getComputedStyle(textarea).lineHeight) || 22;
    const minHeight = minRows * lineHeight;
    const maxHeight = maxRows * lineHeight;
    const scrollHeight = textarea.scrollHeight;
    const newHeight = Math.min(Math.max(scrollHeight, minHeight), maxHeight);
    textarea.style.height = `${newHeight}px`;
  }, [value, minRows, maxRows]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea || disabled || focusRequest <= 0) return;

    textarea.focus({ preventScroll: true });
    const cursorPosition = Math.max(
      0,
      Math.min(focusOffset ?? textarea.value.length, textarea.value.length),
    );
    textarea.setSelectionRange(cursorPosition, cursorPosition);
    onSelectionChange?.({ anchor: cursorPosition, focus: cursorPosition });
    // focusOffset and the callback belong to this focusRequest snapshot. A
    // controlled parent may recreate the callback on every keystroke; that
    // must not reposition the caret until it explicitly increments request.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [disabled, focusRequest]);

  return (
    <textarea
      ref={textareaRef}
      value={value}
      onChange={handleChange}
      onSelect={emitSelection}
      onBlur={emitSelection}
      onScroll={emitSelection}
      onKeyDown={handleKeyDown}
      onPaste={handlePaste}
      placeholder={placeholder}
      disabled={disabled}
      aria-label={ariaLabel}
      rows={minRows}
      style={{
        width: "100%",
        minHeight: minRows * 20,
        maxHeight: maxRows * 20,
        padding: 0,
        border: "none",
        outline: "none",
        background: "transparent",
        color: "var(--aw-text-primary)",
        display: "block",
        fontSize: "var(--aw-font-size-md)",
        lineHeight: "var(--aw-line-height-md)",
        fontFamily: "var(--aw-font-sans)",
        overflow: "auto",
        resize: "none",
        cursor: disabled ? "not-allowed" : "text",
        opacity: disabled ? 0.45 : 1,
      }}
    />
  );
}
