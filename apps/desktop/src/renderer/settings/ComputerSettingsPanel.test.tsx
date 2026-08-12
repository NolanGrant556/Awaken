// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeClient } from "../runtime/client";

let latestSettingsProps: Record<string, unknown> | null = null;

vi.mock("@awaken/design-system", async () => {
  const ReactModule = await import("react");
  return {
    AwakenComputerSettings: (props: Record<string, unknown>) => {
      latestSettingsProps = props;
      return ReactModule.createElement("div", {
        "data-testid": "computer-settings",
      });
    },
  };
});

import { ComputerSettingsPanel } from "./ComputerSettingsPanel";

async function waitForAssertion(assertion: () => void): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion();
      return;
    } catch (error) {
      lastError = error;
      await act(async () => {
        await new Promise((resolve) => window.setTimeout(resolve, 0));
      });
    }
  }
  throw lastError;
}

describe("ComputerSettingsPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    latestSettingsProps = null;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("continues to permission checks when the prepared runtime is ready", async () => {
    const unavailableStatus = {
      available: false,
      healthy: false,
      tcc_screen_recording: false,
      tcc_accessibility: false,
    };
    const readyStatus = {
      available: true,
      healthy: true,
      tcc_screen_recording: true,
      tcc_accessibility: true,
    };
    const getComputerHostStatus = vi
      .fn()
      .mockResolvedValueOnce(unavailableStatus)
      .mockResolvedValue(readyStatus);
    const prepareComputerRuntime = vi.fn(async () => ({
      node_ready: true,
      cua_host_ready: true,
    }));
    const requestComputerPermissions = vi.fn(async () => readyStatus);
    const runtimeClient = {
      getComputerHostStatus,
      getComputerSettings: vi.fn(async () => ({
        revision: 1,
        value: {
          enabled: false,
          allow_background_window_actions: false,
        },
      })),
      saveComputerSettings: vi.fn(async () => ({
        revision: 2,
        value: {
          enabled: true,
          allow_background_window_actions: false,
        },
      })),
      prepareComputerRuntime,
      requestComputerPermissions,
      openComputerPermissionSettings: vi.fn(async () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(<ComputerSettingsPanel runtimeClient={runtimeClient} />);
    });
    await waitForAssertion(() => {
      expect(latestSettingsProps?.settings).toEqual({
        enabled: false,
        allow_background_window_actions: false,
      });
    });

    await act(async () => {
      const onSettingsChange =
        latestSettingsProps?.onSettingsChange as (settings: {
          enabled: boolean;
          allow_background_window_actions: boolean;
        }) => void;
      onSettingsChange({
        enabled: true,
        allow_background_window_actions: false,
      });
    });

    await waitForAssertion(() => {
      expect(prepareComputerRuntime).toHaveBeenCalledOnce();
      expect(getComputerHostStatus).toHaveBeenCalledTimes(3);
      expect(requestComputerPermissions).not.toHaveBeenCalled();
      expect(latestSettingsProps?.settingsBusy).toBe(false);
      expect(latestSettingsProps?.settingsError).toBeNull();
    });
  });

  it("requests both Host permissions and continues to the next pane after focus returns", async () => {
    const missingBoth = {
      available: true,
      healthy: true,
      tcc_screen_recording: false,
      tcc_accessibility: false,
    };
    const missingAccessibility = {
      ...missingBoth,
      tcc_screen_recording: true,
    };
    const getComputerHostStatus = vi
      .fn()
      .mockResolvedValueOnce(missingBoth)
      .mockResolvedValueOnce(missingBoth)
      .mockResolvedValueOnce(missingAccessibility);
    const requestComputerPermissions = vi
      .fn()
      .mockResolvedValueOnce(missingBoth)
      .mockResolvedValueOnce(missingAccessibility);
    const openComputerPermissionSettings = vi.fn(async () => undefined);
    const runtimeClient = {
      getComputerHostStatus,
      getComputerSettings: vi.fn(async () => ({
        revision: 1,
        value: {
          enabled: false,
          allow_background_window_actions: false,
        },
      })),
      saveComputerSettings: vi.fn(async () => ({
        revision: 2,
        value: {
          enabled: true,
          allow_background_window_actions: false,
        },
      })),
      prepareComputerRuntime: vi.fn(async () => ({
        node_ready: true,
        cua_host_ready: true,
      })),
      requestComputerPermissions,
      openComputerPermissionSettings,
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(<ComputerSettingsPanel runtimeClient={runtimeClient} />);
    });
    await waitForAssertion(() => {
      expect(latestSettingsProps?.settings).toEqual({
        enabled: false,
        allow_background_window_actions: false,
      });
    });

    await act(async () => {
      const onSettingsChange =
        latestSettingsProps?.onSettingsChange as (settings: {
          enabled: boolean;
          allow_background_window_actions: boolean;
        }) => void;
      onSettingsChange({
        enabled: true,
        allow_background_window_actions: false,
      });
    });

    await waitForAssertion(() => {
      expect(requestComputerPermissions).toHaveBeenCalledTimes(1);
      expect(openComputerPermissionSettings).toHaveBeenNthCalledWith(
        1,
        "screen_recording",
      );
      expect(latestSettingsProps?.settingsBusy).toBe(false);
    });

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitForAssertion(() => {
      expect(requestComputerPermissions).toHaveBeenCalledTimes(2);
      expect(openComputerPermissionSettings).toHaveBeenNthCalledWith(
        2,
        "accessibility",
      );
    });
  });

  it("does not open System Settings until the user explicitly starts authorization", async () => {
    const missingAccessibility = {
      available: true,
      healthy: true,
      tcc_screen_recording: true,
      tcc_accessibility: false,
    };
    const requestComputerPermissions = vi.fn(async () => missingAccessibility);
    const openComputerPermissionSettings = vi.fn(async () => undefined);
    const saveComputerSettings = vi.fn();
    const prepareComputerRuntime = vi.fn(async () => ({
      node_ready: true,
      cua_host_ready: true,
    }));
    const runtimeClient = {
      getComputerHostStatus: vi.fn(async () => missingAccessibility),
      getComputerSettings: vi.fn(async () => ({
        revision: 4,
        value: {
          enabled: true,
          allow_background_window_actions: false,
        },
      })),
      saveComputerSettings,
      prepareComputerRuntime,
      requestComputerPermissions,
      openComputerPermissionSettings,
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(<ComputerSettingsPanel runtimeClient={runtimeClient} />);
    });

    await waitForAssertion(() => {
      expect(latestSettingsProps?.settings).toEqual({
        enabled: true,
        allow_background_window_actions: false,
      });
    });

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await new Promise((resolve) => window.setTimeout(resolve, 0));
    });

    expect(prepareComputerRuntime).not.toHaveBeenCalled();
    expect(requestComputerPermissions).not.toHaveBeenCalled();
    expect(openComputerPermissionSettings).not.toHaveBeenCalled();
    expect(saveComputerSettings).not.toHaveBeenCalled();

    await act(async () => {
      const onRequestPermissions =
        latestSettingsProps?.onRequestPermissions as () => void;
      onRequestPermissions();
    });

    await waitForAssertion(() => {
      expect(requestComputerPermissions).toHaveBeenCalledOnce();
      expect(openComputerPermissionSettings).toHaveBeenCalledWith(
        "accessibility",
      );
      expect(saveComputerSettings).not.toHaveBeenCalled();
    });
  });
});
