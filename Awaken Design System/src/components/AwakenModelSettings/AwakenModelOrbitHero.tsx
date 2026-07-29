import { useEffect, useRef, type CSSProperties } from "react";
import { Cpu } from "lucide-react";
import {
  AwakenProviderLogo,
  type AwakenProviderBrand,
} from "../AwakenProviderLogo";

type OrbitTheme = "light" | "dark";

type OrbitProvider = {
  brand: AwakenProviderBrand;
  name: string;
  angle: number;
};

type OrbitDefinition = {
  id: string;
  direction: "cw" | "ccw";
  providers: OrbitProvider[];
};

const ORBITS: OrbitDefinition[] = [
  {
    id: "inner",
    direction: "cw",
    providers: [
      { brand: "qwen", name: "Qwen", angle: -45 },
      { brand: "doubao", name: "Doubao Ark", angle: 45 },
    ],
  },
  {
    id: "middle",
    direction: "ccw",
    providers: [
      { brand: "minimax", name: "MiniMax", angle: -45 },
      { brand: "glm", name: "GLM", angle: 45 },
    ],
  },
  {
    id: "outer",
    direction: "cw",
    providers: [
      { brand: "deepseek", name: "DeepSeek", angle: -55 },
      { brand: "kimi", name: "Kimi", angle: 55 },
    ],
  },
];

const POINT_COUNT = 420;
const TARGET_FRAME_RATE = 20;
const FRAME_INTERVAL_MS = 1000 / TARGET_FRAME_RATE;
const MAX_DEVICE_PIXEL_RATIO = 1.25;
const TAU = Math.PI * 2;
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));

const SPHERE_POINTS = Array.from({ length: POINT_COUNT }, (_, index) => {
  const y = 1 - (index / (POINT_COUNT - 1)) * 2;
  return {
    latitude: Math.acos(y),
    longitude: GOLDEN_ANGLE * index,
    size: 0.55 + ((index * 29) % 100) / 180,
    phase: ((index * 47) % 100) / 100,
  };
});

