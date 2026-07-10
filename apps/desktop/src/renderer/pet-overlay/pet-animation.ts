import { useEffect, useRef, useState } from "react";
import {
  PET_ROWS,
  PET_SHEET_COLUMNS,
  PET_SHEET_ROWS,
  petSheetRowsForVersion,
  type PetAnimationState,
  type PetSpriteVersion,
} from "../../shared/pets";

export function framePosition(row: number, column: number, sheetRows = PET_SHEET_ROWS): string {
  return `${(column / (PET_SHEET_COLUMNS - 1)) * 100}% ${(row / (sheetRows - 1)) * 100}%`;
}

export function frameDurationMs(
  state: PetAnimationState,
  frameIndex: number,
): number {
  const row = PET_ROWS[state];
  const duration = row.frameDurationsMs[frameIndex] ?? row.frameDurationsMs[0];
  if (state === "idle") {
    return duration * 6;
  }
  return duration;
}

export function totalCycleDurationMs(state: PetAnimationState): number {
  const row = PET_ROWS[state];
  let total = 0;
  for (let i = 0; i < row.frames; i += 1) {
    total += frameDurationMs(state, i);
  }
  return total;
}

export type AnimationFrameInfo = {
  row: number;
  column: number;
  backgroundPosition: string;
};

export function frameAtElapsedMs(
  state: PetAnimationState,
  elapsedMs: number,
  sheetRows = PET_SHEET_ROWS,
): AnimationFrameInfo {
  const row = PET_ROWS[state];

  if (state === "idle") {
    const cycle = totalCycleDurationMs(state);
    const wrapped = cycle > 0 ? elapsedMs % cycle : 0;
    let accumulated = 0;
    for (let i = 0; i < row.frames; i += 1) {
      accumulated += frameDurationMs(state, i);
      if (wrapped < accumulated) {
        return {
          row: row.row,
          column: i,
          backgroundPosition: framePosition(row.row, i, sheetRows),
        };
      }
    }
    return {
      row: row.row,
      column: row.frames - 1,
      backgroundPosition: framePosition(row.row, row.frames - 1, sheetRows),
    };
  }

  // Codex plays a transient state three times, then falls back to its slow idle
  // loop until a new state arrives.
  const cycle = totalCycleDurationMs(state);
  const threeCycles = cycle * 3;
  if (elapsedMs >= threeCycles) {
    return frameAtElapsedMs("idle", elapsedMs - threeCycles, sheetRows);
  }
  const wrapped = cycle > 0 ? elapsedMs % cycle : 0;
  let accumulated = 0;
  for (let i = 0; i < row.frames; i += 1) {
    accumulated += frameDurationMs(state, i);
    if (wrapped < accumulated) {
      return {
        row: row.row,
        column: i,
        backgroundPosition: framePosition(row.row, i, sheetRows),
      };
    }
  }
  return {
    row: row.row,
    column: row.frames - 1,
    backgroundPosition: framePosition(row.row, row.frames - 1, sheetRows),
  };
}

export function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === "undefined" || !window.matchMedia) return false;
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return undefined;
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const listener = (event: MediaQueryListEvent): void => {
      setReduced(event.matches);
    };
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, []);

  return reduced;
}

export function usePetAnimation(state: PetAnimationState, spriteVersionNumber: PetSpriteVersion = 1): {
  backgroundPosition: string;
  row: number;
  column: number;
} {
  const sheetRows = petSheetRowsForVersion(spriteVersionNumber);
  const [frame, setFrame] = useState<AnimationFrameInfo>(() => ({
    row: PET_ROWS[state].row,
    column: 0,
    backgroundPosition: framePosition(PET_ROWS[state].row, 0, sheetRows),
  }));
  const reducedMotion = useReducedMotion();
  const startTimeRef = useRef<number>(0);
  const rafRef = useRef<number | null>(null);
  const stateRef = useRef<PetAnimationState>(state);

  useEffect(() => {
    stateRef.current = state;
    startTimeRef.current = performance.now();

    if (reducedMotion) {
      setFrame({
        row: PET_ROWS[state].row,
        column: 0,
        backgroundPosition: framePosition(PET_ROWS[state].row, 0, sheetRows),
      });
      return undefined;
    }

    const tick = (now: number): void => {
      const elapsed = now - startTimeRef.current;
      const nextFrame = frameAtElapsedMs(state, elapsed, sheetRows);
      setFrame(nextFrame);
      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current !== null) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = null;
      }
    };
  }, [state, reducedMotion, sheetRows]);

  return frame;
}
