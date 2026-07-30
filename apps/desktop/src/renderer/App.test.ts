import React from "react";
import {
  AwakenComposerBar,
  AwakenContextUsagePanel,
  AwakenProjectTree,
  AwakenUserPromptBlock,
  pastedImageFilesFromClipboardData,
  reorderProjectIds,
  shouldSubmitComposerInputKey,
  shouldActivateComposerMetaItemKey,
  slashPaletteStyleForAnchor,
  slashPreviewStyleForAnchor,
} from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  App,
  composerMetaItemsForFileWorkspaceSurface,
  composerStructuredDraftLength,
  contextUsageMeta,
  contextUsagePanelData,
  isUserQuestionSelectionAnswered,
  insertComposerTokenAtOffset,
  modelPreferenceChangeForOption,
  modelPreferenceRecoveryKey,
  mediaRevealLabelForPlatform,
  mediaArtifactRequestForPath,
  nextUserQuestionPromptIndex,
  nextSubagentInspectorTaskId,
  promptNarrativeTokensForComposer,
  promptTextAfterTokenLeadingText,
  promptTokenViews,
  reconcileComposerStructuredInput,
  resolveBackendProjectOrder,
  resolveRunProjectForRun,
  resolveRunRootsForRun,
  partitionPickedProjectFolders,
  resolveWorkspaceRootForRun,
  selectedUserQuestionLabels,
  sharedFolderTrashWarning,
  shouldEnableInspectorResizeHandle,
  shouldUseCenterComposerWithFileWorkspace,
  shouldResetActiveThreadForWorkspaceChange,
  shouldShowSidebarThreadSpinner,
  toolTracePresentation,
  workbenchInspectorPresentation,
  UserQuestionPrompt,
} from "./App";
import {
  actionsFromSlashProviders,
  clearSlashTriggerFromComposerValue,
  computerUseSlashProvider,
  COMPUTER_USE_SLASH_PROMPT,
  composerTokenIdForKeyboardDelete,
  composerTokenPlacementForSlashTrigger,
  composerTokenFromSlashAction,
  filterSlashActions,
  replaceSlashTriggerWithText,
  skillSlashProvider,
  skillSlashActionsFromSkills,
  slashTriggerFromComposerValue,
} from "./composerSlash";
import { composerRuntimePanelsForStatus } from "./useComposerLifecycle";
import { promptAttachmentViews } from "./composerAttachments";
import type { RuntimeSkill } from "../shared/runtime";
import type { RunNarrativeItem } from "./run-narrative";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  canReconcileTrackedTrashOperation,
  normalizeWorkspacePath,
  parseTrackedTrashOperations,
  projectIdForThread,
  projectOperationReconciliationState,
  removeWorkspaceProject,
  upsertTrackedTrashOperation,
  workspaceLabelForPath,
  workspaceProjectRunRoots,
  type WorkspaceProjectFolder,
} from "./projectWorkspace";
import {
  anchoredTurnMinHeight,
  activeTurnIdForAnchor,
  composerBottomSafeArea,
  composerContentBottomInset,
  contextUsagePanelBottomOffset,
  diffLineStats,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  isNearScrollBottom,
  latestTurnMinHeightBottomReserve,
  latestTurnPromptAnchorSlack,
  projectNarrativeTurnBodySegments,
  projectVisibleNarrativeItems,
  scrollTopForLatestTurnScrollTail,
  scrollTopForPromptAnchor,
  shouldCancelPendingPromptAnchorForScrollIntent,
  shouldFollowLatestTurnScrollTail,
  shouldPauseLatestOutputFollowForScroll,
  shouldPauseLatestOutputFollowForTouchMove,
  shouldPauseLatestOutputFollowForWheel,
  shouldRequestLatestOutputFollowResumeForTouchMove,
  shouldRequestLatestOutputFollowResumeForWheel,
  shouldResumeLatestOutputFollow,
  shouldShowAssistantAnswerFooter,
  shouldShowReturnToLatestAffordance,
  shouldShowTurnMarkerRail,
  turnMarkerRailHeight,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
} from "./workbenchPresentation";
import type { AwakenBridge } from "../shared/runtime";

/** 第一个路径是主文件夹，其余按顺序作为次文件夹。 */
function userFolders(...paths: string[]): WorkspaceProjectFolder[] {
  return paths.map((path, index) => ({
    path,
    role: "user" as const,
    isPrimary: index === 0,
  }));
}

describe("media reveal platform copy", () => {
  it("uses the native file manager name on macOS and Windows", () => {
    expect(mediaRevealLabelForPlatform("macos")).toBe("在 Finder 中显示");
    expect(mediaRevealLabelForPlatform("windows")).toBe("在文件资源管理器中显示");
    expect(mediaRevealLabelForPlatform("linux")).toBe("在文件管理器中显示");
  });
});

describe("tool trace presentation", () => {
  it("keeps replay-only tool details in a nested output card", () => {
    expect(toolTracePresentation(undefined, "legacy result\nsecond line")).toEqual({
      detail: "legacy result",
      output: "legacy result\nsecond line",
      outputLabel: "工具输出",
      kind: "tool",
    });
  });

  it("uses typed provider source for MCP rows instead of guessing from names", () => {
    expect(
      toolTracePresentation({
        content: "created issue #42",
        status: "done",
        providerId: "connector:github:create_issue",
        providerSource: "connector",
      }),
    ).toMatchObject({
      kind: "mcp",
      detail: "github:create_issue",
      output: "created issue #42",
      outputLabel: "MCP 输出",
    });
    expect(
      toolTracePresentation({
        content: "legacy output",
        status: "done",
        providerId: "connector:unknown",
      }).kind,
    ).toBe("tool");
  });

  it("projects command streams as bounded terminal output cards", () => {
    expect(
      toolTracePresentation({
        content: "command succeeded",
        commandOutput: "4 tests passed\n",
        commandOutputTruncated: true,
        status: "done",
        providerSource: "builtin",
      }),
    ).toMatchObject({
      kind: "terminal",
      detail: "command succeeded",
      output: "4 tests passed",
      outputLabel: "终端输出",
      outputTruncated: true,
    });
  });
});

describe("media artifact request compatibility", () => {
  it("uses a transcript-verified path locator when a legacy run has no Artifact event", () => {
    expect(
      mediaArtifactRequestForPath(
        "thread-1",
        "run-1",
        "output/generated-media/image.png",
      ),
    ).toEqual({
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "legacy-media:output/generated-media/image.png",
      artifactPath: "output/generated-media/image.png",
    });
  });

  it("prefers the durable Artifact identity when it is available", () => {
    expect(
      mediaArtifactRequestForPath(
        "thread-1",
        "run-1",
        "output/generated-media/image.png",
        { id: "artifact-1", path: "output/generated-media/image.png" },
      ),
    ).toEqual({ threadId: "thread-1", runId: "run-1", artifactId: "artifact-1" });
  });
});

function stubElectronBridge(): void {
  vi.stubGlobal("window", {
    awaken: {
      platform: "macos",
      onSelectAllRequested: vi.fn(() => () => {}),
      relaunchDesktop: vi.fn(async () => undefined),
      getSearchSettings: vi.fn(async () => ({
        providers: {
          exa: { enabled: true },
          tavily: { enabled: false },
        },
      })),
      saveSearchSettings: vi.fn(),
      testSearchProvider: vi.fn(),
      getACPRunners: vi.fn(async () => ({ runners: [] })),
      createACPRunner: vi.fn(),
      updateACPRunner: vi.fn(),
      deleteACPRunner: vi.fn(),
      probeACPRunner: vi.fn(),
      getRun: vi.fn(),
      getThreadTranscript: vi.fn(),
      listThreads: vi.fn(async () => []),
      renameThread: vi.fn(),
      updateThreadModelPreference: vi.fn(),
      deleteThread: vi.fn(async () => ({ ok: true as const })),
      listProjects: vi.fn(async () => []),
      listProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        relativePath: "",
        entries: [],
      })),
      resolveProjectEntryForTask: vi.fn(async () => ({
        ok: false as const,
        code: "ENTRY_NOT_FOUND" as const,
        message: "not found",
      })),
      copyProjectEntryPath: vi.fn(async () => ({ ok: true as const })),
      openProjectEntryWith: vi.fn(async () => ({ ok: true as const })),
      startProjectFilePreview: vi.fn(async () => ({
        ok: false as const,
        code: "UNSUPPORTED_FILE_TYPE" as const,
        message: "unsupported",
      })),
      closeProjectFilePreview: vi.fn(async () => undefined),
      onProjectFilePreviewInteraction: vi.fn(() => () => {}),
      openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
      revealProjectFile: vi.fn(async () => ({ ok: true as const })),
      upsertProject: vi.fn(),
      reorderProjects: vi.fn(),
      detachProject: vi.fn(),
      deleteProjectThreads: vi.fn(),
      createProject: vi.fn(),
      attachProjectFolder: vi.fn(),
      detachProjectFolder: vi.fn(),
      setPrimaryProjectFolder: vi.fn(),
      trashProjectFolder: vi.fn(),
      getProjectOperation: vi.fn(),
    openArtifact: vi.fn(async () => ({ ok: true as const })),
    getArtifactMediaSource: vi.fn(async () => ({
      ok: true as const,
      src: "awaken-artifact://media/test/artifact.png",
      mimeType: "image/png",
      byteLength: 128,
    })),
      revealPath: vi.fn(async () => ({ ok: true as const })),
      revealArtifact: vi.fn(async () => ({ ok: true as const })),
      startRun: vi.fn(),
      cancelRun: vi.fn(),
      answerUserQuestion: vi.fn(),
      subscribeRun: vi.fn(),
      unsubscribeRun: vi.fn(),
      getDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
      saveDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
      getRuntimeInfo: vi.fn(),
      listConnectors: vi.fn(async () => []),
      connectConnector: vi.fn(),
      disconnectConnector: vi.fn(),
      listCliConnectors: vi.fn(async () => []),
      installCliConnector: vi.fn(),
      refreshCliConnector: vi.fn(),
      resolveExternalInteraction: vi.fn(),
      resolveACPPermission: vi.fn(),
      cancelCliInstall: vi.fn(),
      startCliAuth: vi.fn(),
      cancelCliAuth: vi.fn(),
      retryCliAuth: vi.fn(),
      setCliConnectorActivation: vi.fn(),
      logoutCliConnector: vi.fn(),
      uninstallManagedCli: vi.fn(),
      subscribeCliConnectorState: vi.fn(() => () => {}),
      listMcpServers: vi.fn(async () => []),
      configureMcpServers: vi.fn(),
      getMcpServerConfig: vi.fn(),
      updateMcpServerConfig: vi.fn(),
      disableMcpServer: vi.fn(),
      enableMcpServer: vi.fn(),
      deleteMcpServer: vi.fn(),
      getRuntimeLogs: vi.fn(async () => []),
      listMcpTools: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      getSkillPreview: vi.fn(async () => ({
        frontmatterRaw: "",
        bodyMarkdown: "",
      })),
      installSkill: vi.fn(),
      deleteSkill: vi.fn(),
      disableSkill: vi.fn(),
      enableSkill: vi.fn(),
      listModelProviders: vi.fn(async () => []),
      listModelProfiles: vi.fn(async () => []),
      getAgentPlanAccount: vi.fn(async () => ({
        accountId: "volcengine-agent-plan-default" as const,
        providerId: "volcengine-agent-plan" as const,
        hasKey: false,
        credentialRevision: 1,
        modelIds: ["ark-code-latest"],
        catalog: [{ id: "ark-code-latest", kind: "llm" as const }],
      })),
      saveAgentPlanAccount: vi.fn(),
      refreshAgentPlanModels: vi.fn(),
      deleteAgentPlanAccount: vi.fn(async () => undefined),
      getAgentPlanMediaTools: vi.fn(async () => ({ configurations: [] })),
      saveAgentPlanMediaTool: vi.fn(async () => ({
        settings: { configurations: [] },
        restartStatus: "restarted" as const,
      })),
      deleteAgentPlanMediaTool: vi.fn(async () => ({
        settings: { configurations: [] },
        restartStatus: "restarted" as const,
      })),
      saveModelProfile: vi.fn(),
      deleteModelProfile: vi.fn(),
      setDefaultModelProfile: vi.fn(),
      testModelProfile: vi.fn(async () => ({
        ok: true as const,
        value: { ok: true, message: "ok" },
      })),
      testAgentPlanReasoningCapability: vi.fn(async () => ({
        ok: true as const,
        value: { ok: false, message: "" },
      })),
      syncModelCapabilities: vi.fn(async () => []),
      getModelProfileBalances: vi.fn(async () => ({ balances: [] })),
      pickAttachments: vi.fn(async () => ({ canceled: true as const })),
      pickDirectory: vi.fn(async () => ({ canceled: true as const })),
      pickProjectFolders: vi.fn(async () => ({ canceled: true as const })),
      validateWorkspace: vi.fn(async () => ({ valid: true as const })),
      getSupervisorStatus: vi.fn(async () => ({ status: "running" as const })),
      onSupervisorStatusChange: vi.fn(() => () => {}),
      notifyRendererReady: vi.fn(),
      listPets: vi.fn(async () => []),
      getPetSettings: vi.fn(async () => ({
        schemaVersion: 1 as const,
        selectedPetId: null,
        overlayOpen: false,
        mascotWidthPx: 112,
        importCodexPets: true,
        welcomedPetIds: [],
        dismissedNotificationKeys: [],
        placementByDisplay: {},
      })),
      selectPet: vi.fn(async () => ({
        schemaVersion: 1 as const,
        selectedPetId: null,
        overlayOpen: false,
        mascotWidthPx: 112,
        importCodexPets: true,
        welcomedPetIds: [],
        dismissedNotificationKeys: [],
        placementByDisplay: {},
      })),
      refreshCodexPets: vi.fn(async () => ({
        imported: [],
        updated: [],
        sourceMissing: [],
        failed: [],
        conflicts: [],
        selectedPetIdUnchanged: null,
      })),
      createPet: vi.fn(async () => ({ threadId: "task-1" })),
      setPetOverlayOpen: vi.fn(async () => ({
        schemaVersion: 1 as const,
        selectedPetId: null,
        overlayOpen: false,
        mascotWidthPx: 112,
        importCodexPets: true,
        welcomedPetIds: [],
        dismissedNotificationKeys: [],
        placementByDisplay: {},
      })),
      setPetImportPreferences: vi.fn(async () => ({
        schemaVersion: 1 as const,
        selectedPetId: null,
        overlayOpen: false,
        mascotWidthPx: 112,
        importCodexPets: true,
        welcomedPetIds: [],
        dismissedNotificationKeys: [],
        placementByDisplay: {},
      })),
      updatePetActivity: vi.fn(),
      onPetCatalogChanged: vi.fn(() => () => {}),
      onPetThreadActivationRequested: vi.fn(() => () => {}),
      openAwakenPetsDirectory: vi.fn(async () => undefined),
      openCodexPetsDirectory: vi.fn(async () => undefined),
      listMemories: vi.fn(async () => []),
      getMemory: vi.fn(async () => {
        throw new Error("not found");
      }),
      deleteMemory: vi.fn(async () => undefined),
      getMemorySettings: vi.fn(async () => ({
        revision: 0,
        value: {
          enabled: true,
          automaticRecall: true,
          proactiveMemory: true,
          autoDream: false,
        },
      })),
      updateMemorySettings: vi.fn(async () => ({
        revision: 1,
        value: {
          enabled: true,
          automaticRecall: true,
          proactiveMemory: true,
          autoDream: false,
        },
      })),
      startMemoryDream: vi.fn(async () => ({
        taskId: "dream",
        status: "queued" as const,
        scopes: ["user" as const],
        createdAt: new Date(0).toISOString(),
      })),
      getMemoryDream: vi.fn(async () => ({
        taskId: "dream",
        status: "completed" as const,
        scopes: ["user" as const],
        createdAt: new Date(0).toISOString(),
      })),
      listMemoryDreams: vi.fn(async () => []),
      cancelMemoryDream: vi.fn(async () => undefined),
      onMemoryDreamReceipt: vi.fn(() => () => {}),
      listScheduledTasks: vi.fn(async () => []),
      getScheduledTask: vi.fn(async () => {
        throw new Error("not found");
      }),
      createScheduledTask: vi.fn(async () => {
        throw new Error("not implemented");
      }),
      patchScheduledTask: vi.fn(async () => {
        throw new Error("not implemented");
      }),
      deleteScheduledTask: vi.fn(async () => undefined),
      pauseScheduledTask: vi.fn(async () => {
        throw new Error("not implemented");
      }),
      resumeScheduledTask: vi.fn(async () => {
        throw new Error("not implemented");
      }),
      runScheduledTaskNow: vi.fn(async () => {
        throw new Error("not implemented");
      }),
      cancelScheduledTaskExecution: vi.fn(async () => undefined),
      listScheduledTaskExecutions: vi.fn(async () => []),
      getSystemTimezone: vi.fn(async () => "UTC"),
      subscribeScheduledTaskEvents: vi.fn(() => () => {}),
      onScheduledTaskThreadActivation: vi.fn(() => () => {}),
      resolveComputerReview: vi.fn(async () => undefined),
      computerSessionControl: vi.fn(async () => undefined),
      getComputerHostStatus: vi.fn(async () => ({
        available: false,
        healthy: false,
        tcc_screen_recording: false,
        tcc_accessibility: false,
        message: "not configured",
      })),
      getComputerSettings: vi.fn(async () => ({
        revision: 0,
        value: {
          enabled: false,
          allow_background_window_actions: false,
        },
      })),
      saveComputerSettings: vi.fn(async () => ({
        revision: 1,
        value: {
          enabled: false,
          allow_background_window_actions: false,
        },
      })),
      prepareComputerRuntime: vi.fn(async () => ({
        node_ready: false,
        cua_host_ready: false,
        message: "not configured",
      })),
      getComputerPermissionStatus: vi.fn(async () => "unknown" as const),
      requestComputerPermission: vi.fn(async () => "unknown" as const),
    } satisfies AwakenBridge,
  });
}