function ModelParticleSphere({ theme }: { theme: OrbitTheme }) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || typeof window.CanvasRenderingContext2D === "undefined") {
      return undefined;
    }
    const context = canvas.getContext("2d");
    if (!context) return undefined;

    const reducedMotionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    let reducedMotion = reducedMotionQuery.matches;
    let inViewport = true;
    let pageVisible = !document.hidden;
    let frameId = 0;
    let timerId = 0;
    let startedAt = performance.now();
    let width = 0;
    let height = 0;

    const draw = (elapsedMilliseconds: number) => {
      if (width <= 0 || height <= 0) return;
      const elapsed = reducedMotion ? 0 : elapsedMilliseconds / 1000;
      const radius = Math.min(width, height) * 0.43;
      const centerX = width / 2;
      const centerY = height / 2;
      const rotation = elapsed * 0.14;
      const particle = theme === "dark" ? "158, 143, 255" : "99, 79, 214";

      context.clearRect(0, 0, width, height);
      const halo = context.createRadialGradient(
        centerX,
        centerY,
        radius * 0.16,
        centerX,
        centerY,
        radius * 1.12,
      );
      halo.addColorStop(0, `rgba(${particle}, ${theme === "dark" ? 0.12 : 0.09})`);
      halo.addColorStop(0.72, `rgba(${particle}, ${theme === "dark" ? 0.04 : 0.025})`);
      halo.addColorStop(1, `rgba(${particle}, 0)`);
      context.fillStyle = halo;
      context.beginPath();
      context.arc(centerX, centerY, radius * 1.12, 0, TAU);
      context.fill();

      for (const point of SPHERE_POINTS) {
        const sinLatitude = Math.sin(point.latitude);
        const sourceX = Math.cos(point.longitude + rotation) * sinLatitude;
        const sourceY = Math.cos(point.latitude);
        const sourceZ = Math.sin(point.longitude + rotation) * sinLatitude;
        const perspective = 0.86 + sourceZ * 0.14;
        const x = centerX + sourceX * radius * perspective;
        const y = centerY + sourceY * radius * perspective;
        const depth = (sourceZ + 1) / 2;
        const pulse = reducedMotion
          ? 1
          : 0.82 + Math.sin(elapsed * 1.9 + point.phase * TAU) * 0.18;

        context.beginPath();
        context.arc(x, y, point.size * (0.72 + depth * 0.7) * pulse, 0, TAU);
        context.fillStyle = `rgba(${particle}, ${0.12 + depth * 0.64})`;
        context.fill();
      }
    };

    const resize = () => {
      const bounds = canvas.getBoundingClientRect();
      width = Math.max(1, Math.round(bounds.width));
      height = Math.max(1, Math.round(bounds.height));
      const pixelRatio = Math.min(
        MAX_DEVICE_PIXEL_RATIO,
        window.devicePixelRatio || 1,
      );
      canvas.width = Math.round(width * pixelRatio);
      canvas.height = Math.round(height * pixelRatio);
      context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
      draw(reducedMotion ? 0 : performance.now() - startedAt);
    };

    const stop = () => {
      if (frameId) window.cancelAnimationFrame(frameId);
      if (timerId) window.clearTimeout(timerId);
      frameId = 0;
      timerId = 0;
    };

    const render = (now: number) => {
      frameId = 0;
      if (!inViewport || !pageVisible || reducedMotion) return;
      draw(now - startedAt);
      timerId = window.setTimeout(() => {
        timerId = 0;
        frameId = window.requestAnimationFrame(render);
      }, FRAME_INTERVAL_MS);
    };

    const start = () => {
      if (frameId || timerId || !inViewport || !pageVisible || reducedMotion) return;
      frameId = window.requestAnimationFrame(render);
    };

    const resizeObserver = new ResizeObserver(resize);
    const intersectionObserver = new IntersectionObserver(([entry]) => {
      inViewport = entry?.isIntersecting ?? true;
      if (inViewport) start();
      else stop();
    });
    const handleVisibilityChange = () => {
      pageVisible = !document.hidden;
      if (pageVisible) start();
      else stop();
    };
    const handleMotionChange = (event: MediaQueryListEvent) => {
      reducedMotion = event.matches;
      startedAt = performance.now();
      stop();
      draw(0);
      start();
    };

    resizeObserver.observe(canvas);
    intersectionObserver.observe(canvas);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    reducedMotionQuery.addEventListener("change", handleMotionChange);
    window.addEventListener("resize", resize);
    resize();
    start();

    return () => {
      stop();
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      reducedMotionQuery.removeEventListener("change", handleMotionChange);
      window.removeEventListener("resize", resize);
    };
  }, [theme]);

  return (
    <canvas
      ref={canvasRef}
      className="aw-model-orbit__sphere-canvas"
      aria-hidden="true"
    />
  );
}

export function AwakenModelOrbitHero({
  theme,
  title,
}: {
  theme: OrbitTheme;
  title: string;
}) {
  return (
    <header className={`aw-model-orbit aw-model-orbit--${theme}`}>
      <h1 className="aw-model-orbit__title">
        <Cpu size={28} strokeWidth={1.8} aria-hidden="true" />
        <span>{title}</span>
      </h1>
      <div className="aw-model-orbit__visual" aria-hidden="true">
        <div className="aw-model-orbit__sphere">
          <ModelParticleSphere theme={theme} />
        </div>
        {ORBITS.map((orbit) => (
          <div
            key={orbit.id}
            className={`aw-model-orbit__ring aw-model-orbit__ring--${orbit.id}`}
          >
            {orbit.providers.map((provider) => (
              <div
                key={provider.brand}
                className={`aw-model-orbit__node aw-model-orbit__node--${orbit.direction}`}
                style={{ "--aw-model-orbit-angle": `${provider.angle}deg` } as CSSProperties}
              >
                <div
                  className={`aw-model-orbit__logo aw-model-orbit__logo--${orbit.direction}`}
                  style={{ "--aw-model-orbit-counter-angle": `${-provider.angle}deg` } as CSSProperties}
                >
                  <AwakenProviderLogo
                    brand={provider.brand}
                    name={provider.name}
                  />
                </div>
              </div>
            ))}
          </div>
        ))}
      </div>
    </header>
  );
}
