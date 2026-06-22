import "./AwakenArtifactGrid.css";
import type { ArtifactItem } from "../../types";
import { AwakenArtifactCard } from "../AwakenArtifactCard";

export type AwakenArtifactGridProps = {
  items: ArtifactItem[];
};

export function AwakenArtifactGrid({ items }: AwakenArtifactGridProps) {
  if (items.length === 0) {
    return (
      <div
        style={{
          padding: "28px 0",
          textAlign: "center",
          color: "var(--aw-text-tertiary)",
          fontSize: "var(--aw-font-size-sm)",
          lineHeight: "var(--aw-line-height-sm)",
          border: "1px dashed var(--aw-border-main)",
          borderRadius: "var(--aw-radius-md)",
        }}
      >
        暂无产物
      </div>
    );
  }

  return (
    <div
      role="list"
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
        gap: "12px",
      }}
    >
      {items.map((item) => (
        <div key={item.id} role="listitem" style={{ minWidth: 0 }}>
          <AwakenArtifactCard
            title={item.title}
            description={item.description}
            onClick={item.onClick}
            changeType={item.changeType}
            path={item.path}
          />
        </div>
      ))}
    </div>
  );
}