describe("shouldResetActiveThreadForWorkspaceChange", () => {
  it("returns false when no active thread workspace exists", () => {
    expect(shouldResetActiveThreadForWorkspaceChange(undefined, "/tmp/project")).toBe(false);
  });

  it("returns false when switching within the same workspace", () => {
    expect(shouldResetActiveThreadForWorkspaceChange("/tmp/project", "/tmp/project")).toBe(false);
  });

  it("returns true when switching to a different workspace", () => {
    expect(shouldResetActiveThreadForWorkspaceChange("/tmp/project-a", "/tmp/project-b")).toBe(
      true,
    );
  });
});

describe("project ordering", () => {
  it("moves only the dragged top-level project around the target", () => {
    expect(reorderProjectIds(["a", "b", "c", "d"], "a", "c", "after")).toEqual([
      "b",
      "c",
      "a",
      "d",
    ]);
    expect(reorderProjectIds(["a", "b", "c", "d"], "d", "b", "before")).toEqual([
      "a",
      "d",
      "b",
      "c",
    ]);
  });

  it("keeps the order stable for an invalid or self target", () => {
    const ids = ["a", "b", "c"];
    expect(reorderProjectIds(ids, "b", "b", "after")).toBe(ids);
    expect(reorderProjectIds(ids, "missing", "b", "before")).toBe(ids);
  });

  it("maps the renderer default project id to its durable backend id", () => {
    expect(
      resolveBackendProjectOrder(
        ["project-b", DEFAULT_WORKSPACE_PROJECT_ID, "project-a"],
        "backend-default",
        [
          {
            id: "project-a",
            backendProjectId: "backend-a",
            label: "A",
            path: "/a",
            folders: userFolders("/a"),
          },
          {
            id: "project-b",
            backendProjectId: "backend-b",
            label: "B",
            path: "/b",
            folders: userFolders("/b"),
          },
        ],
      ),
    ).toEqual(["backend-b", "backend-default", "backend-a"]);
    expect(
      resolveBackendProjectOrder([DEFAULT_WORKSPACE_PROJECT_ID], undefined, []),
    ).toBeUndefined();
  });
});

describe("resolveWorkspaceRootForRun", () => {
  it("keeps an independent conversation unbound even when stale project paths exist", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        workspaceTargetPath: "/tmp/target",
        conversationScope: true,
        activeThreadWorkspacePath: "/tmp/managed-thread",
        selectedProjectId: undefined,
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [],
      }),
    ).toBe("");
  });

  it("prefers the immediate workspace target over stale workspace state", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        workspaceTargetPath: "/tmp/new",
        selectedProjectId: "project-new",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "project-new",
            label: "new",
            path: "/tmp/new",
            folders: userFolders("/tmp/new"),
          },
        ],
      }),
    ).toBe("/tmp/new");
  });

  it("uses the selected project path before a stale workspace path", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        selectedProjectId: "project-knowledge",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "project-knowledge",
            label: "knowledge",
            path: "/tmp/knowledge",
            folders: userFolders("/tmp/knowledge"),
          },
        ],
      }),
    ).toBe("/tmp/knowledge");
  });

  it("uses the default workspace when the built-in project is selected", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        selectedProjectId: DEFAULT_WORKSPACE_PROJECT_ID,
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [],
      }),
    ).toBe("/tmp/default");
  });

  it("does not fall back to the hidden default workspace after that project is removed", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "",
        selectedProjectId: undefined,
        defaultWorkspacePath: "/tmp/default",
        defaultProjectRemoved: true,
        projectsFromWorkspace: [],
      }),
    ).toBe("");
  });

  // A project created without a folder anchors nothing on disk. Borrowing the
  // previous thread's directory would silently run the agent somewhere the user
  // never associated with this project, so the root stays empty and the core
  // resolves that project's own session workspace.
  it("returns no root for a project that has no folder bound", () => {
    const folderless = {
      id: "project-folderless",
      label: "folderless",
      path: "",
      folders: [],
    };
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/previous",
        activeThreadWorkspacePath: "/tmp/previous",
        selectedProjectId: "project-folderless",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [folderless],
      }),
    ).toBe("");
  });

  // An explicit target is a direct instruction and still wins, so opening a
  // folder while a folder-less project is selected behaves normally.
  it("still honours an explicit target for a folder-less project", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspaceTargetPath: "/tmp/picked",
        selectedProjectId: "project-folderless",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "project-folderless",
            label: "folderless",
            path: "",
            folders: [],
          },
        ],
      }),
    ).toBe("/tmp/picked");
  });

  it("keeps using the bound folder once one is attached", () => {
    expect(
      resolveWorkspaceRootForRun({
        selectedProjectId: "project-grown",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "project-grown",
            label: "grown",
            path: "/tmp/grown",
            folders: userFolders("/tmp/grown"),
          },
        ],
      }),
    ).toBe("/tmp/grown");
  });
});

describe("picking several project folders at once", () => {
  it("accepts every folder when nothing is taken", () => {
    expect(
      partitionPickedProjectFolders({
        candidates: ["/tmp/a", "/tmp/b", "/tmp/c"],
        attachedPaths: [],
      }),
    ).toEqual({ accepted: ["/tmp/a", "/tmp/b", "/tmp/c"], skipped: [] });
  });

  // One unusable folder must not cost the user the rest of the selection, which
  // is the whole reason the picker reports a partition instead of failing.
  it("keeps the usable folders and explains each one it skips", () => {
    const result = partitionPickedProjectFolders({
      candidates: ["/tmp/fresh", "/tmp/already", "/tmp/second"],
      attachedPaths: ["/tmp/already"],
    });
    expect(result.accepted).toEqual(["/tmp/fresh", "/tmp/second"]);
    expect(result.skipped).toEqual(["“already”已经在这个项目里。"]);
  });

  // A folder belonging to another project is not a reason to skip it: the same
  // directory may be part of several projects.
  it("accepts a folder that another project already holds", () => {
    expect(
      partitionPickedProjectFolders({
        candidates: ["/tmp/shared"],
        attachedPaths: ["/tmp/mine"],
      }),
    ).toEqual({ accepted: ["/tmp/shared"], skipped: [] });
  });

  // Accepted folders join the taken set, so two selections pointing at the same
  // place cannot both be attached.
  it("does not accept the same folder twice within one selection", () => {
    const result = partitionPickedProjectFolders({
      candidates: ["/tmp/docs", "/tmp/docs/", "/tmp/other"],
      attachedPaths: [],
    });
    expect(result.accepted).toEqual(["/tmp/docs", "/tmp/other"]);
    expect(result.skipped).toEqual(["“docs”已经在这个项目里。"]);
  });
});

