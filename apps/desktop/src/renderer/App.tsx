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

const navItems: NavItem[] = [
  { id: "runs", label: "Runs" },
  { id: "artifacts", label: "Artifacts", disabled: true },
  { id: "approvals", label: "Approvals", disabled: true },
];

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
        id: "desktop-phase-3",
        label: "Desktop Phase 3",
        timeLabel: "stream",
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
  const [showApprovalPlaceholder, setShowApprovalPlaceholder] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

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
          id: "phase-3-placeholder",
          label: "Phase 3 desktop stream consumer",
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
    dispatch({ type: "run/requested", prompt });

    try {
      const run = await runtimeClient.createRun(prompt);
      dispatch({ type: "run/created", run });
      const unsubscribe = await runtimeClient.subscribeRun(run.runId, {
        onEvent: (event) => {
          if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
            terminalSeenRef.current = true;
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

  function handleAddContext(): void {
    setNotice("Phase 3 只接 runtime stream；context / approvals / artifacts 在后续阶段接入。");
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
              activeProjectId="desktop-phase-3"
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
                  <button
                    type="button"
                    className="runtime-action"
                    onClick={() => setShowApprovalPlaceholder(true)}
                  >
                    Approval placeholder
                  </button>
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
                  <p className="eyebrow">AG-UI Phase 3</p>
                  <h1>Desktop runtime stream consumer</h1>
                  <p className="lede">
                    Renderer 通过 adapter 创建 run、订阅 Phase 2 SSE、取消 run，再把 AG-UI
                    事件 reduce 成 view state 喂给 `@awaken/ui`。
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
                        <span>Phase 4</span>
                      </div>
                      <ArtifactGrid items={runtimeState.artifactItems} />
                    </section>
                  </RunTimeline>
                </div>
              </CenterStage>
            </div>
          }
        />
      </div>

      <ApprovalCard
        open={showApprovalPlaceholder}
        onOpenChange={setShowApprovalPlaceholder}
        title="Approval event placeholder"
        description="Phase 3 不接 runtime approval 事件；这里只保留 Electron + UI 对接位置。"
        riskTone="warning"
        approveLabel="关闭占位"
        rejectLabel="稍后"
      />
    </>
  );
}
