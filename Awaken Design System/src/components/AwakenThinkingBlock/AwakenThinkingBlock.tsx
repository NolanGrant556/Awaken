import "./AwakenThinkingBlock.css";
export type AwakenThinkingBlockProps = {
  text: string;
  status?: "active" | "done";
};

export function AwakenThinkingBlock({ text, status = "active" }: AwakenThinkingBlockProps) {
  return (
    <article
      style={{
        display: "grid",
        gap: "8px",
        width: "100%",
        maxWidth: "100%",
        boxSizing: "border-box",
        minWidth: 0,
        padding: "14px 16px",
        border: "1px solid color-mix(in srgb, var(--aw-status-running) 18%, transparent)",
        borderRadius: "var(--aw-radius-panel)",
        background: "color-mix(in srgb, var(--aw-status-running) 6%, var(--aw-surface-raised))",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
          minWidth: 0,
        }}
      >
        <h2
          style={{
            margin: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            fontWeight: "var(--aw-font-weight-bold)",
            lineHeight: "var(--aw-line-height-sm)",
          }}
        >
          Thinking
        </h2>
        <span
          style={{
            flex: "0 0 auto",
            color: "var(--aw-status-running)",
            fontSize: "var(--aw-font-size-xs)",
            fontWeight: "var(--aw-font-weight-semibold)",
            lineHeight: "var(--aw-line-height-xs)",
            whiteSpace: "nowrap",
          }}
        >
          {status}
        </span>
      </div>
      <p
        style={{
          maxHeight: "240px",
          margin: 0,
          overflow: "auto",
          color: "var(--aw-text-muted)",
          fontSize: "var(--aw-font-size-sm)",
          lineHeight: "var(--aw-line-height-base)",
          whiteSpace: "pre-wrap",
          overflowWrap: "anywhere",
        }}
      >
        {text || "Thinking is streaming"}
      </p>
    </article>
  );
}
