import type { CSSProperties, ReactNode, SVGProps } from "react";
import { awakenIconDrawing } from "../../icons/drawings";
import {
  awakenIconStrokeStyle,
  awakenIconViewBox,
  projectIconPointTo16,
  resolveAwakenIconOpticalSize,
} from "../../icons/stroke";
import type { AwakenIconId, AwakenIconOp, AwakenIconOpticalSize } from "../../icons/types";
import { isAwakenIconId } from "../../icons/types";
import "./AwakenIcon.css";

export interface AwakenIconProps extends Omit<SVGProps<SVGSVGElement>, "children" | "id"> {
  id: AwakenIconId;
  size?: number | string;
  opticalSize?: AwakenIconOpticalSize;
  filled?: boolean;
  title?: string;
  decorative?: boolean;
  lucideClassName?: string;
}

function scaleValue(value: number, opticalSize: AwakenIconOpticalSize): number {
  return opticalSize === 16 ? projectIconPointTo16(value) : value;
}

function pointsAttr(
  points: ReadonlyArray<readonly [number, number]>,
  opticalSize: AwakenIconOpticalSize,
): string {
  return points.map(([x, y]) => `${scaleValue(x, opticalSize)} ${scaleValue(y, opticalSize)}`).join(" ");
}

function paint(op: AwakenIconOp, filled: boolean): { fill: string; stroke: string } {
  if (op.kind === "line") {
    return { fill: "none", stroke: "currentColor" };
  }
  const isFilledDot = op.kind === "circle" && op.fillable;
  const isFilledShape = filled && op.fillable;
  if (isFilledDot || isFilledShape) {
    return { fill: "currentColor", stroke: "none" };
  }
  return { fill: "none", stroke: "currentColor" };
}

function renderOp(
  op: AwakenIconOp,
  opticalSize: AwakenIconOpticalSize,
  filled: boolean,
  key: string,
): ReactNode {
  if (op.detail === "24" && opticalSize === 16) {
    return null;
  }

  const { fill, stroke } = paint(op, filled);

  if (op.kind === "line") {
    return (
      <line
        key={key}
        x1={scaleValue(op.a[0], opticalSize)}
        y1={scaleValue(op.a[1], opticalSize)}
        x2={scaleValue(op.b[0], opticalSize)}
        y2={scaleValue(op.b[1], opticalSize)}
        fill="none"
        stroke="currentColor"
      />
    );
  }

  if (op.kind === "circle") {
    return (
      <circle
        key={key}
        cx={scaleValue(op.c[0], opticalSize)}
        cy={scaleValue(op.c[1], opticalSize)}
        r={scaleValue(op.r, opticalSize)}
        fill={fill}
        stroke={stroke}
      />
    );
  }

  if (op.kind === "rect") {
    return (
      <rect
        key={key}
        x={scaleValue(op.x, opticalSize)}
        y={scaleValue(op.y, opticalSize)}
        width={scaleValue(op.w, opticalSize)}
        height={scaleValue(op.h, opticalSize)}
        rx={op.r ? scaleValue(op.r, opticalSize) : undefined}
        fill={fill}
        stroke={stroke}
      />
    );
  }

  if (op.close) {
    return (
      <polygon
        key={key}
        points={pointsAttr(op.points, opticalSize)}
        fill={fill}
        stroke={stroke}
      />
    );
  }

  return (
    <polyline
      key={key}
      points={pointsAttr(op.points, opticalSize)}
      fill="none"
      stroke="currentColor"
    />
  );
}

export function AwakenIcon({
  id,
  size,
  opticalSize,
  filled = false,
  title,
  decorative = true,
  className,
  style,
  lucideClassName,
  strokeWidth: _strokeWidth,
  color,
  ...rest
}: AwakenIconProps) {
  if (!isAwakenIconId(id)) {
    throw new Error(`Unknown Awaken icon id: ${String(id)}`);
  }

  const resolvedOptical = opticalSize ?? resolveAwakenIconOpticalSize(size);
  const drawing = awakenIconDrawing(id);
  const numericSize = typeof size === "number" ? size : Number.parseFloat(String(size ?? ""));
  const displaySize = Number.isFinite(numericSize) ? numericSize : resolvedOptical;
  const labelled = Boolean(title) && !decorative;

  return (
    <svg
      {...rest}
      className={["aw-icon", `aw-icon--optical-${resolvedOptical}`, lucideClassName, className]
        .filter(Boolean)
        .join(" ")}
      width={displaySize}
      height={displaySize}
      viewBox={awakenIconViewBox(resolvedOptical)}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      role={labelled ? "img" : "presentation"}
      aria-hidden={labelled ? undefined : true}
      aria-label={labelled ? title : undefined}
      data-icon-id={id}
      data-icon-optical={resolvedOptical}
      data-icon-filled={filled ? "true" : "false"}
      style={{
        ...awakenIconStrokeStyle(resolvedOptical),
        ...(color ? { color } : null),
        ...(style as CSSProperties | undefined),
      }}
    >
      {title ? <title>{title}</title> : null}
      {drawing.ops.map((op, index) => renderOp(op, resolvedOptical, filled, `${id}-${index}`))}
    </svg>
  );
}
