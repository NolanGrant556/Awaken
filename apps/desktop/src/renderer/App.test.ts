import React from "react";
import {
  AwakenComposerBar,
  AwakenProjectTree,
  AwakenUserPromptBlock,
  pastedImageFilesFromClipboardData,
  shouldSubmitComposerInputKey,
  slashPaletteStyleForAnchor,
  slashPreviewStyleForAnchor,
} from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  App,
  isUserQuestionSelectionAnswered,
  nextUserQuestionPromptIndex,
  promptNarrativeTokensForComposer,
  promptTextAfterTokenLeadingText,
  promptTokenViews,
  reconcileComposerStructuredInput,
  resolveWorkspaceRootForRun,
  selectedUserQuestionLabels,
  shouldResetActiveThreadForWorkspaceChange,
  shouldShowSidebarThreadSpinner,
  workbenchInspectorPresentation,
  UserQuestionPrompt,
} from "./App";
import {
  actionsFromSlashProviders,
  clearSlashTriggerFromComposerValue,
  composerTokenIdForKeyboardDelete,
  composerTokenPlacementForSlashTrigger,
  composerTokenFromSlashAction,
  filterSlashActions,
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
} from "./projectWorkspace";
import {
  anchoredTurnMinHeight,
  activeTurnIdForAnchor,
  composerBottomSafeArea,
  composerContentBottomInset,
  diffLineStats,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  isNearScrollBottom,
  latestTurnMinHeightBottomReserve,
  latestTurnPromptAnchorSlack,
  projectNarrativeTurnBodySegments,
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

function stubElectronBridge(): void {
  vi.stubGlobal("window", {
    awaken: {
      platform: "macos",
      relaunchDesktop: vi.fn(async () => undefined),
      getRun: vi.fn(),
      getThreadTranscript: vi.fn(),
      listThreads: vi.fn(async () => []),
      renameThread: vi.fn(),
      deleteThread: vi.fn(async () => ({ ok: true as const })),
      listProjects: vi.fn(async () => []),
      upsertProject: vi.fn(),
      detachProject: vi.fn(),
      deleteProjectThreads: vi.fn(),
      trashProject: vi.fn(),
      getProjectOperation: vi.fn(),
      openArtifact: vi.fn(async () => ({ ok: true as const })),
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
      getSkillPreview: vi.fn(async () => ({ frontmatterRaw: "", bodyMarkdown: "" })),
      installSkill: vi.fn(),
      deleteSkill: vi.fn(),
      listModelProviders: vi.fn(async () => []),
      listModelProfiles: vi.fn(async () => []),
      saveModelProfile: vi.fn(),
      deleteModelProfile: vi.fn(),
      setDefaultModelProfile: vi.fn(),
      testModelProfile: vi.fn(async () => ({
        ok: true as const,
        value: { ok: true, message: "ok" },
      })),
      syncModelCapabilities: vi.fn(async () => []),
      getModelProfileBalances: vi.fn(async () => ({ balances: [] })),
      pickAttachments: vi.fn(async () => ({ canceled: true as const })),
      pickDirectory: vi.fn(async () => ({ canceled: true as const })),
      pickWorkspace: vi.fn(async () => ({ canceled: true as const })),
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
    } satisfies AwakenBridge,
  });
}

describe("shouldResetActiveThreadForWorkspaceChange", () => {
  it("returns false when no active thread workspace exists", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(undefined, "/tmp/project"),
    ).toBe(false);
  });

  it("returns false when switching within the same workspace", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(
        "/tmp/project",
        "/tmp/project",
      ),
    ).toBe(false);
  });

  it("returns true when switching to a different workspace", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(
        "/tmp/project-a",
        "/tmp/project-b",
      ),
    ).toBe(true);
  });
});

describe("resolveWorkspaceRootForRun", () => {
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
});

