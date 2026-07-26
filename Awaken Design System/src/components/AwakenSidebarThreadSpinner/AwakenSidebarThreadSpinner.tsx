import "./AwakenSidebarThreadSpinner.css";

import { AwakenDotmSquare3 } from "../AwakenDotmSquare3/AwakenDotmSquare3";

export type AwakenSidebarThreadSpinnerProps = {
  label?: string;
  className?: string;
};

export function AwakenSidebarThreadSpinner({
  label = "加载中",
  className,
}: AwakenSidebarThreadSpinnerProps) {
  return (
    <AwakenDotmSquare3
      className={["aw-sidebar-thread-spinner", className].filter(Boolean).join(" ")}
      ariaLabel={label}
      size={14}
      dotSize={2}
    />
  );
}
