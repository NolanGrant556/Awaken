import { useEffect, useState } from "react";

export function DesktopStartupStatus({
  starting,
  needsRestart = false,
  placement = "brand",
}: {
  starting: boolean;
  needsRestart?: boolean;
  placement?: "brand" | "inline";
}) {
  const [visible, setVisible] = useState(starting || needsRestart);
  useEffect(() => {
    if (starting || needsRestart) {
      setVisible(true);
      return;
    }
    const timer = setTimeout(() => setVisible(false), 250);
    return () => clearTimeout(timer);
  }, [starting, needsRestart]);
  if (!visible && !starting && !needsRestart) return null;
  return (
    <span
      className={`desktop-startup-status is-${placement}${needsRestart ? " is-error" : ""}${starting || needsRestart ? "" : " is-leaving"}`}
      role={needsRestart ? "alert" : "status"}
      aria-live={needsRestart ? "assertive" : "polite"}
    >
      {needsRestart ? "Awaken 需要重新启动，请退出后重新打开。" : starting ? "正在启动 Awaken…" : "已准备就绪"}
    </span>
  );
}
