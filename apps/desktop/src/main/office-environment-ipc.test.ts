import { EventEmitter } from "node:events";
import { beforeEach, expect, it, vi } from "vitest";
import { OFFICE_ENVIRONMENT_CHANNELS as channels } from "../shared/office-environment.js";
import type { RuntimeControlClient } from "./runtime-control.js";
const handlers = vi.hoisted(() => new Map<string, (...args: any[]) => any>());
vi.mock("electron", () => ({ ipcMain: { handle: (name: string, handler: (...args: any[]) => any) => handlers.set(name, handler) } }));
import { registerOfficeEnvironmentIpc } from "./office-environment-ipc.js";
function setup() {
  const sender = Object.assign(new EventEmitter(), { id: 1, mainFrame: {}, send: vi.fn(), isDestroyed: () => false });
  const event = { sender, senderFrame: sender.mainFrame };
  let signal: AbortSignal;
  const client = { getOfficeEnvironment: vi.fn(), prepareOfficeEnvironment: vi.fn((_action, _expected, value: AbortSignal, progress) => {
    signal = value;
    progress({ state: "preparing", downloaded_bytes: 20, total_bytes: 40 });
    return new Promise<void>((_resolve,reject) => value.addEventListener("abort", () => reject(new Error("aborted")), { once: true }));
  }) };
  const stop = registerOfficeEnvironmentIpc({ requireSender: (e) => { if (e.sender.id !== 1) throw new Error("untrusted"); }, getClient: () => client as unknown as RuntimeControlClient });
  return { event, sender, client, stop, signal: () => signal };
}
beforeEach(() => handlers.clear());
it("cancels only matching window waiter and forwards scoped progress", async () => {
  const test = setup();
  const pending = handlers.get(channels.prepare)!(test.event, { requestId: "first", action: "repair", expectedInstanceId: "old" });
  expect(test.sender.send).toHaveBeenCalledWith(channels.progress, expect.objectContaining({ requestId: "first", operation: expect.objectContaining({ downloaded_bytes: 20 }) }));
  handlers.get(channels.cancel)!(test.event, "foreign");
  expect(test.signal().aborted).toBe(false);
  await expect(handlers.get(channels.prepare)!(test.event, { requestId: "second", action: "prepare" })).rejects.toThrow("已经在等待");
  handlers.get(channels.cancel)!(test.event, "first");
  await expect(pending).resolves.toEqual({ status: "cancelled" });
  expect(test.sender.listenerCount("destroyed")).toBe(0);
  const next = handlers.get(channels.prepare)!(test.event, { requestId: "third", action: "prepare" });
  test.stop();
  await expect(next).resolves.toEqual({ status: "cancelled" });
});
it("rejects foreign senders, subframes, arbitrary install inputs and disconnects crashed renderer", async () => {
  const test = setup();
  expect(() => handlers.get(channels.status)!({sender:{id:2}})).toThrow("untrusted");
  await expect(handlers.get(channels.prepare)!({...test.event,senderFrame:{}},{requestId:"a",action:"prepare"})).rejects.toThrow("主页面");
  await expect(handlers.get(channels.prepare)!(test.event,{requestId:"a",action:"repair",expectedInstanceId:"../tmp"})).rejects.toThrow("无效");
  await expect(handlers.get(channels.prepare)!(test.event,{requestId:"a",action:"prepare",directory:"/tmp"})).rejects.toThrow("无效");
  expect(test.client.prepareOfficeEnvironment).not.toHaveBeenCalled();
  const pending = handlers.get(channels.prepare)!(test.event,{requestId:"a",action:"prepare"});
  test.sender.emit("render-process-gone");
  await expect(pending).resolves.toEqual({status:"cancelled"});
});
