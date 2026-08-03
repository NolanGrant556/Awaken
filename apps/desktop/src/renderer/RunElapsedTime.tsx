import { useEffect, useState } from "react";

export type RunElapsedTimeProps = {
  startedAtMs?: number;
  completedAtMs?: number;
  elapsedMs?: number;
  running?: boolean;
};

function validTimestamp(value: number | undefined): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

export function formatRunElapsedTime(elapsedMs: number): string {
  const totalSeconds = Math.max(0, Math.floor(elapsedMs / 1000));
  const seconds = totalSeconds % 60;
  const totalMinutes = Math.floor(totalSeconds / 60);
  const minutes = totalMinutes % 60;
  const hours = Math.floor(totalMinutes / 60);

  if (hours > 0) {
    return `${hours} 小时 ${minutes} 分 ${seconds} 秒`;
  }
  if (totalMinutes > 0) {
    return `${totalMinutes} 分 ${String(seconds).padStart(2, "0")} 秒`;
  }
  return `${seconds} 秒`;
}

export function resolvedRunElapsedMs({
  startedAtMs,
  completedAtMs,
  elapsedMs,
  running = false,
  nowMs,
}: RunElapsedTimeProps & { nowMs: number }): number | undefined {
  if (validTimestamp(elapsedMs)) {
    return elapsedMs;
  }
  if (!validTimestamp(startedAtMs)) {
    return undefined;
  }
  if (validTimestamp(completedAtMs)) {
    return Math.max(0, completedAtMs - startedAtMs);
  }
  if (running) {
    return Math.max(0, nowMs - startedAtMs);
  }
  return undefined;
}

export function RunElapsedTime({
  startedAtMs,
  completedAtMs,
  elapsedMs,
  running = false,
}: RunElapsedTimeProps) {
  const [nowMs, setNowMs] = useState(() => Date.now());
  const live = running && validTimestamp(startedAtMs) && !validTimestamp(elapsedMs);

  useEffect(() => {
    if (!live) return;

    setNowMs(Date.now());
    const interval = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [live, startedAtMs]);

  const resolvedElapsedMs = resolvedRunElapsedMs({
    startedAtMs,
    completedAtMs,
    elapsedMs,
    running,
    nowMs,
  });
  if (resolvedElapsedMs === undefined) {
    return null;
  }

  const label = `执行时间 ${formatRunElapsedTime(resolvedElapsedMs)}`;
  return (
    <span className="run-elapsed-time" role="timer" aria-label={label}>
      {label}
    </span>
  );
}
