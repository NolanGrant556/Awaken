import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { RuntimeControlClient } from "./runtime-control.js";

describe("RuntimeControlClient", () => {
  it("reads a manifest from the userData runtime directory", async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), "awaken-runtime-control-"));
    try {
      const manifest = {
        schema_version: 1,
        runtime_identity: "packaged:abc",
        instance_id: "core_test",
        pid: 1,
        owner_mode: "desktop" as const,
        base_url: "http://127.0.0.1:9999",
        control_token: "secret",
        started_at: new Date().toISOString(),
      };
      await mkdir(join(userDataDir, "runtime"), { recursive: true });
      await writeFile(
        join(userDataDir, "runtime", "core.json"),
        JSON.stringify(manifest),
        "utf8",
      );
      const read = await RuntimeControlClient.readManifest(userDataDir);
      expect(read).toEqual(manifest);
    } finally {
      await rm(userDataDir, { recursive: true, force: true });
    }
  });

  it("returns undefined when no manifest exists", async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), "awaken-runtime-control-"));
    try {
      const read = await RuntimeControlClient.readManifest(userDataDir);
      expect(read).toBeUndefined();
    } finally {
      await rm(userDataDir, { recursive: true, force: true });
    }
  });

  it("hot reloads one Agent Plan media capability through the control route", async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    try {
      const client = new RuntimeControlClient({
        schema_version: 1,
        runtime_identity: "packaged:abc",
        instance_id: "core_test",
        pid: 1,
        owner_mode: "desktop",
        base_url: "http://127.0.0.1:9999",
        control_token: "secret",
        started_at: new Date().toISOString(),
      });

      await client.reloadAgentPlanMediaTool("media.video.generate");

      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:9999/v1/control/agent-plan/media-tools/reload",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ capability: "media.video.generate" }),
        }),
      );
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it("sends the original Computer session identity with return control", async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    try {
      const client = new RuntimeControlClient({ schema_version: 1, runtime_identity: "test", instance_id: "test", pid: 1, owner_mode: "desktop", base_url: "http://127.0.0.1:9999", control_token: "test-token", started_at: new Date().toISOString() });
      await client.computerSessionControl("old-run", "return", "child-session");
      expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:9999/v1/control/computer/sessions/old-run/return", expect.objectContaining({ body: JSON.stringify({ session_id: "child-session" }) }));
    } finally { vi.unstubAllGlobals(); }
  });

});