describe("multi-folder run scope", () => {
  const multiFolderProject = {
    id: "project-multi",
    label: "multi",
    path: "/tmp/primary",
    folders: userFolders("/tmp/primary", "/tmp/secondary", "/tmp/docs"),
  };

  it("keeps the primary folder first and every other folder in project order", () => {
    expect(workspaceProjectRunRoots(multiFolderProject)).toEqual([
      "/tmp/primary",
      "/tmp/secondary",
      "/tmp/docs",
    ]);
    expect(
      workspaceProjectRunRoots({
        path: "/tmp/primary",
        folders: [
          { path: "/tmp/secondary", role: "user", isPrimary: false },
          { path: "/tmp/primary", role: "user", isPrimary: true },
        ],
      }),
    ).toEqual(["/tmp/primary", "/tmp/secondary"]);
  });

  it("expands the run scope for any folder of the project, not just the primary", () => {
    expect(
      resolveRunRootsForRun({
        workspaceRoot: "/tmp/primary",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual(["/tmp/primary", "/tmp/secondary", "/tmp/docs"]);
  });

  it("keeps a secondary-anchored run inside the whole project with its cwd first", () => {
    expect(
      resolveRunRootsForRun({
        workspaceRoot: "/tmp/secondary",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual(["/tmp/secondary", "/tmp/primary", "/tmp/docs"]);
    expect(
      resolveRunRootsForRun({
        workspaceRoot: "/tmp/docs",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual(["/tmp/docs", "/tmp/primary", "/tmp/secondary"]);
  });

  it("stays single-root for a path that belongs to no project", () => {
    expect(
      resolveRunRootsForRun({
        workspaceRoot: "/tmp/unrelated",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual(["/tmp/unrelated"]);
    expect(
      resolveRunRootsForRun({
        workspaceRoot: "",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual([]);
  });
});

describe("resolveRunProjectForRun", () => {
  const multiFolderProject = {
    id: "project-multi",
    label: "研发项目",
    path: "/tmp/primary",
    folders: userFolders("/tmp/primary", "/tmp/secondary"),
  };

  it("names the project a freshly created folder group belongs to", () => {
    expect(
      resolveRunProjectForRun({
        workspaceRoot: "/tmp/primary",
        selectedProjectId: "project-multi",
        defaultWorkspacePath: "/tmp/awaken-default",
        effectiveDefaultWorkspacePath: "/tmp/awaken-default",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual({ projectId: "project-multi", projectName: "研发项目" });
  });

  // Re-primarying or continuing an older thread anchors the run on a secondary
  // folder; the project identity must survive that, or the model loses its name.
  it("names the project when the run is anchored on a secondary folder", () => {
    expect(
      resolveRunProjectForRun({
        workspaceRoot: "/tmp/secondary",
        selectedProjectId: "project-multi",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual({ projectId: "project-multi", projectName: "研发项目" });
  });

  it("carries the identity of a folder-less project that has no root at all", () => {
    expect(
      resolveRunProjectForRun({
        workspaceRoot: "",
        selectedProjectId: "project-empty",
        projectsFromWorkspace: [
          multiFolderProject,
          { id: "project-empty", label: "无文件夹项目", path: "", folders: [] },
        ],
      }),
    ).toEqual({ projectId: "project-empty", projectName: "无文件夹项目" });
  });

  it("names the default workspace project from its path", () => {
    expect(
      resolveRunProjectForRun({
        workspaceRoot: "/tmp/awaken-default",
        selectedProjectId: DEFAULT_WORKSPACE_PROJECT_ID,
        defaultWorkspacePath: "/tmp/awaken-default",
        effectiveDefaultWorkspacePath: "/tmp/awaken-default",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual({
      projectId: DEFAULT_WORKSPACE_PROJECT_ID,
      projectName: "awaken-default",
    });
  });

  it("reports no project for a root outside every project", () => {
    expect(
      resolveRunProjectForRun({
        workspaceRoot: "/tmp/unrelated",
        selectedProjectId: "project-multi",
        projectsFromWorkspace: [multiFolderProject],
      }),
    ).toEqual({});
  });

  describe("when a folder is shared between projects", () => {
    const sharing = {
      id: "project-sharing",
      label: "共享方",
      path: "/tmp/shared",
      folders: userFolders("/tmp/shared", "/tmp/extra"),
    };
    const projects = [multiFolderProject, sharing];

    // The path alone matches both projects, so the project the user picked wins.
    it("follows the selected project rather than the first match", () => {
      expect(
        resolveRunProjectForRun({
          workspaceRoot: "/tmp/shared",
          selectedProjectId: "project-sharing",
          projectsFromWorkspace: [
            { ...multiFolderProject, folders: userFolders("/tmp/shared") },
            sharing,
          ],
        }),
      ).toEqual({ projectId: "project-sharing", projectName: "共享方" });
    });

    // Continuing an old thread anchors the run outside the selected project, so
    // the path has to decide instead.
    it("falls back to the path when the selection does not hold the root", () => {
      expect(
        resolveRunProjectForRun({
          workspaceRoot: "/tmp/secondary",
          selectedProjectId: "project-sharing",
          projectsFromWorkspace: projects,
        }),
      ).toEqual({ projectId: "project-multi", projectName: "研发项目" });
    });

    // The roots must come from the same project the run is attributed to, or the
    // model is shown another project's folder group.
    it("takes the run roots from the attributed project", () => {
      expect(
        resolveRunRootsForRun({
          workspaceRoot: "/tmp/shared",
          projectsFromWorkspace: [
            { ...multiFolderProject, folders: userFolders("/tmp/shared") },
            sharing,
          ],
          projectId: "project-sharing",
        }),
      ).toEqual(["/tmp/shared", "/tmp/extra"]);
    });
  });
});

describe("sharedFolderTrashWarning", () => {
  const projects = [
    {
      id: "project-a",
      label: "甲项目",
      folders: [{ path: "/tmp/shared" }, { path: "/tmp/only-a" }],
    },
    { id: "project-b", label: "乙项目", folders: [{ path: "/tmp/shared" }] },
    { id: "project-c", label: "丙项目", folders: [{ path: "/tmp/shared" }] },
  ];

  it("names every other project that would lose the folder", () => {
    expect(
      sharedFolderTrashWarning({
        folderPath: "/tmp/shared",
        actingProjectId: "project-a",
        projects,
      }),
    ).toBe("该文件夹同时属于“乙项目”、“丙项目”，也会一并从这些项目中移除。");
  });

  it("stays silent for a folder only this project holds", () => {
    expect(
      sharedFolderTrashWarning({
        folderPath: "/tmp/only-a",
        actingProjectId: "project-a",
        projects,
      }),
    ).toBe("");
  });

  it("stays silent when there is no folder to trash", () => {
    expect(
      sharedFolderTrashWarning({
        actingProjectId: "project-a",
        projects,
      }),
    ).toBe("");
  });
});

describe("AwakenComposerInput keyboard behavior", () => {
  it("submits with Enter and keeps Shift+Enter for new lines", () => {
    expect(shouldSubmitComposerInputKey({ key: "Enter" })).toBe(true);
    expect(shouldSubmitComposerInputKey({ key: "Enter", shiftKey: true })).toBe(false);
    expect(shouldSubmitComposerInputKey({ key: "Enter", isComposing: true })).toBe(false);
    expect(shouldSubmitComposerInputKey({ key: "a" })).toBe(false);
  });

  it("collects pasted image files without pulling in non-image clipboard files", () => {
    const imageFile = new File(["image"], "paste.png", { type: "image/png" });
    const textFile = new File(["text"], "notes.txt", { type: "text/plain" });

    const files = pastedImageFilesFromClipboardData({
      items: [] as unknown as DataTransferItemList,
      files: [imageFile, textFile] as unknown as FileList,
    });

    expect(files).toHaveLength(1);
    expect(files[0]?.name).toBe("paste.png");
  });
});

describe("Composer Slash Palette helpers", () => {
  const skills: RuntimeSkill[] = [
    {
      id: "daily-followup",
      name: "daily-followup",
      displayName: "Daily Followup",
      description: "Create a daily customer follow-up plan.",
      whenToUse: "Use when the user asks for customer follow-up.",
      state: "loaded",
      userInvocable: true,
      sourceOrigin: "project",
      rootDir: "/workspace/.awaken/skills/daily-followup",
      skillFilePath: "/workspace/.awaken/skills/daily-followup/SKILL.md",
      context: "inline",
    },
    {
      id: "pptx",
      name: "pptx",
      description: "Work with PowerPoint decks.",
      state: "invalid",
      userInvocable: true,
      sourceOrigin: "user",
      skillFilePath: "/home/.awaken/skills/pptx/SKILL.md",
      context: "inline",
    },
    {
      id: "mcp-skill",
      name: "mcp-skill",
      description: "MCP provided skill.",
      state: "loaded",
      userInvocable: true,
      sourceOrigin: "mcp",
      context: "inline",
    },
    {
      id: "shadowed",
      name: "shadowed",
      description: "Hidden duplicate.",
      state: "shadowed",
      sourceOrigin: "project",
      context: "inline",
    },
    {
      id: "model-only",
      name: "model-only",
      description: "Only the model may invoke this Skill.",
      state: "loaded",
      userInvocable: false,
      modelInvocable: true,
      sourceOrigin: "plugin",
      context: "inline",
    },
  ];

  it("only opens for slash queries and filters skill actions by metadata", () => {
    expect(slashTriggerFromComposerValue("/")).toMatchObject({ query: "" });
    expect(slashTriggerFromComposerValue("/daily")).toMatchObject({
      query: "daily",
    });
    expect(slashTriggerFromComposerValue("hello /daily")).toMatchObject({
      query: "daily",
      start: 6,
      end: 12,
    });
    expect(slashTriggerFromComposerValue("hello /")).toMatchObject({
      query: "",
      start: 6,
      end: 7,
    });
    expect(slashTriggerFromComposerValue("hello/")).toBeNull();
    expect(slashTriggerFromComposerValue("hello /daily later")).toBeNull();
    expect(slashTriggerFromComposerValue("daily")).toBeNull();

    const actions = skillSlashActionsFromSkills(skills);

    expect(actions.map((action) => action.label)).toEqual(["Daily Followup", "mcp-skill"]);
    expect(actions.every((action) => !action.disabled)).toBe(true);
    expect(filterSlashActions(actions, "powerpoint")).toHaveLength(0);
    expect(filterSlashActions(actions, "mcp provided")).toHaveLength(1);
    expect(filterSlashActions(actions, "follow-up")[0]?.id).toContain("daily-followup");
  });

  it("clears only the trailing slash trigger and preserves natural text", () => {
    expect(clearSlashTriggerFromComposerValue("/daily")).toBe("");
    expect(clearSlashTriggerFromComposerValue("hello /daily")).toBe("hello ");
    expect(clearSlashTriggerFromComposerValue("hello /")).toBe("hello ");
    expect(clearSlashTriggerFromComposerValue("hello/daily")).toBe("hello/daily");
  });

  it("places tokens based on where the slash trigger was typed", () => {
    expect(composerTokenPlacementForSlashTrigger(slashTriggerFromComposerValue("/doc"))).toBe(
      "before",
    );
    expect(composerTokenPlacementForSlashTrigger(slashTriggerFromComposerValue("hello /doc"))).toBe(
      "after",
    );
  });

  it("selects adjacent tokens for keyboard deletion", () => {
    const tokens = [{ id: "before" }, { id: "after" }];
    const placements = { before: "before", after: "after" } as const;

    expect(
      composerTokenIdForKeyboardDelete({
        key: "Backspace",
        selectionStart: 0,
        selectionEnd: 0,
        valueLength: 5,
        tokens,
        placements,
      }),
    ).toBe("after");
    expect(
      composerTokenIdForKeyboardDelete({
        key: "Delete",
        selectionStart: 5,
        selectionEnd: 5,
        valueLength: 5,
        tokens,
        placements,
      }),
    ).toBeUndefined();
    expect(
      composerTokenIdForKeyboardDelete({
        key: "Backspace",
        selectionStart: 3,
        selectionEnd: 3,
        valueLength: 5,
        tokens,
        placements,
      }),
    ).toBeUndefined();
  });

  it("turns selected skill actions into structured composer tokens", () => {
    const [action] = skillSlashActionsFromSkills(skills);
    const token = action ? composerTokenFromSlashAction(action) : null;

    expect(token).toMatchObject({
      id: "skill:daily-followup:project",
      type: "skill",
      label: "Daily Followup",
      payload: {
        skillId: "daily-followup",
        sourceOrigin: "project",
        skillFilePath: "/workspace/.awaken/skills/daily-followup/SKILL.md",
        rootDir: "/workspace/.awaken/skills/daily-followup",
      },
    });
  });

  it("aggregates slash actions from providers without binding the palette to skills", async () => {
    const skillProvider = skillSlashProvider(async (workspaceRoot) => {
      expect(workspaceRoot).toBe("/workspace");
      return skills;
    });
    const actions = await actionsFromSlashProviders(
      [
        {
          id: "files",
          label: "Files",
          getActions: () => [
            {
              id: "file:/workspace/brief.md",
              type: "file",
              label: "brief.md",
              payload: { path: "/workspace/brief.md" },
              composerToken: {
                id: "file:/workspace/brief.md",
                type: "file",
                label: "brief.md",
                payload: { path: "/workspace/brief.md" },
              },
            },
          ],
        },
        skillProvider,
      ],
      { workspaceRootForRun: "/workspace" },
    );

    expect(actions.map((action) => action.type)).toEqual(["file", "skill", "skill"]);
    expect(actions[0]?.label).toBe("brief.md");
  });

  it("offers Computer Use as a slash prompt and replaces only the active trigger", async () => {
    const actions = await actionsFromSlashProviders(
      [computerUseSlashProvider()],
      { workspaceRootForRun: "/workspace" },
    );
    expect(actions).toHaveLength(1);
    expect(actions[0]).toMatchObject({
      id: "prompt:computer-use",
      type: "prompt",
      label: "Computer Use",
    });
    expect(filterSlashActions(actions, "电脑")).toHaveLength(1);
    expect(
      replaceSlashTriggerWithText("继续 /computer", actions[0]?.composerText ?? ""),
    ).toBe(`继续 ${COMPUTER_USE_SLASH_PROMPT} `);
  });

  it("lets non-skill providers define their own composer token payload", () => {
    const token = composerTokenFromSlashAction({
      id: "file:/workspace/brief.md",
      type: "file",
      label: "brief.md",
      payload: { path: "/workspace/brief.md" },
      composerToken: {
        id: "file:/workspace/brief.md",
        type: "file",
        label: "brief.md",
        payload: { path: "/workspace/brief.md" },
      },
    });

    expect(token).toMatchObject({
      id: "file:/workspace/brief.md",
      type: "file",
      label: "brief.md",
      payload: { path: "/workspace/brief.md" },
    });
  });
});

describe("composer lifecycle presentation", () => {
  it("builds runtime panels without changing their placement order", () => {
    expect(
      composerRuntimePanelsForStatus({
        desktopNeedsRestart: true,
        desktopStarting: true,
        runtimeErrorMessage: "boom",
      }),
    ).toEqual([
      {
        id: "restart",
        title: "Awaken needs restart",
        message: "Quit Awaken and open it again.",
        tone: "error",
      },
      {
        id: "starting",
        title: "Starting Awaken",
        message: "Preparing the desktop app.",
      },
      {
        id: "run-error",
        title: "Run error",
        message: "boom",
        tone: "error",
      },
    ]);
  });
});

describe("assistant answer footer contract", () => {
  it("shows final answer actions only for completed final answers", () => {
    expect(shouldShowAssistantAnswerFooter({ status: "done", isFinal: true })).toBe(true);
    expect(shouldShowAssistantAnswerFooter({ status: "done" })).toBe(false);
    expect(shouldShowAssistantAnswerFooter({ status: "active", isFinal: true })).toBe(false);
    expect(shouldShowAssistantAnswerFooter({ status: "error", isFinal: true })).toBe(false);
  });
});

describe("UserQuestionPrompt pager contract", () => {
  it("renders one question at a time with a page counter", () => {
    const html = renderToString(
      React.createElement(UserQuestionPrompt, {
        question: {
          id: "question-1",
          questions: [
            {
              question: "研究深度?",
              options: [{ label: "standard" }, { label: "deep" }],
            },
            {
              question: "报告侧重点?",
              options: [{ label: "技术与架构选型" }],
            },
          ],
        },
        onSubmit: vi.fn(),
      }),
    );

    expect(html).toContain("1 of 2");
    expect(html).toContain("研究深度?");
    expect(html).toContain("standard");
    expect(html).not.toContain("报告侧重点?");
  });

  it("keeps other answers incomplete until text is provided", () => {
    expect(
      isUserQuestionSelectionAnswered({
        selected: ["__other__"],
        otherText: "",
      }),
    ).toBe(false);
    expect(
      selectedUserQuestionLabels({
        selected: ["standard", "__other__"],
        otherText: "自定义方向",
      }),
    ).toEqual(["standard", "自定义方向"]);
  });

  it("advances within the existing question range", () => {
    expect(nextUserQuestionPromptIndex(0, 2)).toBe(1);
    expect(nextUserQuestionPromptIndex(1, 2)).toBe(1);
    expect(nextUserQuestionPromptIndex(0, 0)).toBe(0);
  });
});

describe("AwakenComposerBar layout contract", () => {
  it("activates meta controls from browser Space key values", () => {
    expect(shouldActivateComposerMetaItemKey(" ")).toBe(true);
    expect(shouldActivateComposerMetaItemKey("Spacebar")).toBe(true);
    expect(shouldActivateComposerMetaItemKey("Enter")).toBe(false);
  });

  it("keeps dock input at one row while center keeps the homepage row budget", () => {
    const baseProps = {
      value: "",
      onChange: vi.fn(),
      onSubmit: vi.fn(),
    };

    const dockHtml = renderToString(
      React.createElement(AwakenComposerBar, { ...baseProps, variant: "dock" }),
    );
    const centerHtml = renderToString(
      React.createElement(AwakenComposerBar, {
        ...baseProps,
        variant: "center",
      }),
    );

    expect(dockHtml).toContain("composer-bar-dock");
    expect(dockHtml).toContain('rows="1"');
    expect(centerHtml).toContain("composer-bar-center");
    expect(centerHtml).toContain('rows="3"');
  });

  it("exposes the context trigger as an expanded dialog control", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "",
        onChange: vi.fn(),
        variant: "dock",
        metaItems: [
          {
            id: "context",
            label: "上下文窗口",
            value: "12K / 112K",
            progress: 12_000 / 112_000,
            onClick: vi.fn(),
            ariaHaspopup: "dialog",
            ariaExpanded: true,
            ariaControls: "context-panel",
          },
        ],
      }),
    );

    expect(html).toContain('aria-haspopup="dialog"');
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain('aria-controls="context-panel"');
    expect(html).toContain("上下文窗口");
    expect(html).toContain("12K / 112K");
    expect(html).not.toContain("可用输入");
    expect(html).not.toContain("约 12K");
  });

  it("renders attachment previews and warning copy inside the composer surface", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "",
        onChange: vi.fn(),
        onSubmit: vi.fn(),
        variant: "dock",
        attachments: [
          {
            id: "image-1",
            kind: "image",
            src: "data:image/png;base64,aGVsbG8=",
            alt: "clip.png",
            mimeType: "image/png",
          },
          {
            id: "file-1",
            kind: "file",
            alt: "specification.pdf",
            label: "specification.pdf",
            detail: "PDF · 24 KB",
            tooltip: "specification.pdf\n类型: PDF\n大小: 24 KB",
            extension: "PDF",
            mimeType: "application/pdf",
          },
        ],
        attachmentWarning: "部分附件暂时无法读取",
      }),
    );

    expect(html).toContain("composer-bar-attachments");
    expect(html).toContain("clip.png");
    expect(html).toContain("composer-bar-file-card");
    expect(html).toContain("specification.pdf");
    expect(html).toContain("composer-bar-attachment-warning");
    expect(html).toContain("部分附件暂时无法读取");
  });

  it("renders slash palette actions with the structured editor host", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "/ppt",
        onChange: vi.fn(),
        onSubmit: vi.fn(),
        variant: "dock",
        tokens: [
          {
            id: "skill:pptx:user",
            type: "skill",
            label: "pptx",
            title: "/token/path/SKILL.md",
          },
        ],
        slashPalette: {
          open: true,
          activeActionId: "skill:pptx:user",
          actions: [
            {
              id: "skill:pptx:user",
              type: "skill",
              label: "pptx",
              description: "Work with PowerPoint decks.",
              preview: {
                description: "Long PowerPoint skill description.",
                path: "/home/.awaken/skills/pptx/SKILL.md",
              },
            },
          ],
        },
      }),
    );

    expect(html).toContain("composer-bar-inline-editor");
    expect(html).toContain("composer-bar-slash-palette");
    expect(html).toContain('role="listbox"');
    expect(html).toContain("pptx");
    expect(html).not.toContain("composer-bar-token-remove");
    expect(html).not.toContain("Work with PowerPoint decks.");
    expect(html).not.toContain("Long PowerPoint skill description.");
    expect(html).not.toContain("/home/.awaken/skills/pptx/SKILL.md");
  });

  it("renders one editing host when structured tokens are selected", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "这个技能写",
        onChange: vi.fn(),
        onSubmit: vi.fn(),
        variant: "dock",
        tokens: [
          {
            id: "skill:documents:user",
            type: "skill",
            label: "Documents",
            leadingText: "你好 ",
          },
        ],
      }),
    );

    expect(html).toContain("composer-bar-inline-editor");
    expect(html).toContain('contentEditable="plaintext-only"');
    expect(html).not.toContain("<textarea");
    expect(html).not.toContain("composer-bar-token-remove");
  });

  it("keeps the native textarea when no structured token is selected", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "普通输入",
        onChange: vi.fn(),
        onSubmit: vi.fn(),
        variant: "dock",
      }),
    );

    expect(html).toContain("<textarea");
    expect(html).not.toContain("composer-bar-inline-editor");
  });

  it("keeps slash preview positioning inside the viewport", () => {
    const rightEdge = slashPreviewStyleForAnchor(
      { left: 600, right: 700, top: 40 },
      { width: 760, height: 420 },
    );
    const rightEdgeLeft = Number(rightEdge.left);
    const rightEdgeWidth = Number(rightEdge.width);

    expect(rightEdgeWidth).toBeLessThanOrEqual(220);
    expect(rightEdgeLeft).toBeGreaterThanOrEqual(8);
    expect(rightEdgeLeft + rightEdgeWidth).toBeLessThanOrEqual(752);

    const narrowViewport = slashPreviewStyleForAnchor(
      { left: 150, right: 190, top: 170 },
      { width: 210, height: 220 },
    );
    const narrowLeft = Number(narrowViewport.left);
    const narrowTop = Number(narrowViewport.top);
    const narrowWidth = Number(narrowViewport.width);

    expect(narrowLeft).toBeGreaterThanOrEqual(8);
    expect(narrowLeft + narrowWidth).toBeLessThanOrEqual(202);
    expect(narrowTop).toBeGreaterThanOrEqual(8);
  });

  it("anchors slash preview relative to the palette instead of the desktop", () => {
    const style = slashPreviewStyleForAnchor(
      { left: 420, right: 720, top: 180 },
      { width: 1200, height: 700 },
      { left: 420, top: 120 },
    );

    expect(style.left).toBe(306);
    expect(style.top).toBe(58);
    expect(style.width).toBe(220);
  });

  it("limits slash palette height to the space above the composer", () => {
    expect(slashPaletteStyleForAnchor(160, 700)["--composer-bar-slash-max-height"]).toBe("146px");
    expect(slashPaletteStyleForAnchor(520, 700)["--composer-bar-slash-max-height"]).toBe("210px");
  });
});

describe("context usage presentation", () => {
  it("anchors the panel to the measured composer bar without the output safe gap", () => {
    expect(contextUsagePanelBottomOffset({ composerBarInset: 88 })).toBe(100);
    expect(contextUsagePanelBottomOffset({ composerBarInset: -20 })).toBe(28);
  });

  it("uses the latest complete provider usage as current accumulated context", () => {
    expect(
      contextUsageMeta({
        eventType: "AWAKEN_CONTEXT_SNAPSHOT",
        estimatedTokens: 24_000,
        actualTokens: 99_000,
        modelWindow: 128_000,
        outputReserve: 16_000,
        effectiveWindow: 112_000,
        latestModelUsage: {
          schemaVersion: 1,
          requestId: "req-1",
          purpose: "agent_turn",
          inputTokens: 22_000,
          outputTokens: 800,
          totalTokens: 22_800,
          usageComplete: true,
          latencyMs: 20,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
      }),
    ).toEqual({
      label: "上下文窗口",
      value: "23k / 112k",
      progress: 22_800 / 112_000,
    });
  });

  it("reconciles provider actual usage into the existing six fields", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-actual",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "system_prompt", estimatedTokens: 2_000, sourceCount: 1 },
          { kind: "builtin_tools", estimatedTokens: 6_000, sourceCount: 10 },
          { kind: "mcp", estimatedTokens: 8_000, sourceCount: 4 },
          { kind: "skills", estimatedTokens: 1_000, sourceCount: 1 },
          { kind: "memory", estimatedTokens: 500, sourceCount: 1 },
          { kind: "conversation", estimatedTokens: 6_500, sourceCount: 8 },
        ],
        generatedAt: "2026-07-19T00:00:02Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-actual",
        purpose: "agent_turn",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        inputTokens: 27_200,
        outputTokens: 800,
        totalTokens: 28_000,
        usageComplete: true,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
    });

    expect(data?.estimatedInputTokens).toBe(28_000);
    expect(data?.contributions).toEqual([
      { id: "system_prompt", label: "系统提示词", estimatedTokens: 2_000 },
      { id: "builtin_tools", label: "内置工具", estimatedTokens: 6_000 },
      { id: "mcp", label: "MCP", estimatedTokens: 8_000 },
      { id: "skills", label: "技能", estimatedTokens: 1_000 },
      { id: "memory", label: "记忆", estimatedTokens: 500 },
      { id: "conversation", label: "对话", estimatedTokens: 10_500 },
    ]);
    expect(
      data?.contributions?.reduce((sum, contribution) => sum + contribution.estimatedTokens, 0),
    ).toBe(28_000);
  });

  it("does not reuse provider usage after an incompatible context identity change", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-epoch-b",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        contextEpochId: "epoch-b",
        contextEpochBoundaryReason: "compact_succeeded",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [{ kind: "conversation", estimatedTokens: 24_000, sourceCount: 8 }],
        generatedAt: "2026-07-19T00:00:02Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-model-a",
        purpose: "agent_turn",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        contextEpochId: "epoch-a",
        totalTokens: 99_000,
        usageComplete: true,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
    });

    expect(data?.estimatedInputTokens).toBe(24_000);
  });

  it("keeps the latest provider actual across a tool-schema-only epoch rotation", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-tool-epoch-b",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        contextEpochId: "epoch-b",
        contextEpochBoundaryReason: "tool_schema_changed",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [{ kind: "conversation", estimatedTokens: 24_000, sourceCount: 8 }],
        generatedAt: "2026-07-19T00:00:02Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-model-a",
        purpose: "agent_turn",
        modelId: "model-a",
        protocolFamily: "openai_chat",
        contextEpochId: "epoch-a",
        totalTokens: 28_000,
        usageComplete: true,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
    });

    expect(data?.estimatedInputTokens).toBe(28_000);
  });

  it("does not reuse usage from another provider profile with the same model protocol", () => {
    const data = contextUsagePanelData(
      {
        eventType: "AWAKEN_CONTEXT_SNAPSHOT",
        snapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-provider-b",
          estimatorId: "awaken_leaf_v4",
          basis: "current_projection",
          modelId: "shared-model",
          protocolFamily: "openai_chat",
          contextEpochId: "epoch-b",
          modelWindowTokens: 128_000,
          outputReserveTokens: 16_000,
          effectiveInputWindowTokens: 112_000,
          estimatedInputTokens: 24_000,
          warningThresholdTokens: 95_000,
          autoCompactThresholdTokens: 100_800,
          compactTargetTokens: 40_000,
          contributions: [{ kind: "conversation", estimatedTokens: 24_000, sourceCount: 8 }],
          generatedAt: "2026-07-19T00:00:02Z",
        },
        latestModelUsage: {
          schemaVersion: 1,
          requestId: "req-provider-a",
          purpose: "agent_turn",
          providerId: "provider-a",
          profileId: "profile-a",
          credentialRevision: 1,
          modelId: "shared-model",
          protocolFamily: "openai_chat",
          contextEpochId: "epoch-a",
          totalTokens: 99_000,
          usageComplete: true,
          latencyMs: 20,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
      },
      {
        id: "profile-b",
        providerId: "provider-b",
        credentialRevision: 1,
        modelId: "shared-model",
        protocol: "openai_chat",
      },
    );

    expect(data?.estimatedInputTokens).toBe(24_000);
  });

  it("does not reuse provider usage after the reasoning effort changes", () => {
    const data = contextUsagePanelData(
      {
        eventType: "AWAKEN_CONTEXT_SNAPSHOT",
        snapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-effort-max",
          estimatorId: "awaken_leaf_v4",
          basis: "current_projection",
          modelId: "shared-model",
          protocolFamily: "openai_chat",
          modelWindowTokens: 128_000,
          outputReserveTokens: 16_000,
          effectiveInputWindowTokens: 112_000,
          estimatedInputTokens: 24_000,
          warningThresholdTokens: 95_000,
          autoCompactThresholdTokens: 100_800,
          compactTargetTokens: 40_000,
          contributions: [{ kind: "conversation", estimatedTokens: 24_000, sourceCount: 8 }],
          generatedAt: "2026-07-19T00:00:02Z",
        },
        latestModelUsage: {
          schemaVersion: 1,
          requestId: "req-effort-high",
          purpose: "agent_turn",
          modelId: "shared-model",
          protocolFamily: "openai_chat",
          reasoningPolicy: "high",
          totalTokens: 99_000,
          usageComplete: true,
          latencyMs: 20,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
      },
      {
        id: "profile-a",
        providerId: "provider-a",
        credentialRevision: 1,
        modelId: "shared-model",
        protocol: "openai_chat",
        reasoningEffort: "max",
      },
    );

    expect(data?.estimatedInputTokens).toBe(24_000);
  });

  it("matches endpoint transport usage through a separate capability model identity", () => {
    const data = contextUsagePanelData(
      {
        eventType: "AWAKEN_CONTEXT_SNAPSHOT",
        snapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-doubao-endpoint",
          estimatorId: "awaken_leaf_v4",
          basis: "current_projection",
          modelId: "ep-doubao-test",
          capabilityModelId: "doubao-seed-2.1-pro",
          endpointSelector: "ep-doubao-test",
          modelSelectorKind: "endpoint",
          protocolFamily: "openai_chat",
          modelWindowTokens: 128_000,
          outputReserveTokens: 16_000,
          effectiveInputWindowTokens: 112_000,
          estimatedInputTokens: 24_000,
          warningThresholdTokens: 95_000,
          autoCompactThresholdTokens: 100_800,
          compactTargetTokens: 40_000,
          contributions: [{ kind: "conversation", estimatedTokens: 24_000, sourceCount: 8 }],
          generatedAt: "2026-07-19T00:00:02Z",
        },
        latestModelUsage: {
          schemaVersion: 1,
          requestId: "req-doubao-endpoint",
          purpose: "agent_turn",
          modelId: "ep-doubao-test",
          capabilityModelId: "doubao-seed-2.1-pro",
          endpointSelector: "ep-doubao-test",
          modelSelectorKind: "endpoint",
          protocolFamily: "openai_chat",
          totalTokens: 28_000,
          usageComplete: true,
          latencyMs: 20,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
      },
      {
        id: "profile-doubao",
        providerId: "doubao",
        credentialRevision: 1,
        modelId: "ep-doubao-test",
        capabilityModelId: "doubao-seed-2.1-pro",
        endpointSelector: "ep-doubao-test",
        modelSelectorKind: "endpoint",
        protocol: "openai_chat",
      },
    );

    expect(data?.estimatedInputTokens).toBe(28_000);
  });

  it("projects typed breakdowns without exposing model-call accounting in the panel", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-1",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "system_prompt", estimatedTokens: 2_000, sourceCount: 1 },
          { kind: "builtin_tools", estimatedTokens: 6_000, sourceCount: 10 },
          { kind: "mcp", estimatedTokens: 8_000, sourceCount: 4 },
          { kind: "skills", estimatedTokens: 1_000, sourceCount: 1 },
          { kind: "memory", estimatedTokens: 500, sourceCount: 1 },
          { kind: "conversation", estimatedTokens: 6_500, sourceCount: 8 },
        ],
        generatedAt: "2026-07-19T00:00:00Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-1",
        purpose: "agent_turn",
        inputTokens: 22_000,
        usageComplete: false,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
      modelUsages: [
        {
          schemaVersion: 1,
          requestId: "req-1",
          purpose: "agent_turn",
          inputTokens: 22_000,
          cachedInputTokens: 2_000,
          uncachedInputTokens: 20_000,
          outputTokens: 800,
          calculatedCostUsd: 0.01,
          usageComplete: true,
          latencyMs: 20,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
        {
          schemaVersion: 1,
          requestId: "req-title",
          purpose: "thread_title",
          inputTokens: 100,
          cachedInputTokens: 0,
          uncachedInputTokens: 100,
          outputTokens: 5,
          calculatedCostUsd: 0.001,
          usageComplete: true,
          latencyMs: 10,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:02Z",
        },
      ],
    });

    expect(data?.contributions).toEqual([
      { id: "system_prompt", label: "系统提示词", estimatedTokens: 2_000 },
      { id: "builtin_tools", label: "内置工具", estimatedTokens: 6_000 },
      { id: "mcp", label: "MCP", estimatedTokens: 8_000 },
      { id: "skills", label: "技能", estimatedTokens: 1_000 },
      { id: "memory", label: "记忆", estimatedTokens: 500 },
      { id: "conversation", label: "对话", estimatedTokens: 6_500 },
    ]);
    const html = renderToString(
      React.createElement(AwakenContextUsagePanel, {
        ...data!,
        onClose: vi.fn(),
      }),
    );
    expect(html).toContain("系统提示词");
    expect(html).toContain("上下文用量");
    expect(html).toContain("24K");
    expect(html).toContain("112K");
    expect(html).toContain("Token");
    expect(html).not.toContain("上下文用了");
    expect(html).not.toContain("约 24K");
    expect(html).not.toContain("模型总窗口");
    expect(html).not.toContain("已预留输出");
    expect(html).not.toContain("自动压缩阈值");
    expect(html).not.toContain("本地估算");
    expect(html).not.toContain("可用输入已使用");
    expect(html).not.toContain("运行环境");
    expect(html).not.toContain("本轮输入");
    expect(html).not.toContain("对话历史");
    expect(html).not.toContain("MCP 与动态工具");
    expect(html).not.toContain("技能与记忆");
    expect(html).not.toContain("最近一次模型调用");
    expect(html).not.toContain("本轮模型调用");
    expect(html).not.toContain("供应商返回");
    expect(html).not.toContain("主调用");
    expect(html).not.toContain("辅助调用");
    expect(html).not.toContain("US$0.011000");
  });

  it("projects last-request cache hit and cost when latestModelUsage carries signal", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-1",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "system_prompt", estimatedTokens: 2_000, sourceCount: 1 },
          { kind: "builtin_tools", estimatedTokens: 6_000, sourceCount: 10 },
          { kind: "mcp", estimatedTokens: 8_000, sourceCount: 4 },
          { kind: "skills", estimatedTokens: 1_000, sourceCount: 1 },
          { kind: "memory", estimatedTokens: 500, sourceCount: 1 },
          { kind: "conversation", estimatedTokens: 6_500, sourceCount: 8 },
        ],
        generatedAt: "2026-07-19T00:00:00Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-1",
        purpose: "agent_turn",
        cacheMode: "automatic_prefix",
        cacheKeyApplied: false,
        cachedInputTokens: 18_000,
        uncachedInputTokens: 6_000,
        outputTokens: 800,
        calculatedCostUsd: 0.0123,
        currency: "USD",
        usageComplete: true,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
    });
    expect(data?.latestModelUsage).toEqual({
      status: "succeeded",
      cacheMode: "automatic_prefix",
      cacheKeyApplied: false,
      cachedInputTokens: 18_000,
      uncachedInputTokens: 6_000,
      cacheWriteTokens: undefined,
      inputTokens: undefined,
      outputTokens: 800,
      reasoningTokens: undefined,
      totalTokens: undefined,
      calculatedCostUsd: 0.0123,
      currency: "USD",
      invalidationReason: undefined,
      purpose: "agent_turn",
      errorCode: undefined,
    });
    const html = renderToString(
      React.createElement(AwakenContextUsagePanel, {
        ...data!,
        onClose: vi.fn(),
      }),
    );
    expect(html).toContain("上次请求");
    expect(html).toContain("已命中");
    expect(html).toContain("命中 Token");
    expect(html).toContain("18K");
    expect(html).toContain("未命中 Token");
    expect(html).toContain("6.0K");
    expect(html).toContain("输出 Token");
    expect(html).toContain("成本");
    expect(html).toContain("$0.0123");
  });

  it("shows cache as 未确认 and cost as 未知 for undocumented providers", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 4,
        snapshotId: "ctx-1",
        estimatorId: "awaken_leaf_v4",
        basis: "current_projection",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 24_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "system_prompt", estimatedTokens: 2_000, sourceCount: 1 },
          { kind: "builtin_tools", estimatedTokens: 6_000, sourceCount: 10 },
          { kind: "mcp", estimatedTokens: 8_000, sourceCount: 4 },
          { kind: "skills", estimatedTokens: 1_000, sourceCount: 1 },
          { kind: "memory", estimatedTokens: 500, sourceCount: 1 },
          { kind: "conversation", estimatedTokens: 6_500, sourceCount: 8 },
        ],
        generatedAt: "2026-07-19T00:00:00Z",
      },
      latestModelUsage: {
        schemaVersion: 1,
        requestId: "req-1",
        purpose: "agent_turn",
        cacheMode: "undocumented",
        cacheKeyApplied: false,
        inputTokens: 24_000,
        outputTokens: 800,
        calculatedCostUsd: undefined,
        currency: "USD",
        usageComplete: true,
        latencyMs: 20,
        status: "succeeded",
        completedAt: "2026-07-19T00:00:01Z",
      },
    });
    const html = renderToString(
      React.createElement(AwakenContextUsagePanel, {
        ...data!,
        onClose: vi.fn(),
      }),
    );
    expect(html).toContain("上次请求");
    expect(html).toContain("未确认");
    expect(html).toContain("成本");
    expect(html).toContain("未知");
    expect(html).not.toContain("已命中");
    expect(html).not.toContain("未命中");
  });

  it("keeps legacy aggregates honest when no typed breakdown exists", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_ESTIMATED",
      modelWindow: 128_000,
      outputReserve: 16_000,
      effectiveWindow: 112_000,
      estimatedTokens: 12_000,
      autoCompactThreshold: 100_800,
    });
    const html = renderToString(
      React.createElement(AwakenContextUsagePanel, {
        ...data!,
        onClose: vi.fn(),
      }),
    );
    expect(html).toContain("暂无分类明细");
  });

  it("does not fabricate a Skill/Memory split for v1 combined records", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 1,
        snapshotId: "ctx-v1",
        estimatorId: "awaken_leaf_v1",
        basis: "current_projection",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 12_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "system_prompt", estimatedTokens: 2_000, sourceCount: 1 },
          { kind: "skills_memory", estimatedTokens: 10_000, sourceCount: 2 },
        ],
        generatedAt: "2026-07-19T00:00:00Z",
      },
    });

    expect(data?.contributions).toEqual([]);
  });

  it("retains the original v3 breakdown until a continued thread produces v4", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      snapshot: {
        schemaVersion: 3,
        snapshotId: "ctx-v3",
        estimatorId: "awaken_leaf_v3",
        basis: "sent_request",
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        effectiveInputWindowTokens: 112_000,
        estimatedInputTokens: 12_000,
        warningThresholdTokens: 95_000,
        autoCompactThresholdTokens: 100_800,
        compactTargetTokens: 40_000,
        contributions: [
          { kind: "builtin_tools", estimatedTokens: 10_000, sourceCount: 10 },
          { kind: "skills", estimatedTokens: 2_000, sourceCount: 1 },
        ],
        generatedAt: "2026-07-19T00:00:00Z",
      },
    });

    expect(data?.contributions).toEqual([
      { id: "system_prompt", label: "系统提示词", estimatedTokens: 0 },
      { id: "builtin_tools", label: "内置工具", estimatedTokens: 10_000 },
      { id: "mcp", label: "MCP", estimatedTokens: 0 },
      { id: "skills", label: "技能", estimatedTokens: 2_000 },
      { id: "memory", label: "记忆", estimatedTokens: 0 },
      { id: "conversation", label: "对话", estimatedTokens: 0 },
    ]);
  });
});

