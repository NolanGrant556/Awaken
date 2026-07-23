// @vitest-environment jsdom

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeMemory, RuntimeProject } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

let latestMemorySettingsProps: Record<string, unknown> | null = null;

vi.mock("@awaken/design-system", async () => {
  const ReactModule = await import("react");
  return {
    AwakenMemoryLightFlowLayout: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement("div", null, children),
    AwakenMemorySettings: (props: Record<string, unknown>) => {
      latestMemorySettingsProps = props;
      return ReactModule.createElement("div", { "data-testid": "memory-settings" });
    },
  };
});

import { MemorySettingsPanel } from "./MemorySettingsPanel";

async function waitForAssertion(assertion: () => void): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion();
      return;
    } catch (error) {
      lastError = error;
      await act(async () => { await new Promise((resolve) => window.setTimeout(resolve, 0)); });
    }
  }
  throw lastError;
}

describe("MemorySettingsPanel recent activity behavior", () => {
  const roots: Array<ReturnType<typeof createRoot>> = [];

  afterEach(() => {
    roots.forEach((root) => act(() => root.unmount()));
    roots.length = 0;
    latestMemorySettingsProps = null;
  });

  it("does not replace global recent activity when the library switches to project scope", async () => {
    const userMemory: RuntimeMemory = {
      memoryId: "user-memory",
      name: "用户称呼",
      description: "称呼用户为 Nolan",
      type: "user",
      status: "active",
      revision: 1,
      updatedAt: "2026-07-23T08:00:00Z",
      sourceThreadId: "thread-user",
      sourceRecordId: "record-user",
      scope: "user",
    };
    const projectMemory: RuntimeMemory = {
      ...userMemory,
      memoryId: "project-memory",
      name: "项目约束",
      type: "project",
      sourceThreadId: "thread-project",
      sourceRecordId: "record-project",
      scope: "project",
    };
    const project: RuntimeProject = {
      projectId: "project-1",
      workspaceRoot: "/tmp/project-1",
      displayName: "项目一",
      state: "active",
      createdAt: "2026-07-23T08:00:00Z",
      updatedAt: "2026-07-23T08:00:00Z",
      version: 1,
    };
    const listMemoryCalls: Array<{ scope?: string; projectId?: string }> = [];
    const runtimeClient = {
      getMemorySettings: async () => ({
        revision: 1,
        value: { enabled: true, automaticRecall: true, proactiveMemory: true, autoDream: false },
      }),
      listProjects: async () => [project],
      listMemories: async (request: { scope?: string; projectId?: string }) => {
        listMemoryCalls.push(request);
        return request.projectId === project.projectId ? [projectMemory] : [userMemory];
      },
      listMemoryDreams: async () => [],
    } as unknown as RuntimeClient;
    const container = document.createElement("div");
    const root = createRoot(container);
    roots.push(root);

    await act(async () => { root.render(<MemorySettingsPanel runtimeClient={runtimeClient} />); });
    await waitForAssertion(() => {
      const activities = latestMemorySettingsProps?.activities as Array<{ id: string }> | undefined;
      expect(activities?.map((activity) => activity.id)).toEqual([
        "memory:user:user-memory",
        "memory:project-1:project-memory",
      ]);
    });
    const activityIdsBefore = (latestMemorySettingsProps?.activities as Array<{ id: string }>).map((activity) => activity.id);
    const callsBefore = listMemoryCalls.length;

    await act(async () => {
      const onFilterChange = latestMemorySettingsProps?.onFilterChange as (id: string, value: string) => void;
      onFilterChange("scope", "project");
    });
    await waitForAssertion(() => expect(latestMemorySettingsProps?.canStartDream).toBe(false));

    expect((latestMemorySettingsProps?.activities as Array<{ id: string }>).map((activity) => activity.id)).toEqual(activityIdsBefore);
    expect(listMemoryCalls).toHaveLength(callsBefore);
  });
});
