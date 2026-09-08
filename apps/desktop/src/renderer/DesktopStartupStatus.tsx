import { useEffect, useState } from "react";

export function DesktopStartupStatus({
  starting,
  placement = "brand",
}: {
  starting: boolean;
  placement?: "brand" | "inline";
}) {
  const [visible, setVisible] = useState(starting);
  useEffect(() => {
    if (starting) {
      setVisible(true);
      return;
    }
    const timer = setTimeout(() => setVisible(false), 250);
    return () => clearTimeout(timer);
  }, [starting]);
  if (!visible && !starting) return null;
  return (
    <span
      className={`desktop-startup-status is-${placement}${starting ? "" : " is-leaving"}`}
      role="status"
      aria-live="polite"
    >
      {starting ? "正在启动 Awaken…" : "已准备就绪"}
    </span>
  );
}
