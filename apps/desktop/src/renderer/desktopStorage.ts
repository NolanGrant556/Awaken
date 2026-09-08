export function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function readStoredNumber(
  key: string,
  fallback: number,
  min: number,
  max: number,
): number {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = Number(window.localStorage.getItem(key));
    if (!Number.isFinite(value)) {
      return fallback;
    }
    return clampNumber(value, min, max);
  } catch {
    return fallback;
  }
}

export function readStoredBoolean(key: string, fallback: boolean): boolean {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    if (value === null) {
      return fallback;
    }
    return value === "true";
  } catch {
    return fallback;
  }
}

export function readStoredString(key: string, fallback = ""): string {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    return typeof value === "string" ? value : fallback;
  } catch {
    return fallback;
  }
}

export function writeStoredValue(key: string, value: string): void {
  if (typeof window === "undefined" || !window.localStorage) {
    return;
  }
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Layout persistence is best-effort.
  }
}
