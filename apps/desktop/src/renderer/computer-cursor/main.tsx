import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { ComputerCursorBridge, ComputerCursorHint } from "../../shared/computer-overlay";
import "./computer-cursor.css";

declare global {
  interface Window {
    awakenComputerCursor: ComputerCursorBridge;
  }
}

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
      <path
        d="M1.2 1.2 L1.2 23.4 L8.1 17.8 L12.8 28.4 L17.2 26.5 L12.4 15.7 L21.2 15.7 Z"
        fill="#ffffff"
      />
      <path
        d="M3.1 3.6 L3.1 20.6 L8.4 16.2 L12.8 26.2 L15.4 25.1 L10.9 15 L18.4 15 Z"
        fill="#111111"
      />
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