describe("AwakenUserPromptBlock layout contract", () => {
  it("renders the prompt bubble as a collapsed expandable surface", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "1. First line\n2. Second line\n3. Third line\n4. Fourth line",
      }),
    );

    expect(html).toContain("execution-prompt-block-limited");
    expect(html).toContain("execution-prompt-bubble");
    expect(html).toContain("execution-prompt-bubble-text");
    expect(html).toContain("<p");
    expect(html).toContain('role="button"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain("4. Fourth line");
  });

  it("keeps short prompt bubbles non-expandable", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "One short prompt",
      }),
    );

    expect(html).toContain("execution-prompt-bubble");
    expect(html).toContain("execution-prompt-bubble-text");
    expect(html).toContain("<p");
    expect(html).not.toContain("execution-prompt-block-limited");
    expect(html).toContain('role="button"');
    expect(html).toContain('aria-expanded="false"');
  });

  it("renders selected skill tokens in their inline prompt position", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "请继续",
        tokens: [
          {
            id: "skill:deep-research:user",
            type: "skill",
            label: "deep-research",
            leadingText: "你好，你看到这个 skill 不?",
            title: "/home/.awaken/skills/deep-research/SKILL.md",
          },
        ],
      }),
    );

    expect(html).toContain("execution-prompt-bubble-token");
    expect(html).toContain("deep-research");
    expect(html.indexOf("你好，你看到这个 skill 不?")).toBeLessThan(html.indexOf("deep-research"));
    expect(html.indexOf("deep-research")).toBeLessThan(html.indexOf("请继续"));
    const promptStart = html.indexOf('<p class="execution-prompt-bubble-text">');
    const promptEnd = html.indexOf("</p>", promptStart);
    const inlinePrompt = html.slice(promptStart, promptEnd);
    expect(inlinePrompt).toContain("你好，你看到这个 skill 不?");
    expect(inlinePrompt).toContain("deep-research");
    expect(inlinePrompt).toContain("请继续");
    expect(html).not.toContain("execution-prompt-bubble-token-group");
  });

  it("renders selected skill tokens before text when no inline position is present", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "你好，你看到这个 skill 不?",
        tokens: [
          {
            id: "skill:deep-research:user",
            type: "skill",
            label: "deep-research",
            title: "/home/.awaken/skills/deep-research/SKILL.md",
          },
        ],
      }),
    );

    expect(html.indexOf("deep-research")).toBeLessThan(html.indexOf("你好，你看到这个 skill 不?"));
  });

  it("allows the expanded state to be controlled by the parent renderer", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "1. First line\n2. Second line\n3. Third line\n4. Fourth line",
        expanded: true,
      }),
    );

    expect(html).toContain("execution-prompt-block-expanded");
    expect(html).not.toContain('aria-expanded="false"');
  });

  it("renders prompt attachments above the prompt text", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "Look at this image",
        attachments: [
          {
            id: "attachment-1",
            src: "data:image/png;base64,aGVsbG8=",
            alt: "Prompt image",
          },
        ],
      }),
    );

    expect(html).toContain("execution-prompt-bubble-attachments");
    expect(html).toContain("execution-prompt-bubble-attachment");
    expect(html).toContain("Prompt image");
    expect(html).toContain('role="button"');
  });

  it("renders prompt file attachments as compact file cards", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "Read this document",
        attachments: [
          {
            id: "attachment-1",
            kind: "file",
            alt: "analysis.xlsx",
            label: "analysis.xlsx",
            detail: "Excel · 16 KB",
            tooltip: "analysis.xlsx\n类型: Excel\n大小: 16 KB",
            extension: "XLSX",
          },
        ],
      }),
    );

    expect(html).toContain("execution-prompt-bubble-attachments");
    expect(html).toContain("execution-prompt-bubble-file-card");
    expect(html).toContain("analysis.xlsx");
    expect(html).toContain("XLSX");
  });

  it("collapses extra prompt attachments behind an expand affordance", () => {
    const attachments = Array.from({ length: 5 }, (_, index) => ({
      id: `attachment-${index + 1}`,
      kind: "file" as const,
      alt: `file-${index + 1}.xlsx`,
      label: `file-${index + 1}.xlsx`,
      extension: "XLSX",
    }));

    const collapsedHtml = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "这些附件",
        attachments,
      }),
    );
    expect(collapsedHtml).toContain("execution-prompt-bubble-attachment-more");
    expect(collapsedHtml).toContain("还有 2 个附件");
    expect(collapsedHtml).toContain("file-3.xlsx");
    expect(collapsedHtml).not.toContain("file-4.xlsx");

    const expandedHtml = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "这些附件",
        attachments,
        expanded: true,
      }),
    );
    expect(expandedHtml).not.toContain("execution-prompt-bubble-attachment-more");
    expect(expandedHtml).toContain("file-5.xlsx");
  });

  it("shows the prompt toolbar for short prompts once expanded", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "One short prompt",
        expanded: true,
      }),
    );

    expect(html).toContain("execution-prompt-bubble-toolbar");
    expect(html).toContain("Copy prompt");
  });
});

