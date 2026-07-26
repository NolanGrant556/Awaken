"use client";

import type { CSSProperties } from "react";

import "./AwakenDotmSquare3.css";

const MATRIX_SIZE = 5;
const DOT_COUNT = MATRIX_SIZE * MATRIX_SIZE;

function buildSpiralInwardOrder(): number[] {
  const order = new Array<number>(DOT_COUNT);
  let top = 0;
  let bottom = MATRIX_SIZE - 1;
  let left = 0;
  let right = MATRIX_SIZE - 1;
  let step = 0;

  while (top <= bottom && left <= right) {
    for (let column = left; column <= right; column += 1) {
      order[top * MATRIX_SIZE + column] = step;
      step += 1;
    }

    for (let row = top + 1; row <= bottom; row += 1) {
      order[row * MATRIX_SIZE + right] = step;
      step += 1;
    }

    if (top < bottom) {
      for (let column = right - 1; column >= left; column -= 1) {
        order[bottom * MATRIX_SIZE + column] = step;
        step += 1;
      }
    }

    if (left < right) {
      for (let row = bottom - 1; row > top; row -= 1) {
        order[row * MATRIX_SIZE + left] = step;
        step += 1;
      }
    }

    top += 1;
    bottom -= 1;
    left += 1;
    right -= 1;
  }

  return order;
}

const SPIRAL_INWARD_ORDER = buildSpiralInwardOrder();

export type AwakenDotmSquare3Props = {
  size?: number;
  dotSize?: number;
  ariaLabel?: string;
  className?: string;
};

export function AwakenDotmSquare3({
  size = 36,
  dotSize = 5,
  ariaLabel = "加载中",
  className,
}: AwakenDotmSquare3Props) {
  const gap = Math.max(1, Math.floor((size - dotSize * MATRIX_SIZE) / (MATRIX_SIZE - 1)));
  const rootClassName = ["aw-dotm-square3", className].filter(Boolean).join(" ");

  return (
    <span
      role="status"
      aria-live="polite"
      aria-label={ariaLabel}
      className={rootClassName}
      style={{ width: size, height: size }}
    >
      <span className="aw-dotm-square3__grid" style={{ gap }} aria-hidden="true">
        {SPIRAL_INWARD_ORDER.map((order, index) => (
          <span
            key={index}
            className="aw-dotm-square3__dot"
            style={{
              width: dotSize,
              height: dotSize,
              "--aw-dotm-square3-order": order,
              "--aw-dotm-square3-progress": order / (DOT_COUNT - 1),
            } as CSSProperties}
          />
        ))}
      </span>
    </span>
  );
}
