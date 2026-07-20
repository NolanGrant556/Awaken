import "./AwakenSidebarThreadSpinner.css";

export type AwakenSidebarThreadSpinnerProps = {
  label?: string;
  className?: string;
};

export function AwakenSidebarThreadSpinner({
  label = "加载中",
  className,
}: AwakenSidebarThreadSpinnerProps) {
  return (
    <span
      className={["aw-sidebar-thread-spinner", className].filter(Boolean).join(" ")}
      aria-label={label}
    />
  );
}
