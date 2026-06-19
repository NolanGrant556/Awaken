import { useEffect, useReducer, useRef, useState } from "react";
import {
  AppShell,
  ApprovalCard,
  ArtifactGrid,
  CenterStage,
  Composer,
  RunTimeline,
  Sidebar,
  TopBar,
  WindowTitleBar,
  type ComposerMode,
  type ConversationItem,
  type ModelOption,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/ui";
import { createRuntimeClient, type SubscribeHandlers } from "./runtime/client";
import type { RuntimeInfo, RuntimeRunStatus, StartRunRequest } from "../shared/runtime";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
} from "./runtime/reducer";
import type { ApprovalDecision } from "../shared/runtime";
import { A2UISurfaceView } from "./a2ui/A2UISurfaceView";

const models: ModelOption[] = [
  { id: "awaken-core", label: "awaken-core" },
];

const projects: ProjectItem[] = [
  {
    id: "awaken",
    label: "Awaken",
    timeLabel: "当前",
    children: [
      {
        id: "desktop-phase-4",
        label: "Desktop Phase 4",
        timeLabel: "runtime",
      },
    ],
  },
];

function formatStatusLabel(
  phase: ReturnType<typeof createInitialRuntimeState>["phase"],
  status?: ReturnType<typeof createInitialRuntimeState>["status"],
): string {
  if (status === "awaiting_approval") {
    return "Awaiting approval";
  }
  switch (phase) {
    case "starting":
      return "Creating run";
    case "running":
      return "Streaming";
    case "reconnecting":
      return "Reconnecting...";
    case "cancelling":
      return "Cancelling";
    case "done":
      return "Completed";
    case "cancelled":
      return "Cancelled";
    case "error":
      return "Error";
    default:
      return "Idle";
  }
}

