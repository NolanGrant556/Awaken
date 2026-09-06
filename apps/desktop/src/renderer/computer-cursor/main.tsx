import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { ComputerCursorBridge, ComputerCursorHint } from "../../shared/computer-overlay";
import "./computer-cursor.css";

declare global {
  interface Window {
    awakenComputerCursor: ComputerCursorBridge;
  }
}

// 45° tip, bisector down-right. Trailing edge is a shallow inward notch —
// arrowhead only, no Windows-style stem.
const cursorPath =
  "M 6 4 L 12.5 19.7 L 15.1 13.1 L 21.7 10.5 Z";

function ComputerCursorApp() {
  const [hint, setHint] = useState<ComputerCursorHint | null>({ kind: "idle", pulse: false });

  useEffect(() => {
    return window.awakenComputerCursor.onHintChanged((nextHint) => {
      setHint(nextHint ?? { kind: "idle", pulse: false });
    });
  }, []);

  return (
    <svg
      className={hint?.pulse ? "computer-cursor is-pulse" : "computer-cursor"}
      viewBox="0 0 32 32"
      aria-hidden="true"
    >
      <path className="computer-cursor-outline" d={cursorPath} />
      <path className="computer-cursor-fill" d={cursorPath} />
    </svg>
  );
}

const container = document.getElementById("root");
if (!container) throw new Error("Computer cursor root element not found");

createRoot(container).render(
  <StrictMode>
    <ComputerCursorApp />
  </StrictMode>,
);
