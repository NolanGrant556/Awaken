import type { AwakenIconOp } from "./types";

export function awakenIconPaint(
  op: AwakenIconOp,
  filled: boolean,
): { fill: string; stroke: string } {
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
