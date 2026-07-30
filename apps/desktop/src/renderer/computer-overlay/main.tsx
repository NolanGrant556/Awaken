import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Power } from "lucide-react";
import type {
  ComputerOverlayBridge,
  ComputerOverlayState,
} from "../../shared/computer-overlay";
import "./computer-overlay.css";

declare global {
  interface Window {
    awakenComputerOverlay: ComputerOverlayBridge;
  }
}

function ComputerOverlayApp() {
  const [state, setState] = useState<ComputerOverlayState | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    void window.awakenComputerOverlay.getState().then(setState);
    return window.awakenComputerOverlay.onStateChanged((nextState) => {
      setState(nextState);
      if (!nextState?.stopping) setMessage(null);
    });
  }, []);

  if (!state) return null;

  async function stop() {
    setMessage(null);
    const result = await window.awakenComputerOverlay.stop();
    if (!result.ok) setMessage(result.message ?? "停止失败，请返回 Awaken 重试。");
  }

  return (
    <main className="computer-overlay-root">
      <section className="computer-overlay-pill" aria-live="polite">
        <span className="computer-overlay-dot" aria-hidden="true" />
        <strong>{state.stopping ? "Awaken 正在停止计算机操作" : "Awaken 正在操作您的计算机"}</strong>
        <button
          type="button"
          className="computer-overlay-stop"
          disabled={state.stopping}
          aria-label="停止计算机操作"
          title="停止计算机操作"
          onClick={() => void stop()}
        >
          <Power size={18} strokeWidth={2.2} aria-hidden="true" />
        </button>
      </section>
      {message ? <p className="computer-overlay-error" role="alert">{message}</p> : null}
    </main>
  );
}

const container = document.getElementById("root");
if (!container) throw new Error("Computer overlay root element not found");

createRoot(container).render(
  <StrictMode>
    <ComputerOverlayApp />
  </StrictMode>,
);
