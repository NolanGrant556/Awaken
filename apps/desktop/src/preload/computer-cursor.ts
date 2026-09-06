import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import {
  COMPUTER_CURSOR_CHANNELS,
  type ComputerCursorBridge,
  type ComputerCursorHint,
} from "../shared/computer-overlay.js";

const bridge: ComputerCursorBridge = {
  onHintChanged: (callback) => {
    const handler = (_event: IpcRendererEvent, hint: ComputerCursorHint | null) =>
      callback(hint);
    ipcRenderer.on(COMPUTER_CURSOR_CHANNELS.hintChanged, handler);
    return () => ipcRenderer.removeListener(COMPUTER_CURSOR_CHANNELS.hintChanged, handler);
  },
};

contextBridge.exposeInMainWorld("awakenComputerCursor", bridge);
