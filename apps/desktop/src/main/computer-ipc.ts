import {
  ipcMain
} from "electron";
import {
  openComputerPermissionSettings,
  prepareComputerRuntime,
} from "./computer-setup.js";
import { RuntimeSupervisor } from "./runtime-supervisor.js";

type Dependencies = {
  readonly supervisor: RuntimeSupervisor;
};

export function registerComputerIpc(dependencies: Dependencies): void {
  ipcMain.handle(
    "awaken:resolve-computer-review",
    async (
      _event,
      payload: {
        runId: string;
        reviewId: string;
        decision: "approved_once" | "denied";
      },
    ) => {
      const client = dependencies.supervisor.getControlClient();
      if (!client) {
        throw new Error("runtime control client is not available");
      }
      await client.resolveComputerReview(payload.runId, payload.reviewId, payload.decision);
    },
  );

  ipcMain.handle(
    "awaken:computer-session-control",
    async (
      _event,
      payload: {
        runId: string;
        action: "takeover" | "return" | "pause" | "terminate";
      },
    ) => {
      const client = dependencies.supervisor.getControlClient();
      if (!client) {
        throw new Error("runtime control client is not available");
      }
      await client.computerSessionControl(payload.runId, payload.action);
    },
  );

  ipcMain.handle("awaken:get-computer-host-status", async () => {
    const client = dependencies.supervisor.getControlClient();
    if (!client) {
      throw new Error("runtime control client is not available");
    }
    return client.getComputerHostStatus();
  });

  ipcMain.handle("awaken:get-computer-settings", async () => {
    const client = dependencies.supervisor.getControlClient();
    if (!client) {
      throw new Error("runtime control client is not available");
    }
    return client.getComputerSettings();
  });

  ipcMain.handle(
    "awaken:save-computer-settings",
    async (
      _event,
      payload: {
        expected_revision: number;
        enabled?: boolean;
        allow_background_window_actions?: boolean;
      },
    ) => {
      const client = dependencies.supervisor.getControlClient();
      if (!client) {
        throw new Error("runtime control client is not available");
      }
      return client.saveComputerSettings(payload);
    },
  );

  ipcMain.handle("awaken:prepare-computer-runtime", async () => {
    return prepareComputerRuntime();
  });

  ipcMain.handle("awaken:request-computer-permissions", async () => {
    const client = dependencies.supervisor.getControlClient();
    if (!client) {
      throw new Error("runtime control client is not available");
    }
    return client.requestComputerPermissions();
  });

  ipcMain.handle(
    "awaken:open-computer-permission-settings",
    async (_event, permission: "screen_recording" | "accessibility") => {
      return openComputerPermissionSettings(permission);
    },
  );
}
