import "./AwakenComposerInput.css";
import { useRef, useEffect, useCallback } from "react";

export type AwakenComposerInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  minRows?: number;
  maxRows?: number;
  disabled?: boolean;
};

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
  placeholder = "",
  minRows = 2,
  maxRows = 6,
  disabled,
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
    [onSubmit]
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

  return (
    <textarea
      ref={textareaRef}
      value={value}
      onChange={handleChange}
      onKeyDown={handleKeyDown}
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
