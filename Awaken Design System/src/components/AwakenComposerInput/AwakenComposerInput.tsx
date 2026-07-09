import "./AwakenComposerInput.css";
import { useRef, useEffect, useCallback, type ClipboardEvent } from "react";

const DEFAULT_ACCEPTED_IMAGE_MIME_TYPES = ["image/png", "image/jpeg", "image/webp"] as const;

export type AwakenComposerInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onKeyDown?: (event: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  placeholder?: string;
  minRows?: number;
  maxRows?: number;
  disabled?: boolean;
  focusRequest?: number;
  onPasteImages?: (files: File[]) => void;
};

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
  onKeyDown,
  placeholder = "",
  minRows = 2,
  maxRows = 6,
  disabled,
  focusRequest = 0,
  onPasteImages,
}: AwakenComposerInputProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange(e.target.value);
    },
    [onChange]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      onKeyDown?.(e);
      if (e.defaultPrevented) {
        return;
      }
      if (
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
    [onKeyDown, onSubmit]
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
    const cursorPosition = textarea.value.length;
    textarea.setSelectionRange(cursorPosition, cursorPosition);
  }, [disabled, focusRequest]);

  return (
    <textarea
      ref={textareaRef}
      value={value}
      onChange={handleChange}
      onKeyDown={handleKeyDown}
      onPaste={handlePaste}
      placeholder={placeholder}
      disabled={disabled}
      aria-label="输入任务"
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
