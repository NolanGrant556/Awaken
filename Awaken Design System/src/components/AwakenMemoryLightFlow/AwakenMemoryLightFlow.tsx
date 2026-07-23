import { useEffect, useRef } from "react";
import {
  drawAwakenMemoryLightFlow,
  type AwakenMemoryLightFlowTheme,
} from "./AwakenMemoryLightFlow.model";
import "./AwakenMemoryLightFlow.css";

const TARGET_FRAME_RATE = 20;
const FRAME_INTERVAL_MS = 1000 / TARGET_FRAME_RATE;
const MAX_DEVICE_PIXEL_RATIO = 1.25;

export type AwakenMemoryLightFlowProps = {
  theme?: AwakenMemoryLightFlowTheme;
  className?: string;
  ariaLabel?: string;
};

export function AwakenMemoryLightFlow({
  theme = "light",
  className,
  ariaLabel,
}: AwakenMemoryLightFlowProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const rootClassName = [
    "aw-memory-light-flow",
    `aw-memory-light-flow--${theme}`,
    className,
  ].filter(Boolean).join(" ");

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return undefined;

    const context = canvas.getContext("2d", { alpha: false });
    if (!context) return undefined;

    const reducedMotionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    let reducedMotion = reducedMotionQuery.matches;
    let inViewport = true;
    let pageVisible = !document.hidden;
    let frameId = 0;
    let timerId = 0;
    let animationRunning = false;
    let startedAt = performance.now();
    let cssWidth = 0;
    let cssHeight = 0;

    const canAnimate = () => inViewport && pageVisible && !reducedMotion;

    const drawFrame = (elapsedMilliseconds: number) => {
      if (cssWidth <= 0 || cssHeight <= 0) return;
      drawAwakenMemoryLightFlow(
        context,
        cssWidth,
        cssHeight,
        elapsedMilliseconds,
        theme,
      );
    };

    const resize = () => {
      const bounds = canvas.getBoundingClientRect();
      const nextWidth = Math.max(1, Math.round(bounds.width));
      const nextHeight = Math.max(1, Math.round(bounds.height));
      const pixelRatio = Math.min(MAX_DEVICE_PIXEL_RATIO, window.devicePixelRatio || 1);
      if (nextWidth === cssWidth && nextHeight === cssHeight && canvas.width === Math.round(nextWidth * pixelRatio)) return;

      cssWidth = nextWidth;
      cssHeight = nextHeight;
      canvas.width = Math.round(cssWidth * pixelRatio);
      canvas.height = Math.round(cssHeight * pixelRatio);
      context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
      drawFrame(reducedMotion ? 0 : performance.now() - startedAt);
    };

    const stopAnimation = () => {
      if (frameId) window.cancelAnimationFrame(frameId);
      if (timerId) window.clearTimeout(timerId);
      frameId = 0;
      timerId = 0;
      animationRunning = false;
    };

    const scheduleNextFrame = () => {
      timerId = window.setTimeout(() => {
        timerId = 0;
        frameId = window.requestAnimationFrame(render);
      }, FRAME_INTERVAL_MS);
    };

    const render = (now: number) => {
      frameId = 0;
      if (!canAnimate()) {
        animationRunning = false;
        return;
      }
      drawFrame(now - startedAt);
      scheduleNextFrame();
    };

    const startAnimation = () => {
      if (animationRunning || !canAnimate()) return;
      animationRunning = true;
      frameId = window.requestAnimationFrame(render);
    };

    const handleMotionChange = (event: MediaQueryListEvent) => {
      reducedMotion = event.matches;
      startedAt = performance.now();
      drawFrame(0);
      if (reducedMotion) stopAnimation();
      else startAnimation();
    };

    const handleVisibilityChange = () => {
      pageVisible = !document.hidden;
      if (pageVisible) startAnimation();
      else stopAnimation();
    };

    const resizeObserver = new ResizeObserver(resize);
    const intersectionObserver = new IntersectionObserver((entries) => {
      inViewport = entries[0]?.isIntersecting ?? true;
      if (inViewport) startAnimation();
      else stopAnimation();
    });

    resizeObserver.observe(canvas);
    intersectionObserver.observe(canvas);
    reducedMotionQuery.addEventListener("change", handleMotionChange);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    window.addEventListener("resize", resize);
    resize();
    startAnimation();

    return () => {
      stopAnimation();
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      reducedMotionQuery.removeEventListener("change", handleMotionChange);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      window.removeEventListener("resize", resize);
    };
  }, [theme]);

  return (
    <div
      className={rootClassName}
      role={ariaLabel ? "img" : undefined}
      aria-label={ariaLabel}
      aria-hidden={ariaLabel ? undefined : true}
    >
      <canvas ref={canvasRef} className="aw-memory-light-flow__canvas" aria-hidden="true" />
    </div>
  );
}
