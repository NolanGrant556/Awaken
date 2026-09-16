import { ipcMain, type IpcMainInvokeEvent } from "electron";
import { OFFICE_ENVIRONMENT_CHANNELS as channels, type OfficeEnvironmentAction, type OfficeEnvironmentResult } from "../shared/office-environment.js";
import type { RuntimeControlClient } from "./runtime-control.js";

// Each window owns its HTTP waiter, not the shared Core preparation operation.
export function registerOfficeEnvironmentIpc(deps: {
  requireSender: (event: IpcMainInvokeEvent) => void;
  getClient: () => RuntimeControlClient | null | undefined;
}): () => void {
  const waiters = new Map<number, { id: string; controller: AbortController }>();
  const requireSender = (event: IpcMainInvokeEvent) => {
    deps.requireSender(event);
    if (event.senderFrame !== event.sender.mainFrame) throw new Error("办公环境操作必须来自 Awaken 主页面");
  };
  const client = () => {
    const value = deps.getClient();
    if (!value) throw new Error("本地服务尚未就绪，请稍后重试");
    return value;
  };
  ipcMain.handle(channels.status, (event) => { requireSender(event); return client().getOfficeEnvironment(); });
  ipcMain.handle(channels.cancel, (event, requestId: string) => {
    requireSender(event);
    const waiter = waiters.get(event.sender.id);
    if (waiter?.id === requestId) waiter.controller.abort();
  });
  ipcMain.handle(channels.prepare, async (event, request: OfficeEnvironmentAction): Promise<OfficeEnvironmentResult> => {
    requireSender(event);
    if (!request || typeof request.requestId !== "string" || !/^[a-zA-Z0-9-]{1,80}$/.test(request.requestId) ||
        !["prepare", "repair"].includes(request.action) ||
        Object.keys(request).some((key) => !["requestId", "action", "expectedInstanceId"].includes(key)) ||
        (request.action === "repair" && (typeof request.expectedInstanceId !== "string" || !/^[a-zA-Z0-9_-]{1,128}$/.test(request.expectedInstanceId))) ||
        (request.action === "prepare" && request.expectedInstanceId !== undefined)) {
      throw new Error("办公环境请求无效，请刷新后重试");
    }
    const sender = event.sender;
    if (waiters.has(sender.id)) throw new Error("当前窗口已经在等待办公环境准备");
    const controller = new AbortController();
    const cancel = () => controller.abort();
    waiters.set(sender.id, { id: request.requestId, controller });
    sender.once("destroyed", cancel);
    sender.once("render-process-gone", cancel);
    sender.once("did-navigate", cancel);
    try {
      await client().prepareOfficeEnvironment(request.action, request.expectedInstanceId, controller.signal, (operation) => {
        if (!controller.signal.aborted && !sender.isDestroyed()) sender.send(channels.progress, { requestId: request.requestId, operation });
      });
      return { status: controller.signal.aborted ? "cancelled" : "ready" };
    } catch (error) {
      return controller.signal.aborted ? { status: "cancelled" } : { status: "failed", message: error instanceof Error ? error.message : "办公环境准备失败，可以重试。" };
    } finally {
      sender.removeListener("destroyed", cancel);
      sender.removeListener("render-process-gone", cancel);
      sender.removeListener("did-navigate", cancel);
      waiters.delete(sender.id);
    }
  });
  return () => { for (const waiter of waiters.values()) waiter.controller.abort(); };
}
