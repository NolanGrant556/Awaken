import { beforeEach, describe, expect, it, vi } from "vitest";
import { RuntimeRequestError, runtimeRequestIssue, unwrapRuntimeRequest } from "../shared/runtime-request.js";
import { createRuntimeRequester, RuntimeDomainError } from "./runtime-http.js";
import { registerRuntimeRequest, runRuntimeRequest } from "./runtime-request.js";
import { ipcMain } from "electron";
vi.mock("electron", () => ({ ipcMain: { handle: vi.fn() } }));
beforeEach(() => { vi.restoreAllMocks(); });

describe("structured runtime requests", () => {
  it("keeps the error code and status independent of display language through serialization", async () => {
    const reply = await runRuntimeRequest(() => { throw new RuntimeDomainError("VERSION_CONFLICT", "内容已更新", 409); });
    const serialized = JSON.parse(JSON.stringify(reply));
    await expect(unwrapRuntimeRequest(Promise.resolve(serialized))).rejects.toMatchObject({ code: "VERSION_CONFLICT", status: 409, message: "内容已更新" });
    expect(runtimeRequestIssue(new RuntimeRequestError({ code: "TASK_ALREADY_RUNNING", message: "任意文案" })).code).toBe("TASK_ALREADY_RUNNING");
  });
  it("normalizes old validation details and removes local paths and credentials", async () => {
    const result = await runRuntimeRequest(() => { throw new RuntimeDomainError("VALIDATION_FAILED", "invalid name: token=secret /private/config/state.json", 400); });
    expect(result).toMatchObject({ ok: false, issue: { code: "VALIDATION_FAILED", details: { field: "name" } } });
    expect(JSON.stringify(result)).not.toContain("token=secret");
    expect(JSON.stringify(result)).not.toContain("/private/config");
  });
  it("wraps successful void IPC handlers without interpreting business result fields", async () => {
    registerRuntimeRequest("test:void", async () => undefined);
    const callback = vi.mocked(ipcMain.handle).mock.calls.at(-1)![1];
    await expect(callback({} as never)).resolves.toEqual({ ok: true, value: undefined });
    const business = { ok: false, code: "THREAD_BUSY" };
    await expect(unwrapRuntimeRequest(runRuntimeRequest(() => business))).resolves.toEqual(business);
  });
  it("uses the same HTTP error decoding for JSON and no-content requests", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ code: "THREAD_BUSY", message: "busy" }), { status: 409 })));
    const client = createRuntimeRequester((path) => `http://localhost${path}`);
    await expect(client.request("/run")).rejects.toMatchObject({ code: "THREAD_BUSY", status: 409 });
    await expect(client.requestVoid("/run")).rejects.toMatchObject({ code: "THREAD_BUSY", status: 409 });
    vi.unstubAllGlobals();
  });
});
