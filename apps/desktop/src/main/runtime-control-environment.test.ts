import { expect, it, vi } from "vitest";
import { RuntimeControlClient } from "./runtime-control.js";

it("streams split UTF-8 progress with authenticated control and retains waiter cancellation", async () => {
  const encoder = new TextEncoder();
  const content = encoder.encode(JSON.stringify({ type: "progress", operation: { message: "正在验证环境" } }) + "\n" + JSON.stringify({ type: "ready", binding: { environment_ref: "ref", instance_id: "instance", executables: {} } }) + "\n");
  const fetchMock = vi.fn(async () => new Response(new ReadableStream({ start(controller) { for (let i = 0; i < content.length; i += 7) controller.enqueue(content.slice(i, i + 7)); controller.close(); } })));
  vi.stubGlobal("fetch", fetchMock);
  try {
    const client = new RuntimeControlClient({ schema_version: 1, runtime_identity: "test", instance_id: "test", pid: 1, owner_mode: "desktop", base_url: "http://127.0.0.1:9999", control_token: "test-token", started_at: new Date().toISOString() });
    const controller = new AbortController(); const progress = vi.fn();
    await expect(client.prepareEnvironment("excel.recalc", controller.signal, progress)).resolves.toMatchObject({ instance_id: "instance" });
    expect(progress).toHaveBeenCalledWith("正在验证环境");
    expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:9999/v1/control/environments/prepare", expect.objectContaining({ signal: controller.signal, headers: { Authorization: "Bearer test-token", "Content-Type": "application/json" }, body: JSON.stringify({ capability: "excel.recalc" }) }));
    fetchMock.mockImplementation(async () => new Response('{"type":"error","code":"environment_checksum_mismatch","message":"校验失败"}\n'));
    await expect(client.prepareEnvironment("excel.recalc", controller.signal)).rejects.toThrow("environment_checksum_mismatch");
  } finally { vi.unstubAllGlobals(); }
});


it("office repair sends only the selected instance token and preserves byte progress", async () => {
  const operation = { operation_id: "repair", attempt_id: "one", state: "downloading", message: "正在下载", downloaded_bytes: 1024, total_bytes: 4096 };
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ type: "progress", operation }) + "\n" + '{"type":"ready"}\n'));
  vi.stubGlobal("fetch", fetchMock);
  try {
    const client = new RuntimeControlClient({ schema_version: 1, runtime_identity: "test", instance_id: "test", pid: 1, owner_mode: "desktop", base_url: "http://127.0.0.1:9999", control_token: "test-token", started_at: "" });
    const controller = new AbortController(); const progress = vi.fn();
    await client.prepareOfficeEnvironment("repair", "old", controller.signal, progress);
    expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:9999/v1/control/environments/office/repair", expect.objectContaining({ signal: controller.signal, body: '{"expected_instance_id":"old"}', headers: { Authorization: "Bearer test-token", "Content-Type": "application/json" } }));
    expect(progress).toHaveBeenCalledWith(operation);
    fetchMock.mockImplementation(async () => new Response('{"type":"progress","operation":{}}\n'));
    await expect(client.prepareOfficeEnvironment("prepare", undefined, controller.signal, progress)).rejects.toThrow("连接中断");
  } finally { vi.unstubAllGlobals(); }
});

it("reads CLI ownership from authenticated Core projection", async () => {
  const installation = { executablePath: "/managed/dws", version: "1.0.32", healthy: true };
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ installation })));
  vi.stubGlobal("fetch", fetchMock);
  try {
    const client = new RuntimeControlClient({ schema_version: 1, runtime_identity: "test", instance_id: "test", pid: 1, owner_mode: "desktop", base_url: "http://127.0.0.1:9999", control_token: "test-token", started_at: "" });
    await expect(client.inspectCLIInstallation("dingtalk-workspace")).resolves.toEqual(installation);
    expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:9999/v1/control/environments/components/dingtalk-workspace", { headers: { Authorization: "Bearer test-token" } });
    fetchMock.mockImplementation(async () => new Response('{"installation":null}'));
    await expect(client.inspectCLIInstallation("dingtalk-workspace")).resolves.toBeNull();
    fetchMock.mockImplementation(async () => new Response("unavailable", { status: 503 }));
    await expect(client.inspectCLIInstallation("dingtalk-workspace")).rejects.toThrow("登记无法读取");
  } finally { vi.unstubAllGlobals(); }
});
