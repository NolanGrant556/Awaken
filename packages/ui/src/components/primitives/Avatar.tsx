type AvatarProps = {
  src?: string;
  fallback: string;
  size?: "sm" | "md";
};

export function Avatar({ src, fallback, size = "md" }: AvatarProps) {
  const dimension = size === "sm" ? 28 : 32;
  const letters = fallback.slice(0, 2).toUpperCase();

  return (
    <span
      style={{
        display: "inline-grid",
        placeItems: "center",
        width: dimension,
        height: dimension,
        borderRadius: "999px",
        background: "var(--aw-button-black)",
        color: "var(--aw-text-onblack)",
        fontSize: "var(--aw-font-size-xs)",
        fontWeight: "var(--aw-font-weight-bold)",
        overflow: "hidden",
        flexShrink: 0,
      }}
    >
      {src ? (
        <img
          src={src}
          alt=""
          style={{ width: "100%", height: "100%", objectFit: "cover" }}
        />
      ) : (
        letters
      )}
    </span>
  );
}
