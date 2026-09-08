/** @vitest-environment jsdom */
import { describe, expect, it, vi } from "vitest";
import { useRunSubmission } from "./useRunSubmission";
import { ThreadSession } from "./runtime/thread-session";
import type { RuntimeRun } from "../shared/runtime";
import type { RuntimeAction } from "./runtime/reducer";

type Options = Parameters<typeof useRunSubmission>[0];
function setup(startRun: Options["runtimeClient"]["startRun"]) {
  const client = {
    startRun,
    validateWorkspace: vi.fn(),
    subscribeRun: vi.fn(async () => vi.fn()),
    getThreadTranscript: vi.fn(),
  } as unknown as Options["runtimeClient"];
  const dispatch = vi.fn<(action: RuntimeAction) => void>();
  const storage = new Map<string, string>();
  const session = new ThreadSession({
    client,
    dispatch,
    storage: {
      getItem: (key) => storage.get(key) ?? null,
      setItem: (key, value) => {
        storage.set(key, value);
      },
      removeItem: (key) => {
        storage.delete(key);
      },
    },
    onTranscript: vi.fn(),
  });
  session.begin("A");
  const registry: Options["registry"] = {
    workspacePath: "",
    workspaceTargetPathRef: { current: "" },
    conversationScope: true,
    selectedProjectId: undefined,
    defaultWorkspacePath: "",
    defaultProjectRemoved: false,
    activeThreadWorkspacePath: undefined,
    projectsFromWorkspace: [],
    defaultProjectDisplayName: undefined,
    effectiveDefaultWorkspacePath: undefined,
    isWorkspaceValid: true,
    backendProjectIdFor: () => undefined,
    setRemovedProjectIds: vi.fn(),
  };
  const draft = {
    draft: {
      value: "isolated test",
      attachments: [],
      tokens: [],
      comments: [],
      tokenPlacements: {},
      tokenLeadingTexts: {},
    },
    clearDraft: vi.fn(),
    replaceDraft: vi.fn(),
  } as unknown as Options["draft"];
  const options: Options = {
    runtimeClient: client,
    threadSession: session,
    dispatch,
    runtimeState: { activeThreadId: "A", runId: "run-A" },
    desktopReady: true,
    desktopNeedsRestart: false,
    isBusy: false,
    registry,
    draft,
    composerDraftKey: "thread:A",
    fileWorkspaceSurfaceActive: false,
    setExpandedFileComposerTrackedRun: vi.fn(),
    prepareForRunStart: vi.fn(),
    refreshThreads: vi.fn(async () => {}),
    startThreadTitlePoll: vi.fn(),
    setNotice: vi.fn(),
  };
  return {
    options,
    session,
    client,
    dispatch,
    draft,
    actions: useRunSubmission(options),
  };
}

describe("run submission coordination", () => {
  it("does not attach a late run response to the task selected while it was pending", async () => {
    let resolve!: (run: RuntimeRun) => void;
    const pending = new Promise<RuntimeRun>((done) => {
      resolve = done;
    });
    const f = setup(vi.fn(() => pending));
    const sending = f.actions.handleSubmitWithPrompt();
    f.session.begin("B");
    resolve({
      threadId: "A",
      runId: "run-new-A",
      status: "running",
      createdAt: "",
      updatedAt: "",
      cancelRequested: false,
    });
    await sending;
    expect(
      f.dispatch.mock.calls.some(([action]) => action.type === "run/created"),
    ).toBe(false);
    expect(f.client.subscribeRun).not.toHaveBeenCalled();
    expect(f.options.refreshThreads).toHaveBeenCalled();
  });

  it("restores a failed submission to its original draft bucket", async () => {
    const f = setup(
      vi.fn(async () => {
        throw new Error("offline");
      }),
    );
    await f.actions.handleSubmitWithPrompt();
    expect(f.draft.replaceDraft).toHaveBeenCalledWith(
      expect.objectContaining({ value: "isolated test" }),
      "thread:A",
    );
    expect(f.dispatch).toHaveBeenLastCalledWith({
      type: "run/createFailed",
      message: "offline",
    });
  });
});