describe("AwakenComposerInput keyboard behavior", () => {
  it("submits with Enter and keeps Shift+Enter for new lines", () => {
    expect(shouldSubmitComposerInputKey({ key: "Enter" })).toBe(true);
    expect(shouldSubmitComposerInputKey({ key: "Enter", shiftKey: true })).toBe(
      false,
    );
    expect(
      shouldSubmitComposerInputKey({ key: "Enter", isComposing: true }),
    ).toBe(false);
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

    expect(actions.map((action) => action.label)).toEqual([
      "Daily Followup",
      "mcp-skill",
    ]);
    expect(actions.every((action) => !action.disabled)).toBe(true);
    expect(filterSlashActions(actions, "powerpoint")).toHaveLength(0);
    expect(filterSlashActions(actions, "mcp provided")).toHaveLength(1);
    expect(filterSlashActions(actions, "follow-up")[0]?.id).toContain(
      "daily-followup",
    );
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
    expect(
      composerTokenPlacementForSlashTrigger(
        slashTriggerFromComposerValue("hello /doc"),
      ),
    ).toBe("after");
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

    expect(actions.map((action) => action.type)).toEqual([
      "file",
      "skill",
      "skill",
    ]);
    expect(actions[0]?.label).toBe("brief.md");
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
    expect(
      shouldShowAssistantAnswerFooter({ status: "done", isFinal: true }),
    ).toBe(true);
    expect(shouldShowAssistantAnswerFooter({ status: "done" })).toBe(false);
    expect(
      shouldShowAssistantAnswerFooter({ status: "active", isFinal: true }),
    ).toBe(false);
    expect(
      shouldShowAssistantAnswerFooter({ status: "error", isFinal: true }),
    ).toBe(false);
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
    expect(
      slashPaletteStyleForAnchor(160, 700)["--composer-bar-slash-max-height"],
    ).toBe("146px");
    expect(
      slashPaletteStyleForAnchor(520, 700)["--composer-bar-slash-max-height"],
    ).toBe("210px");
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
    expect(html.indexOf("你好，你看到这个 skill 不?")).toBeLessThan(
      html.indexOf("deep-research"),
    );
    expect(html.indexOf("deep-research")).toBeLessThan(
      html.indexOf("请继续"),
    );
    const promptStart = html.indexOf(
      '<p class="execution-prompt-bubble-text">',
    );
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

    expect(html.indexOf("deep-research")).toBeLessThan(
      html.indexOf("你好，你看到这个 skill 不?"),
    );
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

    expect(
      promptTextAfterTokenLeadingText("请使用  这个技能写", tokens),
    ).toBe(" 这个技能写");
    expect(promptTextAfterTokenLeadingText("请使用", tokens)).toBe("");
  });
});

describe("structured Composer draft reconciliation", () => {
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

  it("waits for the durable project state before reconciling a terminal trash operation", () => {
    expect(canReconcileTrackedTrashOperation("failed", "trash_pending")).toBe(false);
    expect(canReconcileTrackedTrashOperation("failed", "active")).toBe(true);
    expect(canReconcileTrackedTrashOperation("succeeded", "trash_pending")).toBe(false);
    expect(canReconcileTrackedTrashOperation("succeeded", "removed")).toBe(true);
  });

  it("persists valid trash trackers, removes malformed entries, and replaces duplicates", () => {
    const tracked = {
      operationId: "operation-1",
      projectId: "project-ui-1",
      backendProjectId: "project-backend-1",
      label: "待删除项目",
      wasExpanded: true,
    };
    expect(
      parseTrackedTrashOperations(
        JSON.stringify([tracked, { operationId: "broken" }, tracked]),
      ),
    ).toEqual([tracked]);
    expect(parseTrackedTrashOperations("not-json")).toEqual([]);
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
    expect(normalizeWorkspacePath("/Users/nolangrant/Awaken/")).toBe(
      "/Users/nolangrant/Awaken",
    );
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
      projectIdForThread(
        { threadId: "thread-1", workspaceRoot: "/tmp/demo" },
        "/tmp/default",
      ),
    ).toBeUndefined();
    expect(
      projectIdForThread(
        { threadId: "thread-1", workspaceRoot: "/tmp/default" },
        "/tmp/default",
      ),
    ).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
  });

  it("removes folder projects", () => {
    const projects = [
      { id: "project-demo", label: "demo", path: "/tmp/demo" },
      { id: "project-keep", label: "keep", path: "/tmp/keep" },
    ];

    expect(removeWorkspaceProject(projects, "project-demo")).toEqual(
      [{ id: "project-keep", label: "keep", path: "/tmp/keep" }],
    );
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
  });
});

describe("App workbench presentation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders idle as a blank execution stage with the light shell", () => {
    stubElectronBridge();

    const html = renderToString(React.createElement(App));

    expect(html).toContain("On Awaken");
    expect(html).toContain("desktop-root-inspector-expanded");
    expect(html).toContain('aria-label="Collapse inspector"');
    expect(html).toContain('aria-label="Resize inspector"');
    expect(html).toContain("aw-task-welcome__logo");
    expect(html).toContain("把任务交给 Awaken，自主执行，唤醒自由。");
    expect(html).toContain('placeholder=""');
    expect(html).toContain('rows="3"');
    expect(html).toContain("New Agent");
    expect(html).not.toContain("⌘N");
    expect(html).toContain("Add folder");
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
    const eightTurns = Array.from(
      { length: 8 },
      (_, index) => `turn-${index + 1}`,
    );
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

    const twentyTurns = Array.from(
      { length: 20 },
      (_, index) => `turn-${index + 1}`,
    );
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
    const fortyTurns = Array.from(
      { length: 40 },
      (_, index) => `turn-${index + 1}`,
    );
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
      expect.arrayContaining([
        "turn-1",
        "turn-19",
        "turn-20",
        "turn-21",
        "turn-40",
      ]),
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
      }),
    ).toBe(true);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 240,
        nextScrollTop: 260,
      }),
    ).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: null,
        nextScrollTop: 220,
      }),
    ).toBe(false);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: 12 })).toBe(
      true,
    );
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: -12 })).toBe(
      false,
    );
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
