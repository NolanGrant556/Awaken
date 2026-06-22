import "./AwakenModelSelector.css";
import { useCallback, useState } from "react";
import type { ModelOption } from "../../types";
import { AwakenDropdown } from "../AwakenDropdown";
import { AwakenPill } from "../AwakenPill";
import { ChevronDown } from "lucide-react";

export type AwakenModelSelectorProps = {
  value: ModelOption;
  options: ModelOption[];
  onChange?: (option: ModelOption) => void;
  disabled?: boolean;
};

export function AwakenModelSelector({
  value,
  options,
  onChange,
  disabled,
}: AwakenModelSelectorProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = useCallback(
    (option: ModelOption) => {
      if (option.disabled) return;
      onChange?.(option);
      setOpen(false);
    },
    [onChange]
  );

  const isEmpty = options.length === 0;

  return (
    <AwakenDropdown
      open={open}
      onOpenChange={setOpen}
      trigger={
        <AwakenPill
          variant="bordered"
          size="sm"
          icon={<ChevronDown size={14} strokeWidth={1.85} />}
          disabled={disabled || isEmpty}
        >
          {value?.label ?? "选择模型"}
        </AwakenPill>
      }
    >
      {isEmpty ? (
        <div
          style={{
            padding: "8px 12px",
            fontSize: "var(--aw-font-size-sm)",
            color: "var(--aw-text-tertiary)",
            textAlign: "center",
          }}
        >
          无可用模型
        </div>
      ) : (
        options.map((option) => (
          <button
            key={option.id}
            type="button"
            onClick={() => handleSelect(option)}
            disabled={option.disabled}
            style={{
              display: "flex",
              alignItems: "center",
              width: "100%",
              padding: "6px 10px",
              border: "none",
              borderRadius: "var(--aw-radius-sm)",
              background:
                value.id === option.id
                  ? "var(--aw-bg-fill-main)"
                  : "transparent",
              color: "var(--aw-text-primary)",
              fontSize: "var(--aw-font-size-sm)",
              lineHeight: "var(--aw-line-height-sm)",
              textAlign: "left",
              cursor: option.disabled ? "not-allowed" : "pointer",
              opacity: option.disabled ? 0.45 : 1,
              transition: "background-color 120ms ease",
              whiteSpace: "nowrap",
            }}
            onMouseEnter={(e) => {
              if (!option.disabled) {
                e.currentTarget.style.background = "var(--aw-bg-fill-light)";
              }
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background =
                value.id === option.id
                  ? "var(--aw-bg-fill-main)"
                  : "transparent";
            }}
          >
            <span className="aw-truncate">{option.label}</span>
          </button>
        ))
      )}
    </AwakenDropdown>
  );
}
