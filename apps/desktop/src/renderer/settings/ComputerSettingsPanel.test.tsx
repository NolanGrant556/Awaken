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
    const requestComputerPermission = vi.fn(async () => "granted" as const);
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
      requestComputerPermission,
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
      const onSettingsChange = latestSettingsProps?.onSettingsChange as (
        settings: {
          enabled: boolean;
          allow_background_window_actions: boolean;
        },
      ) => void;
      onSettingsChange({
        enabled: true,
        allow_background_window_actions: false,
      });
    });

    await waitForAssertion(() => {
      expect(prepareComputerRuntime).toHaveBeenCalledOnce();
      expect(getComputerHostStatus).toHaveBeenCalledTimes(3);
      expect(requestComputerPermission).not.toHaveBeenCalled();
      expect(latestSettingsProps?.settingsBusy).toBe(false);
      expect(latestSettingsProps?.settingsError).toBeNull();
    });
  });
});
