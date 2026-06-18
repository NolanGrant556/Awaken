import { FileText, FilePlus, FilePen, FileMinus, FileOutput } from "lucide-react";
import type { ArtifactChangeType } from "../../types";

type ArtifactCardProps = {
  title: string;
  description?: string;
  onClick?: () => void;
  changeType?: ArtifactChangeType;
  path?: string;
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

function getIconConfig(changeType?: ArtifactChangeType) {
  switch (changeType) {
    case "added":
      return { Icon: FilePlus, color: "var(--aw-success)" };
    case "modified":
      return { Icon: FilePen, color: "var(--aw-text-blue)" };
    case "deleted":
      return { Icon: FileMinus, color: "var(--aw-error)" };
    case "moved":
      return { Icon: FileOutput, color: "var(--aw-accent-purple)" };
    default:
      return { Icon: FileText, color: "var(--aw-text-tertiary)" };
  }
}

export function ArtifactCard({
  title,
  description,
  onClick,
  changeType,
  path,
}: ArtifactCardProps) {
  const { Icon, color } = getIconConfig(changeType);
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
        <Icon
          size={16}
          strokeWidth={1.85}
          style={{ flex: "0 0 auto", color }}
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
      {path ? (
        <span
          className="aw-truncate"
          style={{
            fontFamily: "var(--aw-font-mono)",
            fontSize: "var(--aw-font-size-xs)",
            lineHeight: "var(--aw-line-height-xs)",
            color: "var(--aw-text-tertiary)",
          }}
          title={path}
        >
          {path}
        </span>
      ) : null}
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
