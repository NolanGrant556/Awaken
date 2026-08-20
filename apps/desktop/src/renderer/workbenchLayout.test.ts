import { describe, expect, it } from "vitest";
import {
  DEFAULT_INSPECTOR_WIDTH,
  FILE_WORKSPACE_INSPECTOR_CAP,
  MAX_INSPECTOR_WIDTH,
  MIN_CENTER_READING_WIDTH,
  MIN_INSPECTOR_WIDTH,
  computeEffectiveInspectorWidth,
  inspectorWidthCap,
  sanitizeInspectorPreference,
} from "./workbenchLayout";

describe("workbenchLayout", () => {
  it("keeps a 1440 file workspace from crushing the 520 reading column", () => {
    const cap = inspectorWidthCap({
      fileWorkspaceOpen: true,
      preferenceWidth: DEFAULT_INSPECTOR_WIDTH,
    });
    expect(cap).toBe(FILE_WORKSPACE_INSPECTOR_CAP);

    const effective = computeEffectiveInspectorWidth({
      windowWidth: 1440,
      sidebarWidth: 300,
      sidebarCollapsed: false,
      inspectorAvailable: true,
      inspectorCollapsed: false,
      cap,
    });

    expect(effective).toBe(619);
    expect(1440 - 300 - 1 - effective).toBeGreaterThanOrEqual(MIN_CENTER_READING_WIDTH);
  });

  it("uses the ordinary preference cap outside the file workspace", () => {
    expect(
      inspectorWidthCap({
        fileWorkspaceOpen: false,
        preferenceWidth: 420,
      }),
    ).toBe(420);

    expect(
      computeEffectiveInspectorWidth({
        windowWidth: 1440,
        sidebarWidth: 300,
        sidebarCollapsed: false,
        inspectorAvailable: true,
        inspectorCollapsed: false,
        cap: 420,
      }),
    ).toBe(420);
  });

  it("clamps stored inspector preferences without rewriting a valid 720 preference", () => {
    expect(sanitizeInspectorPreference(FILE_WORKSPACE_INSPECTOR_CAP)).toBe(FILE_WORKSPACE_INSPECTOR_CAP);
    expect(sanitizeInspectorPreference(760)).toBe(MAX_INSPECTOR_WIDTH);
    expect(sanitizeInspectorPreference(200)).toBe(MIN_INSPECTOR_WIDTH);
  });

  it("returns zero when the inspector is collapsed or unavailable", () => {
    expect(
      computeEffectiveInspectorWidth({
        windowWidth: 1440,
        sidebarWidth: 300,
        sidebarCollapsed: false,
        inspectorAvailable: false,
        inspectorCollapsed: false,
        cap: 300,
      }),
    ).toBe(0);
    expect(
      computeEffectiveInspectorWidth({
        windowWidth: 1440,
        sidebarWidth: 300,
        sidebarCollapsed: false,
        inspectorAvailable: true,
        inspectorCollapsed: true,
        cap: 300,
      }),
    ).toBe(0);
  });

  it("lets a file-workspace override shrink without writing a new preference cap", () => {
    const cap = inspectorWidthCap({
      fileWorkspaceOpen: true,
      preferenceWidth: 300,
      overrideWidth: 400,
    });
    expect(cap).toBe(400);
    expect(
      computeEffectiveInspectorWidth({
        windowWidth: 1440,
        sidebarWidth: 300,
        sidebarCollapsed: false,
        inspectorAvailable: true,
        inspectorCollapsed: false,
        cap,
      }),
    ).toBe(400);
  });
});
