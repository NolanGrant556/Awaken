type DividerProps = {
  spacing?: "sm" | "md" | "lg";
};

const spacingMap = {
  sm: "var(--aw-space-3) 0",
  md: "var(--aw-space-5) 0",
  lg: "var(--aw-space-8) 0",
};

export function Divider({ spacing = "md" }: DividerProps) {
  return (
    <hr
      style={{
        width: "100%",
        height: 0,
        margin: spacingMap[spacing],
        border: 0,
        borderTop: "1px solid var(--aw-border-main)",
      }}
    />
  );
}
