import "./AwakenConnectorBrandIcon.css";

export type AwakenConnectorBrandIconProps = {
  providerId: string;
  imageSrc?: string;
  label?: string;
  tone?: "dingtalk" | "lark" | "neutral";
  size?: "md" | "sm";
};

function fallbackLabel(providerId: string, label?: string): string {
  if (label?.trim()) return label.trim().slice(0, 1);
  if (providerId === "dingtalk-workspace") return "钉";
  if (providerId === "lark-cli") return "飞";
  return providerId.trim().slice(0, 1).toUpperCase() || "·";
}

export function AwakenConnectorBrandIcon({
  providerId,
  imageSrc,
  label,
  tone = "neutral",
  size = "md",
}: AwakenConnectorBrandIconProps) {
  return (
    <span
      className="aw-connector-brand-icon"
      data-tone={tone}
      data-size={size}
      aria-hidden="true"
    >
      {imageSrc ? (
        <img src={imageSrc} alt="" draggable={false} />
      ) : (
        <span>{fallbackLabel(providerId, label)}</span>
      )}
    </span>
  );
}
