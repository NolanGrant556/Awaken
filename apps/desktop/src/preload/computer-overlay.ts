import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import {
  COMPUTER_OVERLAY_CHANNELS,
  type ComputerOverlayBridge,
  type ComputerOverlayState,
  type ComputerOverlayStopResult,
} from "../shared/computer-overlay.js";

const bridge: ComputerOverlayBridge = {
  getState: () =>
    ipcRenderer.invoke(COMPUTER_OVERLAY_CHANNELS.getState) as Promise<ComputerOverlayState | null>,
  onStateChanged: (callback) => {
    const handler = (_event: IpcRendererEvent, state: ComputerOverlayState | null) =>
      callback(state);
    ipcRenderer.on(COMPUTER_OVERLAY_CHANNELS.stateChanged, handler);
    return () => ipcRenderer.removeListener(COMPUTER_OVERLAY_CHANNELS.stateChanged, handler);
  },
  stop: () =>
    ipcRenderer.invoke(COMPUTER_OVERLAY_CHANNELS.stop) as Promise<ComputerOverlayStopResult>,
};

contextBridge.exposeInMainWorld("awakenComputerOverlay", bridge);
