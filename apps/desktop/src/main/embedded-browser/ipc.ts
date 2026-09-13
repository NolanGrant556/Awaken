import { ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from "electron";
import {
  EMBEDDED_BROWSER_CHANNELS,
  type EmbeddedBrowserTabRef,
  type EmbeddedBrowserViewRect,
} from "../../shared/embedded-browser.js";
import { EmbeddedBrowserViewManager } from "./manager.js";

export function registerEmbeddedBrowserIpc(options: {
  getWindow: () => BrowserWindow | null;
  manager: EmbeddedBrowserViewManager;
}): void {
  const requireSender = (event: IpcMainInvokeEvent) => {
    const window = options.getWindow();
    if (!window || window.isDestroyed() || event.sender.id !== window.webContents.id) {
      throw new Error("无法确认浏览器请求来自 Awaken 主窗口。");
    }
  };

  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.navigate, (event, ref: EmbeddedBrowserTabRef, url: string) => {
    requireSender(event);
    return options.manager.navigate(ref, url);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.back, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    return options.manager.back(ref);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.forward, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    return options.manager.forward(ref);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.reload, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    return options.manager.reload(ref);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.stop, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    return options.manager.stop(ref);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.close, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    options.manager.close(ref);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.closeThread, (event, threadId: string) => {
    requireSender(event);
    options.manager.closeThread(threadId);
  });
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.getState, (event, ref: EmbeddedBrowserTabRef) => {
    requireSender(event);
    return options.manager.getState(ref);
  });
  ipcMain.handle(
    EMBEDDED_BROWSER_CHANNELS.setViewport,
    (event, ref: EmbeddedBrowserTabRef, rect: EmbeddedBrowserViewRect | null) => {
      requireSender(event);
      options.manager.setViewport(ref, rect);
    },
  );
  ipcMain.handle(EMBEDDED_BROWSER_CHANNELS.setActive, (event, ref: EmbeddedBrowserTabRef | null) => {
    requireSender(event);
    options.manager.setActive(ref);
  });
}
