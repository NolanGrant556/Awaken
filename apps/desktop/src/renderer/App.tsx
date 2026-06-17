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
import { createRuntimeClient } from "./runtime/client";
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
): string {
  switch (phase) {
    case "starting":
      return "Creating run";
    case "running":
      return "Streaming";
    case "cancelling":
      return "Cancelling";
    case "done":
      return "Completed";
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

  useEffect(() => {
    return () => {
      unsubscribeRef.current?.();
      unsubscribeRef.current = null;
    };
  }, []);

  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "cancelling";

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

  const conversations: ConversationItem[] = runtimeState.prompt
    ? [
        {
          id: runtimeState.runId || "latest-run",
          label: runtimeState.prompt,
          timeLabel: runtimeState.phase === "done" ? "done" : "now",
        },
      ]
    : [
        {
          id: "phase-4-placeholder",
          label: "Phase 4 runtime approvals and artifacts",
          timeLabel: "ready",
        },
      ];

  async function handleSubmit(): Promise<void> {
    const prompt = composerValue.trim();
    if (!prompt || isBusy) {
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    resolvingApprovalIdRef.current = null;
    dispatch({ type: "run/requested", prompt });

    try {
      const run = await runtimeClient.createRun(prompt);
      dispatch({ type: "run/created", run });
      const unsubscribe = await runtimeClient.subscribeRun(run.runId, {
        onEvent: (event) => {
          if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
            terminalSeenRef.current = true;
          }
          if (
            event.type === "AWAKEN_APPROVAL_RESOLVED" &&
            event.approvalId &&
            event.approvalId === resolvingApprovalIdRef.current
          ) {
            resolvingApprovalIdRef.current = null;
            setResolvingApprovalId(null);
          }
          dispatch({ type: "run/event", event });
        },
        onError: (message) => dispatch({ type: "run/streamError", message }),
        onClose: () => {
          if (!terminalSeenRef.current) {
            dispatch({
              type: "run/streamError",
              message: "Runtime stream closed before a terminal event. Please retry the run.",
            });
          }
          unsubscribeRef.current = null;
        },
      });
      unsubscribeRef.current = unsubscribe;
      setComposerValue("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create run";
      dispatch({ type: "run/createFailed", message });
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
    } catch (error) {
      dispatch({
        type: "run/streamError",
        message: error instanceof Error ? error.message : "Failed to cancel run",
      });
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

    try {
      await runtimeClient.resolveApproval(runtimeState.runId, activeApproval.id, decision);
    } catch (error) {
      setResolvingApprovalId(null);
      resolvingApprovalIdRef.current = null;
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
              activeConversationId={conversations[0]?.id}
              projects={projects}
              activeProjectId="desktop-phase-4"
              defaultExpandedIds={["awaken"]}
              profileName="Desktop Consumer"
              profileAvatarFallback="AW"
              platform={platform}
              onSelectNav={setActiveNavId}
              onNewTask={() => {
                unsubscribeRef.current?.();
                unsubscribeRef.current = null;
                setComposerValue("");
                setNotice(null);
                setResolvingApprovalId(null);
                resolvingApprovalIdRef.current = null;
                terminalSeenRef.current = false;
                dispatch({ type: "run/reset" });
              }}
            />
          }
          main={
            <div className="desktop-main">
              <TopBar>
                <div className="runtime-pills">
                  <span className="runtime-pill">{runtimeClient.source}</span>
                  <span className="runtime-pill runtime-pill-muted">
                    {runtimeClient.runtimeBaseUrl}
                  </span>
                  <span className="runtime-pill runtime-pill-status">
                    {formatStatusLabel(runtimeState.phase)}
                  </span>
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
                  disabled={isBusy}
                  isReady={composerValue.trim().length > 0}
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
                          description: [
                            artifact.description,
                            [artifact.kind, artifact.status].filter(Boolean).join(" / "),
                            artifact.path,
                          ]
                            .filter(Boolean)
                            .join(" | "),
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
        riskTone={activeApproval?.riskTone}
        approveLabel={resolvingApprovalId === activeApproval?.id ? "Resolving..." : "Approve"}
        rejectLabel={resolvingApprovalId === activeApproval?.id ? "Resolving..." : "Reject"}
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