describe("prompt token text projection", () => {
  it("preserves durable token boundary whitespace in view models", () => {
    const tokens = promptNarrativeTokensForComposer(
      [
        {
          id: "skill:docx:user",
          type: "skill",
          label: "docx",
          leadingText: "请使用 \n",
        },
      ],
      {},
    );

    expect(tokens[0]?.leadingText).toBe("请使用 \n");
    expect(promptTokenViews(tokens)?.[0]?.leadingText).toBe("请使用 \n");
  });

  it("removes only durable leading segments and keeps suffix whitespace", () => {
    const tokens = [
      {
        id: "skill:docx:user",
        type: "skill",
        label: "docx",
        leadingText: "请使用 ",
      },
    ];

    expect(promptTextAfterTokenLeadingText("请使用  这个技能写", tokens)).toBe(" 这个技能写");
    expect(promptTextAfterTokenLeadingText("请使用", tokens)).toBe("");
  });
});

describe("structured Composer draft reconciliation", () => {
  it("inserts a hidden-path token at the current inline text position", () => {
    const token = {
      id: "file:brief",
      type: "file" as const,
      label: "brief.md",
      payload: { path: "/workspace/docs/brief.md" },
    };
    const result = insertComposerTokenAtOffset({
      tokens: [],
      placements: {},
      leadingTexts: {},
      value: "请分析然后总结",
      offset: 3,
      token,
    });

    expect(result).toEqual({
      tokens: [{ ...token, leadingText: "请分析" }],
      placements: { "file:brief": "after" },
      leadingTexts: { "file:brief": "请分析" },
      value: "然后总结",
      focusOffset: 4,
    });
    expect(composerStructuredDraftLength(result.tokens, result.leadingTexts, result.value)).toBe(8);
  });

  it("preserves existing Skill token order when inserting a file token between text segments", () => {
    const skillToken = {
      id: "skill:docx:user",
      type: "skill" as const,
      label: "docx",
      leadingText: "用 ",
    };
    const fileToken = {
      id: "file:brief",
      type: "file" as const,
      label: "brief.md",
      payload: { path: "/workspace/brief.md" },
    };
    const result = insertComposerTokenAtOffset({
      tokens: [skillToken],
      placements: { [skillToken.id]: "after" },
      leadingTexts: { [skillToken.id]: "用 " },
      value: "处理材料",
      offset: 5,
      token: fileToken,
    });

    expect(result.tokens).toEqual([skillToken, { ...fileToken, leadingText: "处理" }]);
    expect(result.value).toBe("材料");
    expect(result.focusOffset).toBe(6);
  });

  it("maps editable prefix segments and tail text back to durable Desktop state", () => {
    const tokens = [
      {
        id: "skill:docx:user",
        type: "skill",
        label: "docx",
        leadingText: "旧前文",
      },
      {
        id: "skill:deep-research:user",
        type: "skill",
        label: "deep-research",
        leadingText: "旧中间文",
      },
    ];

    expect(
      reconcileComposerStructuredInput(
        tokens,
        {
          "skill:docx:user": "after",
          "skill:deep-research:user": "after",
        },
        {
          tokens: [
            { id: "skill:docx:user", leadingText: "新前文 " },
            {
              id: "skill:deep-research:user",
              leadingText: "\n新中间文 ",
            },
          ],
          value: "新尾文",
        },
      ),
    ).toEqual({
      tokens: [
        {
          id: "skill:docx:user",
          type: "skill",
          label: "docx",
          leadingText: "新前文 ",
        },
        {
          id: "skill:deep-research:user",
          type: "skill",
          label: "deep-research",
          leadingText: "\n新中间文 ",
        },
      ],
      placements: {
        "skill:docx:user": "after",
        "skill:deep-research:user": "after",
      },
      leadingTexts: {
        "skill:docx:user": "新前文 ",
        "skill:deep-research:user": "\n新中间文 ",
      },
      value: "新尾文",
    });
  });

  it("drops deleted atomic tokens without retaining their placement or label", () => {
    expect(
      reconcileComposerStructuredInput(
        [
          { id: "skill:docx:user", type: "skill", label: "docx" },
          {
            id: "skill:deep-research:user",
            type: "skill",
            label: "deep-research",
          },
        ],
        {
          "skill:docx:user": "after",
          "skill:deep-research:user": "after",
        },
        {
          tokens: [
            {
              id: "skill:deep-research:user",
              leadingText: "已恢复的前文 ",
            },
          ],
          value: "尾文",
        },
      ),
    ).toEqual({
      tokens: [
        {
          id: "skill:deep-research:user",
          type: "skill",
          label: "deep-research",
          leadingText: "已恢复的前文 ",
        },
      ],
      placements: { "skill:deep-research:user": "after" },
      leadingTexts: {
        "skill:deep-research:user": "已恢复的前文 ",
      },
      value: "尾文",
    });
  });
});

