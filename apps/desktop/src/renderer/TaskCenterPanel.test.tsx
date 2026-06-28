import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { TaskCenterPanel } from "./TaskCenterPanel";
import { formatThreadUpdatedAt, splitTaskCenterThreads } from "./threadPresentation";
import type { ThreadItem } from "./runtime/reducer";

function makeThread(overrides: Partial<ThreadItem> = {}): ThreadItem {
  return {
    threadId: "thread-1",
    label: "First thread",
    runId: "run-1",
    runStatus: "finished",
    updatedAt: "2026-06-28T08:00:00Z",
    ...overrides,
  };
}

describe("threadPresentation", () => {
  it("groups active threads ahead of recent finished ones", () => {
    const grouped = splitTaskCenterThreads([
      makeThread({ threadId: "thread-2", runStatus: "finished", updatedAt: "2026-06-28T08:05:00Z" }),
      makeThread({ threadId: "thread-1", runStatus: "running", updatedAt: "2026-06-28T08:01:00Z" }),
    ]);

    expect(grouped.running.map((thread) => thread.threadId)).toEqual(["thread-1"]);
    expect(grouped.recent.map((thread) => thread.threadId)).toEqual(["thread-2"]);
  });

  it("formats recent timestamps for compact task-center labels", () => {
    expect(formatThreadUpdatedAt("2026-06-28T08:59:00Z", Date.parse("2026-06-28T09:00:00Z"))).toBe("1m ago");
    expect(formatThreadUpdatedAt("2026-06-28T08:00:00Z", Date.parse("2026-06-28T09:00:00Z"))).toBe("1h ago");
  });
});

describe("TaskCenterPanel", () => {
  it("renders an empty state when no threads exist", () => {
    const html = renderToString(
      <TaskCenterPanel
        open
        threads={[]}
        onRefresh={vi.fn()}
        onSelectThread={vi.fn()}
        onToggle={vi.fn()}
      />,
    );

    expect(html).toContain("No runs yet");
    expect(html).toContain("New agent runs will appear here");
  });

  it("renders running and recent sections with error details", () => {
    const html = renderToString(
      <TaskCenterPanel
        open
        activeThreadId="thread-1"
        threads={[
          makeThread({ threadId: "thread-1", runStatus: "running", updatedAt: "2026-06-28T08:59:00Z" }),
          makeThread({
            threadId: "thread-2",
            label: "Failed thread",
            runStatus: "error",
            lastError: "provider failed",
            updatedAt: "2026-06-28T08:40:00Z",
          }),
        ]}
        onRefresh={vi.fn()}
        onSelectThread={vi.fn()}
        onToggle={vi.fn()}
      />,
    );

    expect(html).toContain("Running");
    expect(html).toContain("Recent");
    expect(html).toContain("Current");
    expect(html).toContain("provider failed");
  });

  it("renders a panel-level error state when thread listing fails", () => {
    const html = renderToString(
      <TaskCenterPanel
        open
        errorMessage="Runtime unavailable"
        threads={[]}
        onRefresh={vi.fn()}
        onSelectThread={vi.fn()}
        onToggle={vi.fn()}
      />,
    );

    expect(html).toContain("Task list unavailable");
    expect(html).toContain("Runtime unavailable");
  });
});
