package evalharness

func BuiltinAcceptanceProfile() Profile {
	realProvider := Requirements{
		RealProvider: &RealProviderRequirement{
			Required: true,
			Provider: "configured-runtime-provider",
			Model:    "configured-runtime-model",
			EnvVars:  []string{"LLM_API_KEY"},
			Reason:   "Awaken runtime acceptance must exercise the real provider path",
		},
	}

	return Profile{
		ID:          "awaken-runtime-acceptance",
		Name:        "Awaken Runtime Acceptance",
		Description: "Data-driven smoke scenarios for runtime, agent loop, tools, approval, and SSE evidence capture.",
		Scenarios: []Scenario{
			{
				ID:          "runtime-health",
				Name:        "Runtime Health",
				Description: "Verify awaken-core health and runtime info without exposing provider secrets.",
				Tags:        []string{"runtime", "smoke"},
				Steps: []Step{
					{
						ID:          "health-and-info",
						Name:        "Health and runtime info",
						Command:     "GET /health && GET /v1/runtime/info",
						Description: "Capture local runtime health and redacted model/provider readiness metadata.",
						Tags:        []string{"runtime"},
					},
				},
			},
			{
				ID:           "agent-loop-readonly",
				Name:         "Agent Loop Read-only Run",
				Description:  "Start a real-provider run and verify the agent loop reaches a terminal result without mutating files.",
				Tags:         []string{"agent_loop", "real_provider"},
				Requirements: realProvider,
				Steps: []Step{
					{
						ID:          "create-readonly-run",
						Name:        "Create read-only run",
						Command:     `POST /v1/runs {"input":"Answer a one-line sanity question without changing files."}`,
						Description: "Create a run through the local runtime API using the configured provider.",
						Tags:        []string{"runtime", "agent_loop"},
					},
					{
						ID:          "observe-terminal-events",
						Name:        "Observe terminal SSE events",
						Command:     "GET /v1/runs/{run_id}/events",
						Description: "Capture RUN_STARTED, text deltas, and RUN_FINISHED or RUN_ERROR as evidence.",
						Tags:        []string{"sse", "agent_loop"},
					},
				},
			},
			{
				ID:           "tool-execution-readonly",
				Name:         "Read-only Tool Execution",
				Description:  "Exercise tool selection and execution with a prompt that should only inspect workspace files.",
				Tags:         []string{"tools", "agent_loop", "real_provider"},
				Requirements: realProvider,
				Steps: []Step{
					{
						ID:          "run-readonly-tool-prompt",
						Name:        "Run read-only tool prompt",
						Command:     `POST /v1/runs {"input":"List the workspace files and read README.md. Do not modify files."}`,
						Description: "Verify provider-driven tool use can complete on read-only workspace inspection.",
						Tags:        []string{"tools"},
					},
					{
						ID:          "capture-tool-events",
						Name:        "Capture tool events",
						Command:     "GET /v1/runs/{run_id}/events?after=0",
						Description: "Capture TOOL_CALL_* and TOOL_CALL_RESULT events as acceptance evidence.",
						Tags:        []string{"tools", "sse"},
					},
				},
			},
			{
				ID:           "approval-and-sse-replay",
				Name:         "Approval and SSE Replay",
				Description:  "Exercise a mutation approval request and replay event history after a known sequence.",
				Tags:         []string{"approval", "sse", "real_provider"},
				Requirements: realProvider,
				Steps: []Step{
					{
						ID:          "run-approval-prompt",
						Name:        "Run approval prompt",
						Command:     `POST /v1/runs {"input":"Propose a small file mutation that requires approval, then wait for the decision."}`,
						Description: "Verify mutation attempts surface an approval request before tool execution.",
						Tags:        []string{"approval", "tools"},
					},
					{
						ID:          "resolve-approval",
						Name:        "Resolve approval",
						Command:     "POST /v1/runs/{run_id}/approvals/{approval_id}/resolve",
						Description: "Record approval resolution and the resulting run status.",
						Tags:        []string{"approval"},
					},
					{
						ID:          "replay-after-seq",
						Name:        "Replay after sequence",
						Command:     "GET /v1/runs/{run_id}/events?after={seq}",
						Description: "Verify SSE replay returns only events after the requested sequence.",
						Tags:        []string{"sse"},
					},
				},
			},
		},
	}
}
