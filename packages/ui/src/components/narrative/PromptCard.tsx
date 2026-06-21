type PromptCardProps = {
  text: string;
  eyebrow?: string;
  status?: string;
};

export function PromptCard({
  text,
  eyebrow = "Prompt",
  status = "done",
}: PromptCardProps) {
  return (
    <article
      style={{
        display: "grid",
        gap: "10px",
        width: "100%",
        maxWidth: "100%",
        boxSizing: "border-box",
        minWidth: 0,
        padding: "18px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-panel)",
        background: "var(--aw-surface-raised)",
        boxShadow: "var(--aw-shadow-raised)",
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
        <span
          style={{
            minWidth: 0,
            color: "var(--aw-text-subtle)",
            fontSize: "var(--aw-font-size-xs)",
            fontWeight: "var(--aw-font-weight-heavy)",
            lineHeight: "var(--aw-line-height-xs)",
            overflowWrap: "anywhere",
          }}
        >
          {eyebrow}
        </span>
        <span
          style={{
            flex: "0 0 auto",
            height: "22px",
            padding: "2px 8px 0",
            border: "1px solid var(--aw-border-main)",
            borderRadius: "var(--aw-radius-control)",
            background: "var(--aw-bg-fill-light)",
            color: "var(--aw-text-subtle)",
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
          maxHeight: "220px",
          margin: 0,
          overflow: "auto",
          color: "var(--aw-text-primary)",
          fontSize: "var(--aw-font-size-md)",
          fontWeight: "var(--aw-font-weight-semibold)",
          lineHeight: "var(--aw-line-height-md)",
          whiteSpace: "pre-wrap",
          overflowWrap: "anywhere",
        }}
      >
        {text}
      </p>
    </article>
  );
}
