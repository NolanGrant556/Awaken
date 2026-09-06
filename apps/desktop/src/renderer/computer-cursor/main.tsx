import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { ComputerCursorBridge, ComputerCursorHint } from "../../shared/computer-overlay";
import "./computer-cursor.css";

declare global {
  interface Window {
    awakenComputerCursor: ComputerCursorBridge;
  }
}

const cursorPath =
  "M5 3 L5 23.6 L9.8 18.9 L13.6 28.2 L17.2 26.6 L13.3 17.2 L21.4 17.2 Z";

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
