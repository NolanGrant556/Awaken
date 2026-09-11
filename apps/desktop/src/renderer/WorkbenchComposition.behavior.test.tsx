/** @vitest-environment jsdom */
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { DesktopInspectorRail } from "./DesktopInspectorRail";
import { DesktopWorkbenchChrome } from "./DesktopWorkbenchChrome";
import { DesktopExecutionFeed } from "./DesktopExecutionFeed";
import { useFileWorkspaceSession } from "./useFileWorkspaceSession";
import { useWorkbenchPanels } from "./useWorkbenchPanels";
import { useNarrativeRenderer } from "./useNarrativeRenderer";
import { createInitialRuntimeState } from "./runtime/reducer";
import { createWorkbenchDisclosureStore } from "./workbenchDisclosureStore";
import type { NarrativeTurnSection } from "./workbenchPresentation";
import { SubagentPanel } from "./SubagentPanel";
import type { SubagentTaskView } from "./runtime/reducer";

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
      const panels = useWorkbenchPanels(undefined, { closePanel: vi.fn() }, vi.fn());
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
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
          subagentTabs={[]}
          onOpenSubagent={vi.fn()}
        />
      );
    });
    return () => files;
  }

  it("places stop-all on the center subagent group instead of the chrome", async () => {
    const stopAll = vi.fn();
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    await mount(() => {
      const panels = useWorkbenchPanels("thread", { closePanel: vi.fn() }, vi.fn());
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
        registry: {
          workspaceLabelProjectId: "project",
          backendProjectIdFor: () => "project",
          projectLabelById: new Map([["project", "Fixture"]]),
          foldersForProjectId: () => [],
        },
        runtimeState: { ...createInitialRuntimeState(), activeThreadId: "thread" },
        forceFull: false,
        hasDraft: false,
        isBusy: false,
        requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(),
        onBeforeFileExpand: vi.fn(),
      });
      const renderer = useNarrativeRenderer({
        runtimeState: createInitialRuntimeState(),
        appearanceTheme: "light",
        platform: "macos",
        replayedNarrativeItemIds: new Set(),
        subagentNowMs: 0,
        handleOpenSubagent: vi.fn(),
        onStopAllAgents: stopAll,
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
        <>
          <DesktopWorkbenchChrome
            fileWorkspace={files}
            platform="macos"
            currentConversationLabel="主任务"
            handleNewTask={vi.fn()}
            subagentTabs={[]}
            onOpenSubagent={vi.fn()}
          />
          {renderer.renderNarrativeItem(
            {
              kind: "subagentGroup",
              id: "group",
              tasks: [
                {
                  taskId: "child",
                  description: "运行 worker.py",
                  agentType: "general",
                  status: "running",
                  toolUseCount: 1,
                },
              ],
            },
            "group",
          )}
        </>
      );
    });
    await act(async () => files.handleOpenFileWorkspace());
    expect(host.querySelector('[aria-label="停止全部任务"]')).toBeNull();
    const button = host.querySelector<HTMLButtonElement>(
      ".aw-subagent-group__stop-all",
    );
    expect(button?.textContent).toBe("停止全部任务");
    await act(async () => button?.click());
    expect(stopAll).toHaveBeenCalledTimes(1);
  });

  it("keeps inspector expand per thread and does not carry it to the next session", async () => {
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    function Harness({ threadId }: { threadId?: string }) {
      const panels = useWorkbenchPanels(
        threadId,
        { closePanel: vi.fn() },
        vi.fn(),
      );
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
        registry: {
          workspaceLabelProjectId: "project",
          backendProjectIdFor: () => "project",
          projectLabelById: new Map([["project", "Fixture"]]),
          foldersForProjectId: () => [],
        },
        runtimeState: {
          ...createInitialRuntimeState(),
          activeThreadId: threadId,
        },
        forceFull: false,
        hasDraft: false,
        isBusy: false,
        requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(),
        onBeforeFileExpand: vi.fn(),
      });
      return null;
    }
    await act(async () => root.render(<Harness threadId="thread-a" />));
    await act(async () => files.handleOpenFileWorkspace());
    expect(files.inspectorCollapsed).toBe(false);
    await act(async () => root.render(<Harness threadId="thread-b" />));
    expect(files.inspectorCollapsed).toBe(true);
    expect(files.fileWorkspaceExpanded).toBe(false);
    await act(async () => root.render(<Harness threadId="thread-a" />));
    expect(files.inspectorCollapsed).toBe(false);
  });

  it("does not collapse the inspector when a new thread first receives an id", async () => {
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    function Harness({ threadId }: { threadId?: string }) {
      const panels = useWorkbenchPanels(
        threadId,
        { closePanel: vi.fn() },
        vi.fn(),
      );
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
        registry: {
          workspaceLabelProjectId: "project",
          backendProjectIdFor: () => "project",
          projectLabelById: new Map([["project", "Fixture"]]),
          foldersForProjectId: () => [],
        },
        runtimeState: {
          ...createInitialRuntimeState(),
          activeThreadId: threadId,
        },
        forceFull: false,
        hasDraft: false,
        isBusy: false,
        requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(),
        onBeforeFileExpand: vi.fn(),
      });
      return null;
    }
    await act(async () => root.render(<Harness />));
    await act(async () => files.handleOpenFileWorkspace());
    expect(files.inspectorCollapsed).toBe(false);
    await act(async () => root.render(<Harness threadId="created" />));
    expect(files.inspectorCollapsed).toBe(false);
  });

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

  it("opens child tabs beside files without a child composer and keeps the exact failed assignment", async () => {
    let panels!: ReturnType<typeof useWorkbenchPanels>;
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    const cancel = vi.fn();
    const tasks: SubagentTaskView[] = [
      { taskId: "failed", agentId: "same-agent", description: "读取样本", agentType: "general", status: "failed", toolUseCount: 4,
        errorCode: "read_failed", errorMessage: "原分配的失败", narrative: createInitialRuntimeState().narrative },
      { taskId: "done", agentId: "same-agent", description: "读取样本", agentType: "general", status: "completed", toolUseCount: 6,
        resultPreview: "后续分配已完成", narrative: createInitialRuntimeState().narrative },
    ];
    await mount(() => {
      panels = useWorkbenchPanels("thread", { closePanel: vi.fn() }, vi.fn());
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
        runtimeState: { ...createInitialRuntimeState(), activeThreadId: "thread" },
        registry: { workspaceLabelProjectId: "project", backendProjectIdFor: () => "project",
          projectLabelById: new Map([["project", "Fixture"]]), foldersForProjectId: () => [] },
        forceFull: false, hasDraft: true, isBusy: false, requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(), onBeforeFileExpand: vi.fn(),
      });
      const task = tasks.find(task => task.taskId === panels.openSubagentTaskId);
      return <>
        <DesktopWorkbenchChrome fileWorkspace={files} platform="macos" currentConversationLabel="主任务" handleNewTask={vi.fn()}
          subagentTabs={tasks.filter(task => panels.subagentTabs.taskIds.includes(task.taskId))}
          onOpenSubagent={panels.openSubagentTab} />
        <textarea aria-label="主任务输入" defaultValue="保留主草稿" />
        {task ? <SubagentPanel task={task} nowMs={0} cancelling={false} onCancel={cancel} renderActivity={() => null} /> : null}
      </>;
    });
    const draft = host.querySelector("textarea")!;
    await act(async () => {
      files.handleOpenFileWorkspace();
      files.handleOpenProjectFileTab({ id: "file", projectId: "project", folderPath: "/fixture", relativePath: "notes.txt", name: "notes.txt", mode: "files" });
      panels.openSubagentTab("failed");
    });
    expect(host.querySelectorAll('[role="tab"][aria-selected="true"]')).toHaveLength(1);
    expect(host.querySelector('[role="tabpanel"]')?.textContent).toContain("原分配的失败");
    expect(host.querySelector('[role="tabpanel"] textarea, [role="tabpanel"] form')).toBeNull();
    expect(host.querySelectorAll("textarea")).toHaveLength(1);
    await act(async () => panels.openSubagentTab("done"));
    await act(async () => host.querySelector<HTMLButtonElement>('#subagent-tab-failed')!.click());
    expect(host.querySelector('[role="tabpanel"]')?.textContent).toContain("原分配的失败");
    expect(host.querySelectorAll('#subagent-tab-failed')).toHaveLength(1);
    await act(async () => host.querySelector<HTMLButtonElement>('[role="tab"][title="notes.txt"]')!.click());
    expect(panels.openSubagentTaskId).toBeUndefined();
    expect(panels.subagentTabs.taskIds).toEqual(["failed", "done"]);
    expect(files.activeFileWorkspaceTab?.name).toBe("notes.txt");
    await act(async () => panels.openSubagentTab("done"));
    await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="关闭子任务标签 读取样本"]')!.click());
    expect(panels.openSubagentTaskId).toBe("done");
    expect(cancel).not.toHaveBeenCalled();
    expect(host.querySelector("textarea")).toBe(draft);
    expect(draft.value).toBe("保留主草稿");
  });

  it.each(["subagent", "files", "browser", "no-project"])("opens a fresh chooser from %s and preserves existing tabs", async (surface) => {
    let panels!: ReturnType<typeof useWorkbenchPanels>;
    let files!: ReturnType<typeof useFileWorkspaceSession>;
    const task: SubagentTaskView = { taskId: "child", agentId: "agent", description: "样本",
      agentType: "general", status: "completed", toolUseCount: 0, narrative: createInitialRuntimeState().narrative };
    await mount(() => {
      panels = useWorkbenchPanels("thread", { closePanel: vi.fn() }, vi.fn());
      files = useFileWorkspaceSession({
        inspectorTabs: panels.inspectorTabs,
        runtimeState: { ...createInitialRuntimeState(), activeThreadId: "thread" },
        registry: { workspaceLabelProjectId: surface === "no-project" ? undefined : "project",
          backendProjectIdFor: () => "project", projectLabelById: new Map(), foldersForProjectId: () => [] },
        forceFull: false, hasDraft: true, isBusy: false, requestComposerFocus: vi.fn(),
        onBeforeFileOpen: vi.fn(), onBeforeFileExpand: vi.fn(),
      });
      return <><DesktopWorkbenchChrome fileWorkspace={files} platform="macos" currentConversationLabel="主任务" handleNewTask={vi.fn()}
        subagentTabs={panels.subagentTabs.taskIds.length ? [task] : []}
        onOpenSubagent={panels.openSubagentTab} />
        {files.inspectorChooserActive ? <DesktopInspectorRail handleOpenFileWorkspace={files.handleChooseInspectorEntry} /> : null}</>;
    });
    await act(async () => {
      if (surface === "files" || surface === "browser") {
        files.handleOpenFileWorkspace(surface);
        files.handleOpenProjectFileTab({ id: "existing", projectId: "project", folderPath: "/fixture",
          relativePath: "sample.html", name: "sample.html", mode: surface });
      } else panels.openSubagentTab("child");
    });
    const before = files.visibleFileWorkspaceTabSlots;
    await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="新建标签"]')!.click());
    expect(files.inspectorChooserActive).toBe(true);
    expect(files.visibleFileWorkspaceTabSlots).toEqual(before);
    expect(panels.openSubagentTaskId).toBeUndefined();
    expect(host.querySelectorAll('[role="tab"][aria-selected="true"]')).toHaveLength(0);
    const choices = host.querySelectorAll<HTMLButtonElement>('.light-inspector-nav button');
    expect(Array.from(choices, button => button.textContent)).toEqual(["文件", "浏览器"]);
    await act(async () => choices[surface === "browser" ? 1 : 0].click());
    expect(files.inspectorChooserActive).toBe(false);
    expect(files.fileWorkspaceOpen).toBe(true);
    expect(files.fileWorkspaceMode).toBe(surface === "browser" ? "browser" : "files");
    expect(files.activeFileWorkspaceTab).toBeUndefined();
    if (surface !== "no-project") {
      expect(files.visibleFileWorkspaceTabSlots).toHaveLength(before.length + 1);
      expect(files.visibleFileWorkspaceTabSlots.slice(0, before.length)).toEqual(before);
      const count = files.visibleFileWorkspaceTabSlots.length;
      await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="新建标签"]')!.click());
      expect(files.inspectorChooserActive).toBe(true);
      expect(files.visibleFileWorkspaceTabSlots).toHaveLength(count);
      if (before[0]) {
        await act(async () => files.handleActivateProjectFileTab(before[0]));
        expect(files.inspectorChooserActive).toBe(false);
        expect(files.activeFileWorkspaceTab).toEqual(before[0].tab);
      }
    }
    if (surface === "subagent" || surface === "no-project") expect(panels.subagentTabs.taskIds).toEqual(["child"]);
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

  it("keeps the independent child tab when opening media and clears it on thread change", async () => {
    let panels!: ReturnType<typeof useWorkbenchPanels>;
    let threadId = "A";
    const closePlan = vi.fn();
    function Harness() {
      panels = useWorkbenchPanels(
        threadId,
        { closePanel: closePlan },
        vi.fn(),
      );
      return null;
    }
    await act(async () => root.render(<Harness />));
    await act(async () => panels.setContextUsagePanelOpen(true));
    await act(async () => panels.openSubagentTab("child"));
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
    expect(panels.openSubagentTaskId).toBe("child");
    expect(closePlan).toHaveBeenCalledWith(false);
    threadId = "B";
    await act(async () => root.render(<Harness />));
    expect(panels.openSubagentTaskId).toBeUndefined();
    expect(panels.subagentTabs.taskIds).toEqual([]);
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