describe("prompt attachment view models", () => {
  it("preserves file attachment metadata for compact cards", () => {
    const views = promptAttachmentViews([
      {
        id: "file-1",
        kind: "file",
        mimeType: "application/pdf",
        path: "/Users/nolangrant/Desktop/specification.pdf",
        displayName: "specification.pdf",
        byteSize: 24 * 1024,
      },
    ]);

    expect(views).toHaveLength(1);
    expect(views[0]).toMatchObject({
      kind: "file",
      label: "specification.pdf",
      detail: "PDF · 24 KB",
      extension: "PDF",
    });
    expect(views[0]?.tooltip).toContain("路径: .../nolangrant/Desktop/specification.pdf");
  });
});

describe("project workspace helpers", () => {
  it.each([
    [{ status: "accepted", phase: "waiting_for_runs" }, "pending"],
    [{ status: "running", phase: "trashing" }, "pending"],
    [{ status: "running", phase: "cleaning" }, "succeeded"],
    [{ status: "succeeded", phase: "completed" }, "succeeded"],
    [{ status: "failed", phase: "failed" }, "failed"],
    [{ status: "blocked", phase: "cancelled" }, "failed"],
  ] as const)("classifies durable project operation %o as %s", (operation, expected) => {
    expect(projectOperationReconciliationState(operation)).toBe(expected);
  });

  it("reconciles a folder trash operation by whether the folder is still attached", () => {
    const stillAttached = {
      state: "active" as const,
      folders: [{ path: "/tmp/primary" }, { path: "/tmp/secondary" }],
    };
    const detached = {
      state: "active" as const,
      folders: [{ path: "/tmp/primary" }],
    };

    expect(
      canReconcileTrackedTrashOperation(
        "failed",
        { ...stillAttached, state: "trash_pending" },
        "/tmp/secondary",
      ),
    ).toBe(false);
    expect(canReconcileTrackedTrashOperation("failed", stillAttached, "/tmp/secondary")).toBe(true);
    expect(canReconcileTrackedTrashOperation("failed", detached, "/tmp/secondary")).toBe(false);
    expect(canReconcileTrackedTrashOperation("succeeded", stillAttached, "/tmp/secondary")).toBe(
      false,
    );
    expect(canReconcileTrackedTrashOperation("succeeded", detached, "/tmp/secondary")).toBe(true);
  });

  it("persists valid trash trackers, removes malformed entries, and replaces duplicates", () => {
    const tracked = {
      operationId: "operation-1",
      projectId: "project-ui-1",
      backendProjectId: "project-backend-1",
      folderPath: "/tmp/secondary",
      label: "待删除文件夹",
      wasExpanded: true,
    };
    expect(
      parseTrackedTrashOperations(JSON.stringify([tracked, { operationId: "broken" }, tracked])),
    ).toEqual([tracked]);
    expect(parseTrackedTrashOperations("not-json")).toEqual([]);
    expect(
      parseTrackedTrashOperations(JSON.stringify([{ ...tracked, folderPath: undefined }])),
    ).toEqual([]);
    expect(
      upsertTrackedTrashOperation([tracked], {
        ...tracked,
        label: "更新后的项目名",
        wasExpanded: false,
      }),
    ).toEqual([
      {
        ...tracked,
        label: "更新后的项目名",
        wasExpanded: false,
      },
    ]);
  });

  it("normalizes workspace paths and labels them by folder name", () => {
    expect(normalizeWorkspacePath("/Users/nolangrant/Awaken/")).toBe("/Users/nolangrant/Awaken");
    expect(workspaceLabelForPath("/Users/nolangrant/Awaken/")).toBe("Awaken");
  });

  it("uses only stable runtime project IDs", () => {
    expect(
      projectIdForThread(
        {
          threadId: "thread-stable",
          projectId: "project_123",
          workspaceRoot: "/tmp/demo",
        },
        "/tmp/default",
      ),
    ).toBe("project_123");
    expect(
      projectIdForThread({ threadId: "thread-1", workspaceRoot: "/tmp/demo" }, "/tmp/default"),
    ).toBeUndefined();
    expect(
      projectIdForThread({ threadId: "thread-1", workspaceRoot: "/tmp/default" }, "/tmp/default"),
    ).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
  });

  it("removes folder projects", () => {
    const projects = [
      {
        id: "project-demo",
        label: "demo",
        path: "/tmp/demo",
        folders: userFolders("/tmp/demo"),
      },
      {
        id: "project-keep",
        label: "keep",
        path: "/tmp/keep",
        folders: userFolders("/tmp/keep"),
      },
    ];

    expect(removeWorkspaceProject(projects, "project-demo")).toEqual([
      {
        id: "project-keep",
        label: "keep",
        path: "/tmp/keep",
        folders: userFolders("/tmp/keep"),
      },
    ]);
  });
});

describe("sidebar thread status presentation", () => {
  it("shows a spinner only while the thread still has an active run", () => {
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-running",
        label: "Running",
        runStatus: "running",
        activeRunId: "run-1",
      }),
    ).toBe(true);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-finished",
        label: "Finished",
        runStatus: "finished",
      }),
    ).toBe(false);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-recovering",
        label: "Recovering",
        activeRunId: "run-3",
      }),
    ).toBe(true);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-done",
        label: "Done",
        runStatus: "finished",
        activeRunId: undefined,
      }),
    ).toBe(false);
  });

  it("keeps the loading spinner visible even for the selected thread row", () => {
    const html = renderToString(
      React.createElement(AwakenProjectTree, {
        items: [
          {
            id: "project:awaken",
            label: "Awaken",
            kind: "project",
            children: [
              {
                id: "thread:active",
                kind: "thread",
                label: "正在执行的会话",
                isLoading: true,
              },
            ],
          },
        ],
        activeId: "thread:active",
        defaultExpandedIds: ["project:awaken"],
        onSelect: vi.fn(),
      }),
    );

    expect(html).toContain("正在执行的会话");
    expect(html).toContain('aria-current="true"');
    expect(html).toContain('aria-label="加载中"');
    expect(html).toContain("aw-sidebar-thread-spinner");
    expect(html).toContain("aw-dotm-square3");
  });
});

