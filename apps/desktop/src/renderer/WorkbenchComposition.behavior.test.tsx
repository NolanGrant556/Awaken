/** @vitest-environment jsdom */
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { DesktopWorkbenchChrome } from "./DesktopWorkbenchChrome";
import { DesktopExecutionFeed } from "./DesktopExecutionFeed";
import { useFileWorkspaceSession } from "./useFileWorkspaceSession";
import { useWorkbenchPanels } from "./useWorkbenchPanels";
import { useNarrativeRenderer } from "./useNarrativeRenderer";
import { createInitialRuntimeState } from "./runtime/reducer";
import { createWorkbenchDisclosureStore } from "./workbenchDisclosureStore";
import type { NarrativeTurnSection } from "./workbenchPresentation";

describe("Desktop composition boundaries", () => {
  let host: HTMLDivElement, root: Root;
  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    localStorage.clear();
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
  });
  afterEach(async () => {
    await act(async () => root.unmount());
    host.remove();
    delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean })
      .IS_REACT_ACT_ENVIRONMENT;
  });
  async function mount(render: () => ReactNode) {
    function Harness() {
      return render();
    }
    await act(async () => root.render(<Harness />));
  }

  async function chrome() {
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    const registry = {
      workspaceLabelProjectId: "project",
      backendProjectIdFor: () => "project",
      projectLabelById: new Map([["project", "Fixture"]]),
      foldersForProjectId: () => [
        { path: "/tmp/fixture", role: "user" as const, isPrimary: true },
      ],
    } satisfies Parameters<typeof useFileWorkspaceSession>[0]["registry"];
    await mount(() => {
      files = useFileWorkspaceSession({
        registry,
        runtimeState: createInitialRuntimeState(),
        forceFull: false,
        hasDraft: false,
        isBusy: false,
        requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(),
        onBeforeFileExpand: vi.fn(),
      });
      return (
        <DesktopWorkbenchChrome
          fileWorkspace={files}
          platform="macos"
          currentConversationLabel="Fixture thread"
          handleNewTask={vi.fn()}
        />
      );
    });
    return () => files;
  }

  it("keeps scrollable tabs separate from fixed expand and panel controls", async () => {
    const files = await chrome();
    await act(async () => files().handleOpenFileWorkspace());
    const rail = host.querySelector(".project-files-tab-rail")!;
    const actions = host.querySelector(".project-files-chrome-actions")!;
    expect(rail.querySelector(".project-files-tabs")).not.toBeNull();
    expect(rail.contains(actions)).toBe(false);
    expect(
      actions.querySelector(".workbench-chrome-expand-toggle"),
    ).not.toBeNull();
    expect(
      actions.querySelector(".workbench-chrome-panel-toggle"),
    ).not.toBeNull();
  });

  it("preserves the mounted Artifact tab while selecting a project file", async () => {
    const files = await chrome();
    await act(async () =>
      files().handleOpenDeliverablePreview(
        {
          artifactId: "image",
          runId: "run",
          title: "render.png",
          fileType: "image",
        },
        "thread",
      ),
    );
    const artifactButton = host.querySelector<HTMLButtonElement>(
      '[role="tab"][title="render.png"]',
    )!;
    expect(artifactButton.getAttribute("aria-selected")).toBe("true");
    await act(async () =>
      files().handleOpenProjectFileTab({
        id: "file",
        projectId: "project",
        folderPath: "/tmp/fixture",
        relativePath: "notes.txt",
        name: "notes.txt",
        mode: "files",
      }),
    );
    expect(host.querySelector('[role="tab"][title="render.png"]')).toBe(
      artifactButton,
    );
    expect(artifactButton.getAttribute("aria-selected")).toBe("false");
    expect(
      host
        .querySelector('[role="tab"][title="notes.txt"]')
        ?.getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("retains the inspector button node and switches its action when collapsed", async () => {
    const files = await chrome();
    const toggle = host.querySelector<HTMLButtonElement>(
      ".workbench-chrome-panel-toggle",
    )!;
    const before = files().inspectorPresentation.effectiveCollapsed;
    await act(async () => toggle.click());
    expect(files().inspectorPresentation.effectiveCollapsed).toBe(!before);
    expect(host.querySelector(".workbench-chrome-panel-toggle")).toBe(toggle);
  });

  it("closes competing panels on media and subagent actions and clears selection on thread change", async () => {
    let panels!: ReturnType<typeof useWorkbenchPanels>;
    let threadId = "A";
    const closePlan = vi.fn();
    function Harness() {
      panels = useWorkbenchPanels(
        threadId,
        { closePanel: closePlan },
        () => "child",
        vi.fn(),
      );
      return null;
    }
    await act(async () => root.render(<Harness />));
    await act(async () => panels.setContextUsagePanelOpen(true));
    await act(async () => panels.handleOpenSubagent());
    expect(panels.contextUsagePanelOpen).toBe(false);
    expect(panels.openSubagentTaskId).toBe("child");
    await act(async () =>
      panels.openMediaPreview(
        [
          {
            id: "media",
            kind: "image",
            title: "image",
            sourceKey: "source",
            request: { threadId: "A", runId: "run", artifactId: "image" },
          },
        ],
        "media",
      ),
    );
    expect(panels.openSubagentTaskId).toBeUndefined();
    expect(closePlan).toHaveBeenCalledWith(false);
    threadId = "B";
    await act(async () => root.render(<Harness />));
    expect(panels.mediaPreview).toBeNull();
    expect(panels.contextUsagePanelOpen).toBe(false);
  });

  it("keeps turn disclosure identities stable when older history is inserted", async () => {
    const segment = vi.fn<
      ReturnType<typeof useNarrativeRenderer>["renderTurnBodySegment"]
    >(() => []);
    const turn = (id: string): NarrativeTurnSection => ({
      id,
      prompt: {
        kind: "prompt",
        id: `prompt-${id}`,
        text: id,
        runId: `run-${id}`,
      },
      items: [{ kind: "hint", id: `hint-${id}`, text: "done" }],
    });
    const props = {
      promptActions: {
        promptBubbleModeByTurnId: {},
        setPromptBubbleModeByTurnId: vi.fn(),
        editingPrompt: undefined,
        setEditingPrompt: vi.fn(),
        pendingPromptRewind: undefined,
        messageActionPendingRunId: undefined,
        requestPromptRewind: vi.fn(),
        dismissPromptRewindDialog: vi.fn(),
      },
      runtimeState: {
        activeThreadId: "thread",
        threadId: "thread",
        runStartedAtMs: undefined,
        runCompletedAtMs: undefined,
      },
      latestPromptId: "B",
      activeWorkbenchTurnId: "B",
      isBusy: false,
      latestTurnMinHeightPx: 0,
      composerPluginBrandByName: new Map<string, string>(),
      activeThreadSupportsMessageRewind: true,
      renderNarrativeItem: vi.fn(() => null),
      renderTurnBodySegment: segment,
    };
    await act(async () =>
      root.render(
        <DesktopExecutionFeed
          {...props}
          narrativeSections={{ leadItems: [], turns: [turn("B")] }}
        />,
      ),
    );
    const originalSection = host.querySelector('[data-execution-turn-id="B"]');
    const originalKey = segment.mock.calls[0][3];
    segment.mockClear();
    await act(async () =>
      root.render(
        <DesktopExecutionFeed
          {...props}
          narrativeSections={{ leadItems: [], turns: [turn("A"), turn("B")] }}
        />,
      ),
    );
    expect(host.querySelector('[data-execution-turn-id="B"]')).toBe(
      originalSection,
    );
    expect(segment.mock.calls.at(-1)?.[3]).toBe(originalKey);
    expect(originalKey).toContain("thread:thread:turn:B:");
  });

  it("renders reconnect in the narrative body without adding a detail overlay", async () => {
    await mount(() => {
      const renderer = useNarrativeRenderer({
        runtimeState: createInitialRuntimeState(),
        appearanceTheme: "light",
        platform: "macos",
        replayedNarrativeItemIds: new Set(),
        subagentNowMs: 0,
        handleOpenSubagent: vi.fn(),
        openMediaPreview: vi.fn(),
        handleOpenDeliverablePreview: vi.fn(),
        setNotice: vi.fn(),
        workbenchDisclosureStore: createWorkbenchDisclosureStore(),
        media: {
          artifactMediaSources: {},
          artifactMediaMetadata: {},
          openingArtifactId: null,
          recordArtifactMediaMetadata: vi.fn(),
          refreshArtifactMediaSource: vi.fn(),
          openArtifactExternally: async () => {},
          revealArtifact: async () => {},
        },
      });
      return (
        <div className="execution-turn-body">
          {renderer.renderNarrativeItem(
            {
              kind: "providerReconnect",
              id: "retry",
              phase: "reconnecting",
              attempt: 2,
              maxAttempts: 4,
            },
            "retry",
          )}
        </div>
      );
    });
    expect(host.querySelector(".execution-turn-body")?.textContent).toContain(
      "2/4",
    );
    expect(host.querySelector(".subagent-detail-overlay")).toBeNull();
  });
});
