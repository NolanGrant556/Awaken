// @vitest-environment jsdom
import React from "react";
import { createRoot, type Root } from "react-dom/client";
import { act } from "react-dom/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import { HooksSettingsPanel } from "./HooksSettingsPanel";

describe("HooksSettingsPanel", () => {
  it("renders the no-config state when Core returns an empty source array", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const runtimeClient = {
      getHookStatus: vi.fn().mockResolvedValue({ snapshotId: "empty", schemaVersion: 1, createdAt: new Date().toISOString(), enabledHookCount: 0, sources: [] }),
      listProjects: vi.fn().mockResolvedValue([]),
    } as unknown as RuntimeClient;
    const root = createRoot(container);
    await act(async () => { root.render(<HooksSettingsPanel runtimeClient={runtimeClient} />); });
    expect(container.textContent).toContain("当前范围没有钩子配置。");
    await act(async () => { root.unmount(); });
    container.remove();
  });

  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => { container = document.createElement("div"); document.body.appendChild(container); root = createRoot(container); });
  afterEach(() => { act(() => root.unmount()); container.remove(); });

  it("shows project risk and trusts the current digest", async () => {
    const getHookStatus = vi.fn(async () => ({
      snapshotId: "snapshot-1", schemaVersion: 1, createdAt: "2026-08-12T00:00:00Z",
      sources: [{
        kind: "project" as const, path: "/work/.awaken/hooks.json", digest: "abcdef1234567890", status: "untrusted" as const, hookCount: 1,
        hooks: [{ id: "validate", event: "PreToolUse", type: "command" as const, toolNames: ["Write", "Edit"], command: ["node", "scripts/check.js"], cwd: "project_root", timeoutMs: 10000, maxOutputBytes: 65536, onFailure: "block" as const }],
      }],
    }));
    const trustProjectHook = vi.fn(async () => ({
      snapshotId: "snapshot-2", schemaVersion: 1, createdAt: "2026-08-12T00:00:01Z",
      sources: [{
        kind: "project" as const, path: "/work/.awaken/hooks.json", digest: "abcdef1234567890", status: "enabled" as const, hookCount: 1,
        hooks: [{ id: "validate", event: "PreToolUse", type: "command" as const, toolNames: ["Write", "Edit"], command: ["node", "scripts/check.js"], cwd: "project_root", timeoutMs: 10000, maxOutputBytes: 65536, onFailure: "block" as const }],
      }],
    }));
    const runtimeClient = {
      getHookStatus, trustProjectHook, revokeProjectHook: vi.fn(),
      listProjects: vi.fn(async () => [{ projectId: "project-1", displayName: "Awaken", state: "active", workspaceRoot: "/work", folders: [], createdAt: "", updatedAt: "", version: 1 }]),
    } as unknown as RuntimeClient;
    await act(async () => { root.render(<HooksSettingsPanel runtimeClient={runtimeClient} currentProjectId="project-1" />); });
    expect(container.textContent).toContain("以当前用户身份运行");
    expect(container.textContent).toContain("工具执行前");
    expect(container.textContent).toContain("validate");
    expect(container.textContent).toContain('["node","scripts/check.js"]');
    expect(container.textContent).toContain("Write | Edit");
    expect(container.textContent).toContain("10 秒");
    const button = [...container.querySelectorAll("button")].find((item) => item.textContent === "信任当前配置");
    expect(button).toBeTruthy();
    await act(async () => { button!.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
    expect(trustProjectHook).toHaveBeenCalledWith({ projectId: "project-1", sourcePath: "/work/.awaken/hooks.json", digest: "abcdef1234567890" });
    expect(container.textContent).toContain("只会影响后续新任务");
  });

  it("shows project-local Hook as a separately trustable source", async () => {
    const getHookStatus = vi.fn(async () => ({
      snapshotId: "snapshot-local", schemaVersion: 1, createdAt: "2026-08-18T00:00:00Z", enabledHookCount: 0,
      sources: [{
        kind: "project_local" as const, path: "/work/.awaken/hooks.local.json", digest: "local1234567890", status: "untrusted" as const, hookCount: 1,
        hooks: [{ id: "local-check", event: "RunStart", type: "prompt" as const, prompt: "Check local state", timeoutMs: 10000, maxOutputBytes: 65536, onFailure: "continue" as const }],
      }],
    }));
    const trustProjectHook = vi.fn(async () => ({ snapshotId: "trusted", schemaVersion: 1, createdAt: "2026-08-18T00:00:01Z", enabledHookCount: 1, sources: [] }));
    const runtimeClient = {
      getHookStatus, trustProjectHook, revokeProjectHook: vi.fn(),
      listProjects: vi.fn(async () => [{ projectId: "project-1", displayName: "Awaken", state: "active", workspaceRoot: "/work", folders: [], createdAt: "", updatedAt: "", version: 1 }]),
    } as unknown as RuntimeClient;
    await act(async () => { root.render(<HooksSettingsPanel runtimeClient={runtimeClient} currentProjectId="project-1" />); });
    expect(container.textContent).toContain("项目本地配置");
    expect(container.textContent).toContain("hooks.local.json");
    const button = [...container.querySelectorAll("button")].find((item) => item.textContent === "信任当前配置");
    await act(async () => { button!.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
    expect(trustProjectHook).toHaveBeenCalledWith({ projectId: "project-1", sourcePath: "/work/.awaken/hooks.local.json", digest: "local1234567890" });
  });
});
