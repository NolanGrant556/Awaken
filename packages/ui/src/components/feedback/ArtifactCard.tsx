import { FileText } from "lucide-react";

type ArtifactCardProps = {
  title: string;
  description?: string;
  onClick?: () => void;
};

const baseCardStyle = {
  display: "grid",
  gap: "6px",
  width: "100%",
  padding: "14px",
  border: "1px solid var(--aw-border-main)",
  borderRadius: "var(--aw-radius-md)",
  background: "var(--aw-bg-white)",
  textAlign: "left" as const,
};

export function ArtifactCard({ title, description, onClick }: ArtifactCardProps) {
  const content = (
    <>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "8px",
          minWidth: 0,
        }}
      >
        <FileText
          size={16}
          strokeWidth={1.85}
          style={{ flex: "0 0 auto", color: "var(--aw-text-tertiary)" }}
        />
        <span
          className="aw-truncate"
          style={{
            fontSize: "var(--aw-font-size-sm)",
            lineHeight: "var(--aw-line-height-sm)",
            fontWeight: "var(--aw-font-weight-semibold)",
            color: "var(--aw-text-primary)",
          }}
          title={title}
        >
          {title}
        </span>
      </div>
      {description ? (
        <span
          style={{
            display: "-webkit-box",
            WebkitLineClamp: 2,
            WebkitBoxOrient: "vertical",
            overflow: "hidden",
            fontSize: "var(--aw-font-size-xs)",
            lineHeight: "var(--aw-line-height-xs)",
            color: "var(--aw-text-secondary)",
          }}
          title={description}
        >
          {description}
        </span>
      ) : null}
    </>
  );

  if (!onClick) {
    return (
      <article style={{ ...baseCardStyle, cursor: "default" }}>{content}</article>
    );
  }

  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        ...baseCardStyle,
        cursor: "pointer",
        transition:
          "background-color 160ms ease, border-color 160ms ease, transform 160ms ease",
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.background = "var(--aw-bg-fill-light)";
        e.currentTarget.style.borderColor = "var(--aw-border-main)";
        e.currentTarget.style.transform = "translateY(-1px)";
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "var(--aw-bg-white)";
        e.currentTarget.style.borderColor = "var(--aw-border-main)";
        e.currentTarget.style.transform = "translateY(0)";
      }}
      onMouseDown={(e) => {
        e.currentTarget.style.transform = "translateY(0)";
      }}
      onMouseUp={(e) => {
        e.currentTarget.style.transform = "translateY(-1px)";
      }}
    >
      {content}
    </button>
  );
}
