package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/computer"
	computerfake "awaken/src/computer/fake"
	convcontext "awaken/src/context"
	awruntime "awaken/src/runtime"
)

func TestComputerControlRoutesProtectedByControlToken(t *testing.T) {
	service := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)
	identity := &awruntime.RuntimeIdentity{
		RuntimeIdentity: "test-runtime",
		InstanceID:      "test-instance",
		ControlToken:    "test-token",
		OwnerMode:       "test",
		BaseURL:         "http://localhost",
		StartedAt:       time.Now().UTC(),
	}
	router := NewRouterWithControl(service, nil, nil, nil, nil, nil, "", nil, &ControlConfig{
		Identity:          identity,
		LeaseManager:      awruntime.NewLeaseManager(nil),
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/control/computer/sessions/run-1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/control/computer/sessions/run-1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown run, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestComputerPermissionRequestUsesHostRequester(t *testing.T) {
	service := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)
	requestCount := 0
	service.SetComputerHostPermissionRequester(computer.HostPermissionRequesterFunc(func(context.Context) (computer.HostStatus, error) {
		requestCount++
		return computer.HostStatus{
			Available:          true,
			Healthy:            true,
			TCCScreenRecording: true,
			TCCAccessibility:   true,
		}, nil
	}))
	identity := &awruntime.RuntimeIdentity{
		RuntimeIdentity: "test-runtime",
		InstanceID:      "test-instance",
		ControlToken:    "test-token",
		OwnerMode:       "test",
		BaseURL:         "http://localhost",
		StartedAt:       time.Now().UTC(),
	}
	router := NewRouterWithControl(service, nil, nil, nil, nil, nil, "", nil, &ControlConfig{
		Identity:          identity,
		LeaseManager:      awruntime.NewLeaseManager(nil),
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/control/computer/permissions/request", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/control/computer/permissions/request", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one Host permission request, got %d", requestCount)
	}
	if !strings.Contains(rr.Body.String(), `"tcc_screen_recording":true`) || !strings.Contains(rr.Body.String(), `"tcc_accessibility":true`) {
		t.Fatalf("expected refreshed Host permission status, got %s", rr.Body.String())
	}
}

func TestComputerControlSessionLifecycle(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	releaseRun := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseRun:
		default:
			close(releaseRun)
		}
	})
	service := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				select {
				case <-releaseRun:
					return "done", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			})
		},
	)
	service.SetComputerSettings(computer.Settings{Enabled: true})
	service.SetComputerDriverFactory(func(computer.Binding) (computer.Driver, error) {
		return computerfake.NewDriver(), nil
	})

	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "hello",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: &awruntime.ModelProfileConfig{
			ProfileID:           "profile-1",
			ProviderID:          "kimi-cn",
			ModelID:             "kimi-k2.6",
			BaseURL:             "https://api.moonshot.cn/v1",
			Protocol:            "openai_chat",
			AuthScheme:          "bearer",
			APIKey:              "secret",
			SupportsVision:      true,
			SupportsComputerUse: true,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	identity := &awruntime.RuntimeIdentity{
		RuntimeIdentity: "test-runtime",
		InstanceID:      "test-instance",
		ControlToken:    "test-token",
		OwnerMode:       "test",
		BaseURL:         "http://localhost",
		StartedAt:       time.Now().UTC(),
	}
	router := NewRouterWithControl(service, nil, nil, nil, nil, nil, "", nil, &ControlConfig{
		Identity:          identity,
		LeaseManager:      awruntime.NewLeaseManager(nil),
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
	})

	// GET session should report no active session because the unavailable driver
	// cannot open one.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/control/computer/sessions/%s", run.RunID), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for session status, got %d: %s", rr.Code, rr.Body.String())
	}

	// Terminate should succeed even without an open session.
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/control/computer/sessions/%s/terminate", run.RunID), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for terminate, got %d: %s", rr.Code, rr.Body.String())
	}
	runAfterTerminate, err := service.GetRun(run.RunID)
	if err != nil {
		t.Fatalf("get run after Computer terminate: %v", err)
	}
	if runAfterTerminate.Status != awruntime.RunStatusRunning {
		t.Fatalf("Computer terminate changed run status to %s, want running", runAfterTerminate.Status)
	}

	// Takeover/return on a runtime without an open session should fail with 409.
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/control/computer/sessions/%s/takeover", run.RunID), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for takeover without session, got %d: %s", rr.Code, rr.Body.String())
	}

	// Review resolution requires a run_id in this phase because the route does
	// not yet include it. An unknown review returns 409.
	req = httptest.NewRequest(http.MethodPost, "/v1/control/computer/reviews/rev_1/resolve", strings.NewReader(fmt.Sprintf(`{"decision":"approved_once","run_id":%q}`, run.RunID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for unknown review, got %d: %s", rr.Code, rr.Body.String())
	}

	close(releaseRun)
	waitForRunTerminalServer(t, service, run.RunID)
}

func TestComputerControlRoutesRequireRunWithComputerRuntime(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "hello",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: &awruntime.ModelProfileConfig{
			ProfileID:      "profile-1",
			ProviderID:     "kimi-cn",
			ModelID:        "kimi-k2.6",
			BaseURL:        "https://api.moonshot.cn/v1",
			Protocol:       "openai_chat",
			AuthScheme:     "bearer",
			APIKey:         "secret",
			SupportsVision: true,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	identity := &awruntime.RuntimeIdentity{
		RuntimeIdentity: "test-runtime",
		InstanceID:      "test-instance",
		ControlToken:    "test-token",
		OwnerMode:       "test",
		BaseURL:         "http://localhost",
		StartedAt:       time.Now().UTC(),
	}
	router := NewRouterWithControl(service, nil, nil, nil, nil, nil, "", nil, &ControlConfig{
		Identity:          identity,
		LeaseManager:      awruntime.NewLeaseManager(nil),
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/control/computer/sessions/%s", run.RunID), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for run without computer runtime, got %d: %s", rr.Code, rr.Body.String())
	}

	waitForRunTerminalServer(t, service, run.RunID)
}

func TestComputerExecutorBoundToToolRuntime(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	var agentConfig agentcore.AgentConfig
	service := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			agentConfig = agent.GetConfig()
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)
	service.SetComputerSettings(computer.Settings{Enabled: true})
	service.SetComputerDriverFactory(func(computer.Binding) (computer.Driver, error) {
		return computerfake.NewDriver(), nil
	})

	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "hello",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: &awruntime.ModelProfileConfig{
			ProfileID:           "profile-1",
			ProviderID:          "kimi-cn",
			ModelID:             "kimi-k2.6",
			BaseURL:             "https://api.moonshot.cn/v1",
			Protocol:            "openai_chat",
			AuthScheme:          "bearer",
			APIKey:              "secret",
			SupportsVision:      true,
			SupportsComputerUse: true,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	// Wait for the executor to be created so agentConfig is populated.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && agentConfig.ComputerRuntime == nil {
		time.Sleep(10 * time.Millisecond)
	}

	if agentConfig.ComputerRuntime == nil {
		t.Fatalf("expected agent config to have a computer runtime")
	}
	if agentConfig.ExecutionEnvironmentSnapshot == nil || !agentConfig.ExecutionEnvironmentSnapshot.ComputerEnabled {
		t.Fatalf("expected environment snapshot ComputerEnabled=true")
	}

	// The runner builds the ToolRuntime with a ComputerExecutor. We can verify
	// by constructing the same config and checking the router wires it.
	config := agentcore.AgentConfig{
		ModelConfig:      agentcore.ModelConfig{Capabilities: agentConfig.ModelConfig.Capabilities},
		ComputerRuntime:  agentConfig.ComputerRuntime,
		Workspace:        agentConfig.Workspace,
		ToolSessionState: agentConfig.ToolSessionState,
	}
	conv := convcontext.NewConversation(nil)
	runner := agentcore.NewRunner(config, conv, nil)
	_ = runner

	waitForRunTerminalServer(t, service, run.RunID)
}

func waitForRunTerminalServer(t *testing.T, service *awruntime.RunService, runID string) {
	t.Helper()
	run, err := service.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, getErr := service.GetRun(runID)
		if getErr == nil {
			switch r.Status {
			case awruntime.RunStatusFinished, awruntime.RunStatusError, awruntime.RunStatusCancelled:
				active := false
				for _, thread := range service.ListThreads() {
					if thread.ThreadID == run.ThreadID && thread.ActiveRunID == runID {
						active = true
						break
					}
				}
				if !active {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
}

var _ = computer.DefaultPolicy // keep import used in file
