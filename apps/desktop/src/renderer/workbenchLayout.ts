export const DEFAULT_INSPECTOR_WIDTH = 300;
export const MIN_INSPECTOR_WIDTH = 260;
export const MAX_INSPECTOR_WIDTH = 760;
export const FILE_WORKSPACE_INSPECTOR_CAP = 720;
export const MIN_CENTER_READING_WIDTH = 520;
export const LAYOUT_SEPARATOR_TOTAL = 1;
export const DEFAULT_WINDOW_WIDTH = 1440;

export function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function normalizeInspectorPreference(value: number): number {
  return clampNumber(value, MIN_INSPECTOR_WIDTH, MAX_INSPECTOR_WIDTH);
}

export function sanitizeInspectorPreference(stored: number): number {
  return normalizeInspectorPreference(stored);
}

export function inspectorWidthCap(input: {
  fileWorkspaceOpen: boolean;
  preferenceWidth: number;
  overrideWidth?: number;
}): number {
  if (!input.fileWorkspaceOpen) {
    return normalizeInspectorPreference(input.preferenceWidth);
  }
  const override = input.overrideWidth;
  if (typeof override === "number" && Number.isFinite(override)) {
    return clampNumber(override, MIN_INSPECTOR_WIDTH, FILE_WORKSPACE_INSPECTOR_CAP);
  }
  return FILE_WORKSPACE_INSPECTOR_CAP;
}

export function computeEffectiveInspectorWidth(input: {
  windowWidth: number;
  sidebarWidth: number;
  sidebarCollapsed: boolean;
  inspectorAvailable: boolean;
  inspectorCollapsed: boolean;
  cap: number;
}): number {
  if (!input.inspectorAvailable || input.inspectorCollapsed) {
    return 0;
  }

  const sidebarOrZero = input.sidebarCollapsed ? 0 : input.sidebarWidth;
  const available = input.windowWidth - sidebarOrZero - MIN_CENTER_READING_WIDTH - LAYOUT_SEPARATOR_TOTAL;
  return clampNumber(Math.min(input.cap, available), MIN_INSPECTOR_WIDTH, input.cap);
}

export function readWindowWidth(): number {
  if (typeof window === "undefined" || !Number.isFinite(window.innerWidth) || window.innerWidth <= 0) {
    return DEFAULT_WINDOW_WIDTH;
  }
  return window.innerWidth;
}
