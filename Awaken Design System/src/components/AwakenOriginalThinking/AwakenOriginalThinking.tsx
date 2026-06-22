import { useEffect, useMemo, useRef } from "react";
import "./AwakenOriginalThinking.css";

const originalThinkingConfig = {
  name: "Original Thinking",
  tag: "Custom Rose Trail",
  rotate: true,
  particleCount: 64,
  trailSpan: 0.38,
  durationMs: 4600,
  rotationDurationMs: 28000,
  pulseDurationMs: 4200,
  strokeWidth: 5.5,
  baseRadius: 7,
  detailAmplitude: 3,
  petalCount: 7,
  curveScale: 3.9,
};

type OriginalThinkingPoint = {
  x: number;
  y: number;
};

type OriginalThinkingParticle = OriginalThinkingPoint & {
  radius: number;
  opacity: number;
};

export type AwakenOriginalThinkingProps = {
  className?: string;
};

function normalizeProgress(progress: number): number {
  return ((progress % 1) + 1) % 1;
}

function getDetailScale(time: number): number {
  const pulseProgress = (time % originalThinkingConfig.pulseDurationMs) / originalThinkingConfig.pulseDurationMs;
  const pulseAngle = pulseProgress * Math.PI * 2;
  return 0.52 + ((Math.sin(pulseAngle + 0.55) + 1) / 2) * 0.48;
}

function getRotation(time: number): number {
  if (!originalThinkingConfig.rotate) {
    return 0;
  }
  return -((time % originalThinkingConfig.rotationDurationMs) / originalThinkingConfig.rotationDurationMs) * 360;
}

function getPoint(progress: number, detailScale: number): OriginalThinkingPoint {
  const t = progress * Math.PI * 2;
  const petals = Math.round(originalThinkingConfig.petalCount);
  const x =
    originalThinkingConfig.baseRadius * Math.cos(t) -
    originalThinkingConfig.detailAmplitude * detailScale * Math.cos(petals * t);
  const y =
    originalThinkingConfig.baseRadius * Math.sin(t) -
    originalThinkingConfig.detailAmplitude * detailScale * Math.sin(petals * t);

  return {
    x: 50 + x * originalThinkingConfig.curveScale,
    y: 50 + y * originalThinkingConfig.curveScale,
  };
}

function buildPath(detailScale: number, steps = 480): string {
  return Array.from({ length: steps + 1 }, (_, index) => {
    const point = getPoint(index / steps, detailScale);
    return `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`;
  }).join(" ");
}

function getParticle(index: number, progress: number, detailScale: number): OriginalThinkingParticle {
  const tailOffset = index / (originalThinkingConfig.particleCount - 1);
  const point = getPoint(normalizeProgress(progress - tailOffset * originalThinkingConfig.trailSpan), detailScale);
  const fade = Math.pow(1 - tailOffset, 0.56);

  return {
    x: point.x,
    y: point.y,
    radius: 0.9 + fade * 2.7,
    opacity: 0.04 + fade * 0.96,
  };
}

function getFormula(): string {
  return [
    `x(t) = 50 + (${originalThinkingConfig.baseRadius.toFixed(1)} cos t - ${originalThinkingConfig.detailAmplitude.toFixed(1)}s cos ${Math.round(originalThinkingConfig.petalCount)}t) * ${originalThinkingConfig.curveScale.toFixed(1)}`,
    `y(t) = 50 + (${originalThinkingConfig.baseRadius.toFixed(1)} sin t - ${originalThinkingConfig.detailAmplitude.toFixed(1)}s sin ${Math.round(originalThinkingConfig.petalCount)}t) * ${originalThinkingConfig.curveScale.toFixed(1)}`,
    "s = detailScale(time)",
  ].join("\n");
}

export function AwakenOriginalThinking({ className }: AwakenOriginalThinkingProps) {
  const groupRef = useRef<SVGGElement | null>(null);
  const pathRef = useRef<SVGPathElement | null>(null);
  const particleRefs = useRef<Array<SVGCircleElement | null>>([]);
  const particleIndexes = useMemo(
    () => Array.from({ length: originalThinkingConfig.particleCount }, (_, index) => index),
    [],
  );
  const rootClassName = className ? `aw-original-thinking ${className}` : "aw-original-thinking";

  useEffect(() => {
    const startedAt = performance.now();
    let frameId = 0;

    function render(now: number): void {
      const group = groupRef.current;
      const path = pathRef.current;

      if (!group || !path) {
        frameId = requestAnimationFrame(render);
        return;
      }

      const time = now - startedAt;
      const progress = (time % originalThinkingConfig.durationMs) / originalThinkingConfig.durationMs;
      const detailScale = getDetailScale(time);

      group.setAttribute("transform", `rotate(${getRotation(time)} 50 50)`);
      path.setAttribute("d", buildPath(detailScale));

      particleRefs.current.forEach((node, index) => {
        if (!node) {
          return;
        }

        const particle = getParticle(index, progress, detailScale);
        node.setAttribute("cx", particle.x.toFixed(2));
        node.setAttribute("cy", particle.y.toFixed(2));
        node.setAttribute("r", particle.radius.toFixed(2));
        node.setAttribute("opacity", particle.opacity.toFixed(3));
      });

      frameId = requestAnimationFrame(render);
    }

    frameId = requestAnimationFrame(render);
    return () => cancelAnimationFrame(frameId);
  }, []);

  return (
    <div className={rootClassName}>
      <div className="aw-original-thinking__demo">
        <div className="aw-original-thinking__frame">
          <svg className="aw-original-thinking__svg" viewBox="0 0 100 100" fill="none" aria-hidden="true">
            <g ref={groupRef}>
              <path
                ref={pathRef}
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={originalThinkingConfig.strokeWidth}
                opacity="0.1"
              />
              {particleIndexes.map((index) => (
                <circle
                  key={index}
                  ref={(node) => {
                    particleRefs.current[index] = node;
                  }}
                  fill="currentColor"
                />
              ))}
            </g>
          </svg>
        </div>
        <div className="aw-original-thinking__meta">
          <div className="aw-original-thinking__title">{originalThinkingConfig.name}</div>
          <div className="aw-original-thinking__tag">{originalThinkingConfig.tag}</div>
        </div>
        <pre className="aw-original-thinking__formula">{getFormula()}</pre>
        <a
          className="aw-original-thinking__back-link"
          href="https://paidax01.github.io/math-curve-loaders/"
          target="_blank"
          rel="noreferrer"
        >
          View All
        </a>
      </div>
    </div>
  );
}