describe("App workbench presentation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps the existing dock composer visible over the expanded file surface", () => {
    expect(
      shouldUseCenterComposerWithFileWorkspace({
        standardCenterComposer: true,
        fileWorkspaceOpen: true,
        fileWorkspaceExpanded: true,
        expandedWorkspaceSurface: "file",
      }),
    ).toBe(false);
    expect(
      shouldUseCenterComposerWithFileWorkspace({
        standardCenterComposer: true,
        fileWorkspaceOpen: true,
        fileWorkspaceExpanded: true,
        expandedWorkspaceSurface: "conversation",
      }),
    ).toBe(true);
    expect(
      shouldUseCenterComposerWithFileWorkspace({
        standardCenterComposer: true,
        fileWorkspaceOpen: true,
        fileWorkspaceExpanded: false,
        expandedWorkspaceSurface: "file",
      }),
    ).toBe(true);
  });

  it("pauses the outer inspector resize handle only while the file workspace is expanded", () => {
    expect(
      shouldEnableInspectorResizeHandle({
        inspectorAvailable: true,
        fileWorkspaceExpanded: false,
      }),
    ).toBe(true);
    expect(
      shouldEnableInspectorResizeHandle({
        inspectorAvailable: true,
        fileWorkspaceExpanded: true,
      }),
    ).toBe(false);
    expect(
      shouldEnableInspectorResizeHandle({
        inspectorAvailable: false,
        fileWorkspaceExpanded: false,
      }),
    ).toBe(false);
  });

  it("hides project and context metadata only on the expanded file surface", () => {
    const metaItems = [
      { id: "model", label: "kimi-for-coding" },
      { id: "workspace", label: "测试项目" },
      { id: "context", label: "上下文窗口", value: "61k / 246k" },
    ];

    expect(composerMetaItemsForFileWorkspaceSurface(metaItems, true)).toEqual([
      { id: "model", label: "kimi-for-coding" },
    ]);
    expect(composerMetaItemsForFileWorkspaceSurface(metaItems, false)).toBe(metaItems);
  });

  it("selects the target model even when its effort equals the current model effort", () => {
    expect(
      modelPreferenceChangeForOption("profile-target", "profile-current", "high"),
    ).toEqual({
      modelProfileId: "profile-target",
      reasoningEffort: "high",
    });
    expect(
      modelPreferenceChangeForOption("profile-current", "profile-current", "high"),
    ).toBeUndefined();
  });

  it("does not recover a model preference while a new thread is awaiting metadata", () => {
    expect(
      modelPreferenceRecoveryKey(
        "thread-new",
        undefined,
        "profile-default",
        undefined,
        "high",
      ),
    ).toBe("");
  });

  it("recovers only explicit unavailable model or effort preferences", () => {
    expect(
      modelPreferenceRecoveryKey(
        "thread-existing",
        "profile-deleted",
        "profile-default",
        "high",
        "high",
      ),
    ).toBe("thread-existing:profile-deleted:high");
    expect(
      modelPreferenceRecoveryKey(
        "thread-existing",
        "profile-default",
        "profile-default",
        "max",
        "high",
      ),
    ).toBe("thread-existing:profile-default:max");
  });

  it("renders idle as a blank execution stage with the light shell", () => {
    stubElectronBridge();

    const html = renderToString(React.createElement(App));

    expect(html).toContain('aria-label="文件"');
    const fileEntry = html.match(
      /<button[^>]*class="light-inspector-tab[^>]*>[\s\S]*?<\/button>/,
    )?.[0];
    expect(fileEntry).toBeDefined();
    expect(fileEntry).not.toContain("disabled");
    expect(fileEntry).not.toContain("aria-current");
    expect(html).toContain(">文件<");
    expect(html).not.toContain("On Awaken");
    expect(html).not.toContain("右侧栏待开发");
    expect(html).not.toContain(">审阅<");
    expect(html).not.toContain(">终端<");
    expect(html).toContain(">浏览器<");
    expect(html).not.toContain(">侧边任务<");
    expect(html).toContain("desktop-root-inspector-expanded");
    expect(html).toContain('aria-label="Collapse inspector"');
    expect(html).toContain('aria-label="Resize inspector"');
    expect(html).toContain("aw-task-welcome__logo");
    expect(html).toContain("把任务交给 Awaken，自主执行，唤醒自由。");
    expect(html).toContain('placeholder=""');
    expect(html).toContain('rows="3"');
    expect(html).toContain("New Agent");
    expect(html).not.toContain("⌘N");
    expect(html).toContain('aria-label="创建项目"');
    expect(html).toContain('aria-label="展开全部项目"');
    expect(html).toContain('aria-label="新增对话"');
    expect(html).toContain(">对话<");
    expect(html).toContain('aria-label="选择项目，当前：选择项目"');
    expect(html).toContain(">选择项目<");
    expect(html).toContain("连接器");
    expect(html).toContain("技能");
    expect(html).toContain("请点击头像配置模型");
    expect(html).not.toContain(">Models<");
    expect(html).toContain("composer-bar-center");
    expect(html).not.toContain("Plan, Build, / for skills, @ for context");
    expect(html).not.toContain("composer-bar-dock");
    expect(html).not.toContain("Planning next moves");
    expect(html).not.toContain("Ready for a run");
    expect(html).not.toContain("No narrative yet");
    expect(html).not.toContain("等待运行");
    expect(html).not.toContain("run-stage-header");
    expect(html).not.toContain("run-timeline-shell");
    expect(html).toContain("composer-runtime-panels");
    expect(html).toContain("Starting Awaken");
  });

  it("only exposes inspector controls on the main workbench", () => {
    expect(workbenchInspectorPresentation(undefined, false)).toEqual({
      available: true,
      effectiveCollapsed: false,
      rootClassName: "desktop-root-inspector-expanded",
    });
    expect(workbenchInspectorPresentation(undefined, true)).toEqual({
      available: true,
      effectiveCollapsed: true,
      rootClassName: "desktop-root-inspector-collapsed",
    });
    expect(workbenchInspectorPresentation("connectors", false)).toEqual({
      available: false,
      effectiveCollapsed: true,
      rootClassName: "desktop-root-inspector-hidden",
    });
    expect(workbenchInspectorPresentation("skills", true)).toEqual({
      available: false,
      effectiveCollapsed: true,
      rootClassName: "desktop-root-inspector-hidden",
    });
  });

  it("toggles the subagent inspector detail without changing task identity", () => {
    expect(nextSubagentInspectorTaskId(undefined, "task-1")).toBe("task-1");
    expect(nextSubagentInspectorTaskId("task-1", "task-1")).toBeUndefined();
  });

  it("uses the dock composer only after a run exists or narrative appears", () => {
    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: false,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(true);

    expect(
      shouldUseCenterComposer({
        hasNarrative: true,
        hasRun: false,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(false);

    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: true,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(false);

    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: false,
        hasNotice: true,
        hasError: true,
        needsRestart: true,
      }),
    ).toBe(true);
  });

  it("shows only the trailing planning item while a run awaits its first visible output", () => {
    const prompt: RunNarrativeItem = {
      kind: "prompt",
      id: "prompt:draft:1",
      text: "Start the run",
    };
    const planning: RunNarrativeItem = {
      kind: "planning",
      id: "planning:draft:1",
      text: "Planning next moves",
      status: "done",
    };
    const thinking: RunNarrativeItem = {
      kind: "thinking",
      id: "thinking:run-1:reasoning-1",
      text: "Inspecting the current state",
      status: "active",
    };

    expect(
      projectVisibleNarrativeItems({
        items: [prompt, planning],
        isBusy: true,
      }),
    ).toEqual([prompt, planning]);

    expect(
      projectVisibleNarrativeItems({
        items: [prompt, planning, thinking],
        isBusy: true,
      }),
    ).toEqual([prompt, thinking]);

    expect(
      projectVisibleNarrativeItems({
        items: [prompt, planning],
        isBusy: false,
      }),
    ).toEqual([prompt]);
  });

  it("shows turn markers once the run stage has at least three turns", () => {
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: true,
        turnCount: 4,
      }),
    ).toBe(false);
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: false,
        turnCount: 2,
      }),
    ).toBe(false);
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: false,
        turnCount: 3,
      }),
    ).toBe(true);
  });

  it("adapts turn marker density to the available rail height", () => {
    const eightTurns = Array.from({ length: 8 }, (_, index) => `turn-${index + 1}`);
    expect(
      turnMarkerRailDisplay({
        showCenterComposer: false,
        turnIds: eightTurns,
        latestTurnId: "turn-8",
        railHeight: 420,
      }),
    ).toEqual({
      mode: "regular",
      items: eightTurns.map((id, index) => ({ id, index })),
    });

    const twentyTurns = Array.from({ length: 20 }, (_, index) => `turn-${index + 1}`);
    const compactDisplay = turnMarkerRailDisplay({
      showCenterComposer: false,
      turnIds: twentyTurns,
      latestTurnId: "turn-20",
      railHeight: 420,
    });
    expect(compactDisplay.mode).toBe("compact");
    expect(compactDisplay.items).toHaveLength(20);
  });

  it("samples dense turn markers around the active turn without dropping endpoints", () => {
    const fortyTurns = Array.from({ length: 40 }, (_, index) => `turn-${index + 1}`);
    const display = turnMarkerRailDisplay({
      showCenterComposer: false,
      turnIds: fortyTurns,
      activeTurnId: "turn-20",
      latestTurnId: "turn-40",
      railHeight: 110,
    });

    expect(display.mode).toBe("summary");
    expect(display.items.length).toBeLessThanOrEqual(8);
    expect(display.items.map((item) => item.id)).toEqual(
      expect.arrayContaining(["turn-1", "turn-19", "turn-20", "turn-21", "turn-40"]),
    );
  });

  it("hides dense markers when the rail is too short to render a useful set", () => {
    const turns = Array.from({ length: 40 }, (_, index) => `turn-${index + 1}`);
    expect(
      turnMarkerRailDisplay({
        showCenterComposer: false,
        turnIds: turns,
        latestTurnId: "turn-40",
        railHeight: 20,
      }),
    ).toEqual({ mode: "hidden", items: [] });
  });

  it("counts real diff additions and removals for inspector deltas", () => {
    expect(
      diffLineStats(
        [
          "diff --git a/file b/file",
          "--- a/file",
          "+++ b/file",
          "-old",
          "+new",
          "+another",
          " context",
        ].join("\n"),
      ),
    ).toEqual({ added: 2, removed: 1 });
  });

  it("groups prompt-led narrative items into turn sections", () => {
    const grouped = groupNarrativeByTurns([
      { kind: "thinking", id: "intro", text: "warmup", status: "done" },
      {
        kind: "prompt",
        id: "prompt:thread-1:1",
        text: "你好",
        attachments: [
          {
            id: "image-1",
            kind: "image",
            mimeType: "image/png",
            dataBase64: "aGVsbG8=",
            displayName: "clip.png",
            byteSize: 5,
          },
        ],
      },
      {
        kind: "thinking",
        id: "thinking:1",
        text: "first thought",
        status: "done",
      },
      {
        kind: "compactBoundary",
        id: "compact:thread-1:compact_1",
        status: "finished",
        title: "已自动压缩上下文",
        boundaryId: "compact_1",
      },
      {
        kind: "answer",
        id: "answer:1",
        markdown: "first answer",
        status: "done",
      },
      { kind: "prompt", id: "prompt:thread-1:2", text: "再次告诉我" },
      {
        kind: "answer",
        id: "answer:2",
        markdown: "second answer",
        status: "active",
      },
    ]);

    expect(grouped.leadItems).toEqual([
      { kind: "thinking", id: "intro", text: "warmup", status: "done" },
    ]);
    expect(grouped.turns).toHaveLength(2);
    expect(grouped.turns[0].prompt.text).toBe("你好");
    expect(grouped.turns[0].prompt.attachments).toHaveLength(1);
    expect(grouped.turns[0].items).toEqual([
      {
        kind: "thinking",
        id: "thinking:1",
        text: "first thought",
        status: "done",
      },
      {
        kind: "compactBoundary",
        id: "compact:thread-1:compact_1",
        status: "finished",
        title: "已自动压缩上下文",
        boundaryId: "compact_1",
      },
      {
        kind: "answer",
        id: "answer:1",
        markdown: "first answer",
        status: "done",
      },
    ]);
    expect(grouped.turns[1].prompt.text).toBe("再次告诉我");
    expect(grouped.turns[1].items).toEqual([
      {
        kind: "answer",
        id: "answer:2",
        markdown: "second answer",
        status: "active",
      },
    ]);
  });

  it("keeps the turn body flat until the final answer is marked final", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const tool = {
      kind: "tool",
      id: "tool:1",
      title: "Read file",
      detail: "docs/README.md",
      status: "done",
    } as const;
    const answer = {
      kind: "answer",
      id: "answer:1",
      markdown: "streaming answer",
      status: "active",
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, tool, answer],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "items",
        id: "turn-items:prompt:thread-1:1",
        items: [thinking, tool, answer],
      },
    ]);
  });

  it("collapses all pre-final narrative into an execution-process segment once the final answer exists", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const interimAnswer = {
      kind: "answer",
      id: "answer:1",
      markdown: "我先检查目录结构。",
      status: "done",
    } as const;
    const tool = {
      kind: "tool",
      id: "tool:1",
      title: "Read file",
      detail: "docs/README.md",
      status: "done",
    } as const;
    const finalAnswer = {
      kind: "answer",
      id: "answer:2",
      markdown: "这是最终回答。",
      status: "done",
      isFinal: true,
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, interimAnswer, tool, finalAnswer],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "executionProcess",
        id: "execution-process:prompt:thread-1:1",
        title: "执行过程",
        items: [thinking, interimAnswer, tool],
        open: false,
      },
      {
        kind: "items",
        id: "answer-and-after:prompt:thread-1:1",
        items: [finalAnswer],
      },
    ]);
  });

  it.each(["image", "video"] as const)(
    "folds a ready %s result and every surrounding process item into execution process",
    (mediaKind) => {
      const thinking = {
        kind: "thinking",
        id: "thinking:media",
        text: "准备提示词",
        status: "done",
      } as const;
      const media: Extract<RunNarrativeItem, { kind: "mediaGeneration" }> = {
        kind: "mediaGeneration",
        id: "media:run-1:call-1",
        toolCallId: "call-1",
        runId: "run-1",
        mediaKind,
        title: mediaKind === "image" ? "图片生成" : "视频生成",
        detail: mediaKind === "image" ? "图片已生成" : "视频已生成",
        status: "ready",
        artifactPaths: [
          mediaKind === "image"
            ? "output/generated-media/image.png"
            : "output/generated-media/video.mp4",
        ],
      };
      const postMediaThinking = {
        kind: "thinking",
        id: "thinking:publish",
        text: "准备发布媒体交付物",
        status: "done",
      } as const;
      const interimAnswer = {
        kind: "answer",
        id: "answer:publish",
        markdown: "媒体已生成，让我发布交付物。",
        status: "done",
      } as const;
      const publishTool = {
        kind: "tool",
        id: "tool:publish",
        title: "artifact_publish",
        status: "done",
      } as const;
      const readGroup: Extract<RunNarrativeItem, { kind: "exploring" }> = {
        kind: "exploring",
        id: "exploring:read",
        actions: [],
        status: "done",
      };
      const answer = {
        kind: "answer",
        id: "answer:media",
        markdown: "图片已经完成。",
        status: "done",
        isFinal: true,
      } as const;

      expect(projectNarrativeTurnBodySegments({
        turnId: "prompt:media",
        items: [
          thinking,
          media,
          postMediaThinking,
          interimAnswer,
          publishTool,
          readGroup,
          answer,
        ],
        executionProcessOpen: false,
      })).toEqual([
        {
          kind: "executionProcess",
          id: "execution-process:prompt:media",
          title: "执行过程",
          items: [
            thinking,
            media,
            postMediaThinking,
            interimAnswer,
            publishTool,
            readGroup,
          ],
          open: false,
        },
        {
          kind: "items",
          id: "answer-and-after:prompt:media",
          items: [answer],
        },
      ]);
    },
  );

  it.each(["partial", "unknown", "error", "cancelled"] as const)(
    "folds a %s media result into execution process",
    (status) => {
      const thinking = {
        kind: "thinking",
        id: `thinking:${status}`,
        text: "等待媒体结果",
        status: "done",
      } as const;
      const media: Extract<RunNarrativeItem, { kind: "mediaGeneration" }> = {
        kind: "mediaGeneration",
        id: `media:run-1:${status}`,
        toolCallId: `call-${status}`,
        runId: "run-1",
        mediaKind: "image",
        title: "图片生成",
        status,
        artifactPaths: [],
      };
      const answer = {
        kind: "answer",
        id: `answer:${status}`,
        markdown: "媒体任务已结束。",
        status: "done",
        isFinal: true,
      } as const;

      const segments = projectNarrativeTurnBodySegments({
        turnId: `prompt:${status}`,
        items: [
          thinking,
          media,
          {
            kind: "thinking",
            id: `thinking:after-${status}`,
            text: "收尾媒体任务",
            status: "done",
          },
          answer,
        ],
        executionProcessOpen: false,
      });
      expect(segments[0]).toMatchObject({
        kind: "executionProcess",
        items: [
          thinking,
          media,
          {
            kind: "thinking",
            id: `thinking:after-${status}`,
            text: "收尾媒体任务",
            status: "done",
          },
        ],
      });
      expect(segments[1]).toMatchObject({
        kind: "items",
        items: [answer],
      });
    },
  );

  it("keeps execution-process items visible when the disclosure is expanded", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const fileChange: Extract<RunNarrativeItem, { kind: "fileChange" }> = {
      kind: "fileChange",
      id: "file:1",
      files: [
        {
          id: "file-entry:1",
          title: "App.tsx",
          status: "done",
        },
      ],
      status: "done",
    };
    const answer = {
      kind: "answer",
      id: "answer:1",
      markdown: "done answer",
      status: "done",
      isFinal: true,
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, fileChange, answer],
        executionProcessOpen: true,
      }),
    ).toEqual([
      {
        kind: "executionProcess",
        id: "execution-process:prompt:thread-1:1",
        title: "执行过程",
        items: [thinking, fileChange],
        open: true,
      },
      {
        kind: "items",
        id: "answer-and-after:prompt:thread-1:1",
        items: [answer],
      },
    ]);
  });

  it("keeps the turn body flat before any answer item exists", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const legacyTool = {
      kind: "tool",
      id: "tool:legacy-1",
      title: "Legacy tool record",
      status: "done",
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, legacyTool],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "items",
        id: "turn-items:prompt:thread-1:1",
        items: [thinking, legacyTool],
      },
    ]);
  });

  it("calculates the scroll position that pins a prompt at the anchor", () => {
    expect(
      scrollTopForPromptAnchor({
        currentScrollTop: 240,
        containerTop: 100,
        promptTop: 460,
        anchorOffset: 0,
      }),
    ).toBe(600);
    expect(
      scrollTopForPromptAnchor({
        currentScrollTop: 0,
        containerTop: 100,
        promptTop: 50,
        anchorOffset: 0,
      }),
    ).toBe(0);
  });

  it("selects the turn nearest to the prompt anchor for marker state", () => {
    const turns = [
      { id: "turn-1", top: 100 },
      { id: "turn-2", top: 260 },
      { id: "turn-3", top: 480 },
    ];

    expect(activeTurnIdForAnchor({ turns: [], anchorY: 200 })).toBeUndefined();
    expect(activeTurnIdForAnchor({ turns, anchorY: 90 })).toBe("turn-1");
    expect(activeTurnIdForAnchor({ turns, anchorY: 260 })).toBe("turn-2");
    expect(activeTurnIdForAnchor({ turns, anchorY: 999 })).toBe("turn-3");
  });

  it("calculates composer bottom safe area", () => {
    expect(
      composerBottomSafeArea({
        composerHeight: 88,
        safeGap: 24,
      }),
    ).toBe(112);
    expect(
      composerBottomSafeArea({
        composerHeight: -1,
        safeGap: 24,
      }),
    ).toBe(24);
  });

  it("keeps turn marker rail anchored to the composer bar when runtime panels are tall", () => {
    expect(
      turnMarkerRailHeight({
        scrollViewportHeight: 520,
        composerContentInset: 360,
        bottomSafeArea: 152,
      }),
    ).toBe(700);
  });

  it("measures composer reserve from real dock content instead of the fade", () => {
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        contentTop: 590,
        fallbackInset: 162,
      }),
    ).toBe(130);
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        fallbackInset: 162,
      }),
    ).toBe(162);
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        contentTop: 760,
        fallbackInset: 162,
      }),
    ).toBe(0);
  });

  it("keeps the latest turn tall enough for sticky prompt reading", () => {
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
      }),
    ).toBe(640);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 12,
      }),
    ).toBe(628);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
        bottomReserve: 124,
      }),
    ).toBe(516);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
        bottomReserve: 72,
      }),
    ).toBe(568);
  });

  it("reserves the composer gap for every latest turn height state", () => {
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: true,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: false,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: true,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(72);
  });

  it("does not add temporary prompt slack that creates a second bottom target", () => {
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: true,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: true,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(0);
  });

  it("keeps prompt-only latest turns from creating blank scroll bottom", () => {
    const containerHeight = 640;
    const outputGap = 72;
    const latestTurnHeight =
      anchoredTurnMinHeight({
        containerHeight,
        anchorOffset: 0,
        bottomReserve: latestTurnMinHeightBottomReserve({
          hasBodyItems: false,
          isPromptAnchorPending: true,
          outputGap,
        }),
      }) +
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap,
      });

    expect(latestTurnHeight + outputGap).toBe(containerHeight);
  });

  it("uses the same native bottom target for programmatic follow and manual scroll bottom", () => {
    const manualScrollBottom = 620;
    expect(
      scrollTopForLatestTurnScrollTail({
        maxScrollTop: manualScrollBottom,
      }),
    ).toBe(manualScrollBottom);
  });

  it("clamps the latest turn follow target to the native bottom instead of a prompt anchor", () => {
    expect(
      scrollTopForLatestTurnScrollTail({
        maxScrollTop: 320,
      }),
    ).toBe(320);
  });

  it("follows the latest turn tail after prompt anchoring and body output exist", () => {
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: false,
        isPromptAnchorPending: false,
      }),
    ).toBe(false);
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: true,
        isPromptAnchorPending: true,
      }),
    ).toBe(false);
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: true,
        isPromptAnchorPending: false,
      }),
    ).toBe(true);
  });

  it("keeps layout scroll from cancelling the pending prompt anchor", () => {
    expect(
      shouldCancelPendingPromptAnchorForScrollIntent({
        hasDirectUserInput: false,
      }),
    ).toBe(false);
    expect(
      shouldCancelPendingPromptAnchorForScrollIntent({
        hasDirectUserInput: true,
      }),
    ).toBe(true);
  });

  it("detects whether the user is still following the latest output", () => {
    expect(
      isNearLatestTurnOutputTail({
        containerTop: 100,
        outputBottom: 650,
        containerHeight: 640,
        bottomReserve: 120,
        threshold: 48,
      }),
    ).toBe(true);
    expect(
      isNearLatestTurnOutputTail({
        containerTop: 100,
        outputBottom: 700,
        containerHeight: 640,
        bottomReserve: 120,
        threshold: 48,
      }),
    ).toBe(false);
  });

  it("detects whether the native scroll position is at the bottom", () => {
    expect(
      isNearScrollBottom({
        scrollTop: 620,
        scrollHeight: 1260,
        clientHeight: 640,
        threshold: 2,
      }),
    ).toBe(true);
    expect(
      isNearScrollBottom({
        scrollTop: 600,
        scrollHeight: 1260,
        clientHeight: 640,
        threshold: 2,
      }),
    ).toBe(false);
  });

  it("pauses latest output follow as soon as the user scrolls toward history", () => {
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: -12 })).toBe(true);
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: 12 })).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 240,
        nextScrollTop: 220,
        isAtScrollBottom: false,
      }),
    ).toBe(true);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 240,
        nextScrollTop: 260,
        isAtScrollBottom: false,
      }),
    ).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: null,
        nextScrollTop: 220,
        isAtScrollBottom: false,
      }),
    ).toBe(false);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: 12 })).toBe(true);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: -12 })).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForTouchMove({
        startY: 100,
        currentY: 112,
      }),
    ).toBe(true);
    expect(
      shouldPauseLatestOutputFollowForTouchMove({
        startY: 100,
        currentY: 96,
      }),
    ).toBe(false);
    expect(
      shouldRequestLatestOutputFollowResumeForTouchMove({
        startY: 100,
        currentY: 88,
      }),
    ).toBe(true);
    expect(
      shouldRequestLatestOutputFollowResumeForTouchMove({
        startY: 100,
        currentY: 112,
      }),
    ).toBe(false);
  });

  it("does not pause latest output follow when the browser passively clamps a shrinking tail", () => {
    // scrollHeight 收缩（mermaid 坍缩、process Disclosure 折叠）时浏览器把
    // 贴底的 scrollTop 钳小：下降幅度再大，只要仍贴底就不是用户上滑。
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 1160,
        nextScrollTop: 560,
        isAtScrollBottom: true,
      }),
    ).toBe(false);
    // 同样的下降发生在离开底部之后，仍是真实的用户上滑。
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 1160,
        nextScrollTop: 560,
        isAtScrollBottom: false,
      }),
    ).toBe(true);
  });

  it("does not resume latest output follow until the user asks to return to the tail", () => {
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: false,
      }),
    ).toBe(false);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: false,
        hasResumeIntent: true,
      }),
    ).toBe(false);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: true,
      }),
    ).toBe(true);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: false,
        isNearTail: true,
        isAtScrollBottom: false,
        hasResumeIntent: false,
      }),
    ).toBe(true);
  });

  it("requires explicit resume intent after an upward wheel pause", () => {
    const pausedByWheel = shouldPauseLatestOutputFollowForWheel({
      deltaY: -18,
    });
    expect(pausedByWheel).toBe(true);

    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: pausedByWheel,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: false,
      }),
    ).toBe(false);

    const resumeIntent = shouldRequestLatestOutputFollowResumeForWheel({
      deltaY: 18,
    });
    expect(resumeIntent).toBe(true);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: pausedByWheel,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: resumeIntent,
      }),
    ).toBe(true);
  });

  it("shows return-to-latest whenever the user paused away from the latest turn", () => {
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: true,
      }),
    ).toBe(true);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "following_latest",
        hasLatestTurn: true,
      }),
    ).toBe(false);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: true,
      }),
    ).toBe(true);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: false,
      }),
    ).toBe(false);
  });
});