export function App() {
  const runtimeClientRef = useRef(createRuntimeClient());
  const unsubscribeRef = useRef<null | (() => void)>(null);
  const terminalSeenRef = useRef(false);
  const resolvingApprovalIdRef = useRef<string | null>(null);
  const threadLoadSeqRef = useRef(0);
  const runtimeClient = runtimeClientRef.current;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const [composerValue, setComposerValue] = useState("");
  const [mode, setMode] = useState<ComposerMode>("Plan");
  const [selectedModel, setSelectedModel] = useState<ModelOption>(models[0]);
  const [activeNavId, setActiveNavId] = useState("runs");
  const [notice, setNotice] = useState<string | null>(null);
  const [resolvingApprovalId, setResolvingApprovalId] = useState<string | null>(null);
  const [resolvedApproval, setResolvedApproval] = useState<{
    approvalId: string;
    decision: "approved" | "rejected";
  } | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [workspacePath, setWorkspacePath] = useState<string>("");

  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      try {
        const info = await runtimeClient.getRuntimeInfo();
        if (!cancelled) {
          setRuntimeInfo(info);
          setConnectionError(null);
        }
      } catch (error) {
        if (!cancelled) {
          setRuntimeInfo(null);
          setConnectionError(error instanceof Error ? error.message : "Runtime unreachable");
        }
      }
    }

    void poll();
    const interval = setInterval(() => {
      void poll();
    }, 3000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [runtimeClient]);

  useEffect(() => {
    return () => {
      unsubscribeRef.current?.();
      unsubscribeRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (runtimeInfo && workspacePath === "") {
      setWorkspacePath(runtimeInfo.defaultWorkspace);
    }
  }, [runtimeInfo, workspacePath]);

  async function refreshThreads(): Promise<void> {
    try {
      const threads = await runtimeClient.listThreads();
      dispatch({ type: "run/threadsLoaded", threads });
    } catch {
      // Thread listing is best-effort; runtime reachability is already surfaced
      // by the runtime info poll.
    }
  }

  useEffect(() => {
    if (!runtimeInfo) {
      return;
    }
    void refreshThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtimeInfo, runtimeClient]);

  useEffect(() => {
    const persistedRunId = localStorage.getItem("awaken:runId");
    if (!persistedRunId) return;

    let cancelled = false;

    async function recover(): Promise<void> {
      try {
        const run = await runtimeClient.getRun(persistedRunId!);
        if (cancelled) return;

        // Always replay full history (after_seq=0) regardless of run status.
        // For terminal runs, the replay restores timeline/artifacts/final text,
        // and the terminal event in the stream clears localStorage via onEvent.
        dispatch({ type: "run/recovered", run });

        const unsubscribe = await runtimeClient.subscribeRun(
          run.runId,
          createSubscribeHandlers(),
          0,
        );
        if (cancelled) {
          unsubscribe();
          return;
        }
        unsubscribeRef.current = unsubscribe;
      } catch {
        if (!cancelled) {
          localStorage.removeItem("awaken:runId");
        }
      }
    }

    void recover();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtimeClient]);

  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";

  const isWorkspaceValid =
    workspacePath.trim() !== "" && /^([A-Za-z]:[\\/]|\/)/.test(workspacePath.trim());

  const activeApproval = runtimeState.approvals[0];
  const latestArtifact =
    runtimeState.artifacts.length > 0
      ? runtimeState.artifacts[runtimeState.artifacts.length - 1]
      : undefined;
  const a2uiArtifacts = runtimeState.artifacts.filter(
    (artifact) => artifact.kind === "a2ui_surface",
  );

  const navItems: NavItem[] = [
    { id: "runs", label: "Runs" },
    { id: "artifacts", label: "Artifacts", badge: runtimeState.artifacts.length || undefined },
    { id: "approvals", label: "Approvals", badge: runtimeState.approvals.length || undefined },
  ];

  const conversations: ConversationItem[] = runtimeState.threads.map((thread) => ({
    id: thread.threadId,
    label: thread.label,
    timeLabel: thread.runId === runtimeState.runId ? "active" : "earlier",
  }));

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
        if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
          terminalSeenRef.current = true;
          localStorage.removeItem("awaken:runId");
        }
        if (event.type === "RUN_STATUS_CHANGED" && event.status === "cancelled") {
          localStorage.removeItem("awaken:runId");
        }
        if (event.type === "AWAKEN_APPROVAL_REQUESTED" && event.approvalId) {
          setResolvedApproval(null);
        }
        if (
          event.type === "AWAKEN_APPROVAL_RESOLVED" &&
          event.approvalId &&
          event.approvalId === resolvingApprovalIdRef.current
        ) {
          setResolvedApproval({
            approvalId: event.approvalId,
            decision: event.approvalDecision === "rejected" ? "rejected" : "approved",
          });
          resolvingApprovalIdRef.current = null;
          setResolvingApprovalId(null);
        }
        dispatch({ type: "run/event", event });
      },
      onError: (message) => dispatch({ type: "run/streamError", message }),
      onReconnect: () => dispatch({ type: "run/reconnecting" }),
      onClose: () => {
        // Non-terminal close is handled by the client layer's attemptReconnect
        // (which dispatches run/reconnecting and retries). This callback only
        // fires on terminal close; the give-up path (max retries exceeded) is
        // delivered through onError.
        unsubscribeRef.current = null;
      },
    };
  }

  async function handleSubmit(): Promise<void> {
    const prompt = composerValue.trim();
    if (!prompt) {
      setNotice("Input is required");
      return;
    }
    if (isBusy) {
      return;
    }
    if (!isWorkspaceValid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const workspaceRoot = workspacePath.trim();
    const validation = await runtimeClient.validateWorkspace(workspaceRoot);
    if (!validation.valid) {
      setNotice(validation.error);
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    dispatch({ type: "run/requested", prompt });

    try {
      const startRunRequest: StartRunRequest = {
        input: prompt,
        threadId: runtimeState.activeThreadId || undefined,
        workspace: {
          workspaceRoot,
          writableRoots: workspaceRoot ? [workspaceRoot] : [],
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      dispatch({ type: "run/created", run });
      dispatch({
        type: "run/threadAdded",
        threadId: run.threadId,
        label: prompt,
        runId: run.runId,
      });
      void refreshThreads();
      localStorage.setItem("awaken:runId", run.runId);
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
      );
      unsubscribeRef.current = unsubscribe;
      setComposerValue("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create run";
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleSelectConversation(threadId: string): Promise<void> {
    if (threadId === runtimeState.activeThreadId) {
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    const loadSeq = threadLoadSeqRef.current + 1;
    threadLoadSeqRef.current = loadSeq;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    dispatch({ type: "run/threadSwitch", threadId });

    const thread = runtimeState.threads.find((item) => item.threadId === threadId);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      const run = await runtimeClient.getRun(thread.runId);
      if (threadLoadSeqRef.current !== loadSeq) {
        return;
      }
      dispatch({ type: "run/recovered", run });
      if (run.status === "running" || run.status === "awaiting_approval" || run.status === "cancelling") {
        localStorage.setItem("awaken:runId", run.runId);
      } else {
        localStorage.removeItem("awaken:runId");
      }
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
        0,
      );
      if (threadLoadSeqRef.current !== loadSeq) {
        unsubscribe();
        return;
      }
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load thread");
    }
  }

  async function handleCancel(): Promise<void> {
    if (!runtimeState.runId || !isBusy) {
      return;
    }

    dispatch({ type: "run/cancelRequested" });

    try {
      const run = await runtimeClient.cancelRun(runtimeState.runId);
      dispatch({ type: "run/cancelSync", run });
      const terminalStatuses: RuntimeRunStatus[] = ["finished", "error", "cancelled"];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      dispatch({
        type: "run/streamError",
        message: error instanceof Error ? error.message : "Failed to cancel run",
      });
    }
  }

  async function handlePickWorkspace(): Promise<void> {
    try {
      const result = await runtimeClient.pickWorkspace(workspacePath || runtimeInfo?.defaultWorkspace);
      if (!result.canceled) {
        setWorkspacePath(result.path);
        setNotice(null);
      } else if (result.error) {
        setNotice(result.error);
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to pick workspace");
    }
  }

  async function handleResolveApproval(decision: ApprovalDecision): Promise<void> {
    if (!runtimeState.runId || !activeApproval) {
      return;
    }
    if (
      resolvingApprovalId === activeApproval.id ||
      resolvingApprovalIdRef.current === activeApproval.id
    ) {
      return;
    }

    setNotice(null);
    setResolvingApprovalId(activeApproval.id);
    resolvingApprovalIdRef.current = activeApproval.id;
    setResolvedApproval(null);

    try {
      await runtimeClient.resolveApproval(runtimeState.runId, activeApproval.id, decision);
    } catch (error) {
      setResolvingApprovalId(null);
      resolvingApprovalIdRef.current = null;
      setResolvedApproval(null);
      setNotice(error instanceof Error ? error.message : "Failed to resolve approval");
    }
  }

  function handleAddContext(): void {
    setNotice("Context picker is not wired yet. Phase 4 only closes the run, approval, and artifact runtime loop.");
  }

  const platform = (runtimeClient.platform === "web"
    ? "macos"
    : runtimeClient.platform) as PlatformKind;

  return (
    <>
      <div className="desktop-root">
        <WindowTitleBar platform={platform} />
        <AppShell
          platform={platform}
          sidebar={
            <Sidebar
              title="Awaken"
              navItems={navItems}
              activeNavId={activeNavId}
              conversations={conversations}
              activeConversationId={runtimeState.activeThreadId}
              projects={projects}
              activeProjectId="desktop-phase-4"
              defaultExpandedIds={["awaken"]}
              profileName="Desktop Consumer"
              profileAvatarFallback="AW"
              platform={platform}
              onSelectNav={setActiveNavId}
              onSelectConversation={(id) => {
                void handleSelectConversation(id);
              }}
              onNewTask={() => {
                unsubscribeRef.current?.();
                unsubscribeRef.current = null;
                threadLoadSeqRef.current += 1;
                setComposerValue("");
                setNotice(null);
                setResolvingApprovalId(null);
                setResolvedApproval(null);
                resolvingApprovalIdRef.current = null;
                terminalSeenRef.current = false;
                localStorage.removeItem("awaken:runId");
                dispatch({ type: "run/reset" });
              }}
            />
          }
          main={
            <div className="desktop-main">
              <TopBar>
                <div className="runtime-pills">
                  <span className="runtime-pill">{runtimeClient.source}</span>
                  <span className="runtime-pill runtime-pill-status">
                    {formatStatusLabel(runtimeState.phase, runtimeState.status)}
                  </span>
                  <span className="runtime-pill runtime-pill-muted">
                    {runtimeClient.runtimeBaseUrl}
                  </span>
                  <span
                    className={`runtime-pill runtime-pill-status ${
                      runtimeInfo ? "runtime-pill-connected" : "runtime-pill-disconnected"
                    }`}
                  >
                    {runtimeInfo ? "Connected" : "Disconnected"}
                  </span>
                  {runtimeInfo ? (
                    <span className="runtime-pill">
                      {runtimeInfo.configuredModel || "unknown model"}
                    </span>
                  ) : null}
                  <label className="workspace-picker">
                    <span className="workspace-picker-label">Workspace</span>
                    <div className="workspace-picker-row">
                      <input
                        type="text"
                        className="workspace-picker-input"
                        value={workspacePath}
                        onChange={(e) => setWorkspacePath(e.target.value)}
                        placeholder="/path/to/project"
                        disabled={isBusy}
                      />
                      <button
                        type="button"
                        className="runtime-action"
                        onClick={() => {
                          void handlePickWorkspace();
                        }}
                        disabled={isBusy}
                      >
                        Choose…
                      </button>
                    </div>
                  </label>
                  {!isWorkspaceValid ? (
                    <span className="runtime-pill runtime-pill-warning">Workspace required</span>
                  ) : null}
                  <span className="runtime-pill">
                    approvals {runtimeState.approvals.length}
                  </span>
                  <span className="runtime-pill">
                    artifacts {runtimeState.artifacts.length}
                  </span>
                  <button
                    type="button"
                    className="runtime-action"
                    onClick={() => {
                      void handleCancel();
                    }}
                    disabled={!runtimeState.runId || !isBusy}
                  >
                    Cancel run
                  </button>
                </div>
              </TopBar>

              <CenterStage width={860} topMargin={72}>
                {connectionError ? (
                  <div className="runtime-panel runtime-panel-error">
                    <strong>Runtime disconnected</strong>
                    <p>
                      Unable to reach runtime at {runtimeClient.runtimeBaseUrl}. Please start
                      awaken-core.
                    </p>
                  </div>
                ) : null}

                <div className="hero-block">
                  <p className="eyebrow">AG-UI Phase 4</p>
                  <h1>Desktop runtime approvals and artifacts</h1>
                  <p className="lede">
                    Renderer 现在会消费 approval/artifact extension events，调用 runtime
                    resolve API，并把真实 runtime state 映射到 `ApprovalCard` 和
                    `ArtifactGrid`。
                  </p>
                </div>

                <Composer
                  value={composerValue}
                  onChange={setComposerValue}
                  onSubmit={() => {
                    void handleSubmit();
                  }}
                  onAddContext={handleAddContext}
                  mode={mode}
                  onModeChange={setMode}
                  model={selectedModel}
                  models={models}
                  onModelChange={setSelectedModel}
                  placeholder="Describe the change you want Awaken to run"
                  disabled={isBusy || !isWorkspaceValid}
                  isReady={composerValue.trim().length > 0 && isWorkspaceValid}
                />

                <div className="panel-stack">
                  <RunTimeline
                    title={runtimeState.timelineTitle}
                    status={runtimeState.timelineStatus}
                    steps={runtimeState.steps}
                  >
                    {runtimeState.errorMessage ? (
                      <div className="runtime-panel runtime-panel-error">
                        <strong>Runtime error</strong>
                        <p>{runtimeState.errorMessage}</p>
                      </div>
                    ) : null}

                    {notice ? (
                      <div className="runtime-panel">
                        <strong>Notice</strong>
                        <p>{notice}</p>
                      </div>
                    ) : null}

                    <div className="runtime-columns">
                      <section className="runtime-panel">
                        <div className="panel-heading">
                          <h2>Assistant stream</h2>
                          <span>{runtimeState.assistantText ? "live" : "empty"}</span>
                        </div>
                        <pre>{runtimeState.assistantText || "Waiting for TEXT_MESSAGE_* events"}</pre>
                      </section>

                      <section className="runtime-panel">
                        <div className="panel-heading">
                          <h2>Reasoning stream</h2>
                          <span>{runtimeState.reasoningText ? "live" : "empty"}</span>
                        </div>
                        <pre>{runtimeState.reasoningText || "Waiting for REASONING_* events"}</pre>
                      </section>
                    </div>

                    <section className="runtime-panel">
                      <div className="panel-heading">
                        <h2>Approvals</h2>
                        <span>{runtimeState.approvals.length}</span>
                      </div>
                      {activeApproval ? (
                        <p className="placeholder-copy">
                          Pending decision: {activeApproval.title}
                          {resolvingApprovalId === activeApproval.id ? " (resolving...)" : ""}
                        </p>
                      ) : (
                        <p className="placeholder-copy">
                          No pending approval requests.
                        </p>
                      )}
                    </section>

                    <section className="runtime-panel">
                      <div className="panel-heading">
                        <h2>Tool results</h2>
                        <span>{runtimeState.toolResults.length}</span>
                      </div>
                      {runtimeState.toolResults.length === 0 ? (
                        <p className="placeholder-copy">
                          Waiting for `TOOL_CALL_RESULT` events.
                        </p>
                      ) : (
                        <div className="tool-result-list">
                          {runtimeState.toolResults.map((tool) => (
                            <article key={tool.id} className="tool-result-card">
                              <h3>{tool.title}</h3>
                              <pre>{tool.content}</pre>
                            </article>
                          ))}
                        </div>
                      )}
                    </section>

                    <section className="runtime-panel">
                      <div className="panel-heading">
                        <h2>Artifacts</h2>
                        <span>{runtimeState.artifacts.length}</span>
                      </div>
                      <ArtifactGrid
                        items={runtimeState.artifacts.map((artifact) => ({
                          id: artifact.id,
                          title: artifact.title,
                          description: artifact.description,
                          changeType:
                            artifact.kind === "added" ||
                            artifact.kind === "modified" ||
                            artifact.kind === "deleted" ||
                            artifact.kind === "moved"
                              ? artifact.kind
                              : undefined,
                          path: artifact.path,
                        }))}
                      />
                      {latestArtifact && latestArtifact.kind !== "a2ui_surface" ? (
                        <div style={{ marginTop: "12px" }}>
                          <strong>{latestArtifact.title}</strong>
                          <pre>
                            {[
                              latestArtifact.path ? `path: ${latestArtifact.path}` : "",
                              `kind: ${latestArtifact.kind}`,
                              `status: ${latestArtifact.status}`,
                              latestArtifact.content ? "" : "No artifact content provided.",
                              latestArtifact.content || "",
                            ]
                              .filter(Boolean)
                              .join("\n")}
                          </pre>
                        </div>
                      ) : null}
                      {a2uiArtifacts.length > 0 ? (
                        <div className="a2ui-stack">
                          {a2uiArtifacts.map((artifact) => (
                            <A2UISurfaceView
                              key={artifact.id}
                              title={artifact.title}
                              content={artifact.content}
                            />
                          ))}
                        </div>
                      ) : null}
                    </section>
                  </RunTimeline>
                </div>
              </CenterStage>
            </div>
          }
        />
      </div>

      <ApprovalCard
        open={Boolean(activeApproval)}
        onOpenChange={() => {}}
        title={activeApproval?.title || "Approval required"}
        description={activeApproval?.description}
        affectedPaths={activeApproval?.affectedPaths}
        patchSummary={activeApproval?.patchSummary}
        riskTone={activeApproval?.riskTone}
        approveLabel={resolvingApprovalId === activeApproval?.id ? "Resolving..." : "Approve"}
        rejectLabel={resolvingApprovalId === activeApproval?.id ? "Resolving..." : "Reject"}
        resolving={resolvingApprovalId === activeApproval?.id}
        resolvedDecision={
          resolvedApproval?.approvalId === activeApproval?.id
            ? resolvedApproval.decision
            : undefined
        }
        onApprove={() => {
          void handleResolveApproval("approved");
        }}
        onReject={() => {
          void handleResolveApproval("rejected");
        }}
      />
    </>
  );
}
