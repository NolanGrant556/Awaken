import { useRef, useEffect, useCallback } from "react";

type ComposerInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  minRows?: number;
  maxRows?: number;
  disabled?: boolean;
};

export function ComposerInput({
  value,
  onChange,
  onSubmit,
  placeholder = "输入任务...",
  minRows = 2,
  maxRows = 6,
  disabled,
}: ComposerInputProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange(e.target.value);
    },
    [onChange]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        onSubmit?.();
      }
    },
    [onSubmit]
  );

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    textarea.style.height = "auto";
    const lineHeight = 22;
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
        minHeight: minRows * 22,
        maxHeight: maxRows * 22,
        padding: 0,
        border: "none",
        background: "transparent",
        color: "var(--aw-text-primary)",
        fontSize: "var(--aw-font-size-md)",
        lineHeight: "var(--aw-line-height-md)",
        fontFamily: "var(--aw-font-sans)",
        resize: "none",
        cursor: disabled ? "not-allowed" : "text",
        opacity: disabled ? 0.45 : 1,
      }}
    />
  );
}
