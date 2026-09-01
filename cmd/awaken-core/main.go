package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"awaken/src/acp"
	"awaken/src/acp/protocol"
	"awaken/src/agentcore"
	"awaken/src/computer"
	"awaken/src/computer/cua"
	"awaken/src/connectorauth"
	"awaken/src/connectors"
	"awaken/src/desktopdata"
	"awaken/src/filestore"
	"awaken/src/hookruntime"
	"awaken/src/instructions"
	"awaken/src/llm"
	"awaken/src/mcpclient"
	"awaken/src/memory"
	"awaken/src/modelprofile"
	"awaken/src/networkroute"
	"awaken/src/outlook"
	"awaken/src/pluginauth"
	"awaken/src/pluginrules"
	"awaken/src/plugins"
	"awaken/src/runtime"
	"awaken/src/scheduledtask"
	"awaken/src/server"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools"
	bashtool "awaken/src/tools/bash"
	configureacprunner "awaken/src/tools/configure_acp_runner"
	configurehook "awaken/src/tools/configure_hook"
	delegateexternalagent "awaken/src/tools/delegate_external_agent"
	mediagenerate "awaken/src/tools/media_generate"
	plugintool "awaken/src/tools/plugin"
)

// microsoftOutlookClientID is a public OAuth client identifier. Release builds
// may set it with -X main.microsoftOutlookClientID=...; development can use
// AWAKEN_MICROSOFT_CLIENT_ID without rebuilding Core.
var microsoftOutlookClientID string

func main() {
	ownerMode := flag.String("owner-mode", "desktop", "Core owner mode: desktop or headless")
	flag.Parse()
	routeManager := networkroute.NewFromEnvironment(os.Environ())
	networkroute.InstallDefaultHTTPTransport(routeManager)

	addr := os.Getenv("AWAKEN_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}

	userDataDir := strings.TrimSpace(os.Getenv("AWAKEN_USER_DATA_DIR"))
	var runtimeIdentity *runtime.RuntimeIdentity
	if userDataDir != "" && (*ownerMode == "desktop" || *ownerMode == "headless") {
		if err := os.MkdirAll(userDataDir, 0o700); err != nil {
			log.Fatalf("ensure userData dir: %v", err)
		}
	} else if *ownerMode == "headless" {
		log.Fatalf("headless mode requires AWAKEN_USER_DATA_DIR")
	}

	defaultWorkspace := resolveDefaultWorkspace()
	infoProvider := server.NewLLMRuntimeInfoProvider("", "", "", defaultWorkspace, "")
	legacyCLIReport, err := connectors.MigrateLegacyBuiltinCLIConfigs()
	if err != nil {
		log.Printf("[connectors] legacy CLI MCP migration failed: %v", err)
	} else if len(legacyCLIReport.Removed) > 0 || len(legacyCLIReport.ReviewRequired) > 0 {
		log.Printf("[connectors] legacy CLI MCP migration removed=%v review_required=%v", legacyCLIReport.Removed, legacyCLIReport.ReviewRequired)
	}
	if changed, prepareErr := connectors.PreparePlaywrightLogRouting(); prepareErr != nil {
		log.Printf("[connectors] Playwright MCP log routing unavailable: %v", prepareErr)
	} else if changed {
		log.Printf("[connectors] Playwright MCP log routing prepared under AWAKEN_HOME")
	}

	pluginStore, err := plugins.NewStore()
	if err != nil {
		log.Printf("[plugins] store unavailable: %v", err)
	} else if avail := pluginStore.Available(); avail != nil {
		log.Printf("[plugins] store fail closed: %v", avail)
	} else {
		if builtinRoot := strings.TrimSpace(os.Getenv(plugins.EnvBuiltinPluginDir)); builtinRoot != "" {
			synced, syncErr := pluginStore.SyncBuiltinPlugins(context.Background(), builtinRoot)
			if syncErr != nil {
				log.Printf("[plugins] built-in plugin sync incomplete: %v", syncErr)
			}
			log.Printf("[plugins] synchronized %d built-in plugin(s)", len(synced))
		}
		skills.SetPluginSkillSelections(pluginStore.EnabledSkillSelections())
		subagentruntime.SetRoleResolver(pluginStore.ResolvePluginAgentRole)
	}
	plugintool.SetStore(pluginStore)
	credentialStore := mcpclient.NewPersistentCredentialStore()
	inProcRegistry := mcpclient.NewInProcRegistry()
	outlookRuntime := outlook.NewRuntime(nil)
	if err := inProcRegistry.Register(outlook.ServerName, outlookRuntime.Factory()); err != nil {
		log.Fatalf("[outlook] register in-process MCP runtime: %v", err)
	}
	manager := buildMCPManager(defaultWorkspace, pluginStore, inProcRegistry, credentialStore)
	connRegistry := buildConnectorRegistry(manager, defaultWorkspace, legacyCLIReport.ReviewRequired)
	connectorAuth := connectorauth.NewService(connRegistry, credentialStore)
	if connRegistry != nil {
		connRegistry.SetAccountReadinessProvider(connectorAuth)
	}
	outlookRuntime.SetTokenProvider(connectorAuth)
	outlookRuntime.SetNeedsAuthCallback(manager.MarkServerNeedsAuth)
	if err := outlookRuntime.Start(); err != nil {
		log.Printf("[outlook] scheduled mail unavailable: %v", err)
	}
	defer outlookRuntime.Close()
	pluginAuth := pluginauth.NewService(pluginStore, manager, connRegistry, credentialStore)
	pluginAuth.SetHostConnectorAuth(connectorAuth)
	manager.SetRuntimeSecretResolver(pluginAuth)
	fileHookSecrets := hookruntime.NewFileSecretStore(desktopdata.AwakenHomeRoot())
	hookSecrets := hookruntime.SecretResolverFunc(func(ctx context.Context, ref string) (string, error) {
		if strings.HasPrefix(ref, "plugin-auth:") {
			return pluginAuth.ResolveHookSecret(ctx, ref)
		}
		return fileHookSecrets.ResolveHookSecret(ctx, ref)
	})
	skillRegistry := buildSkillRegistry(manager, defaultWorkspace)
	if pluginStore != nil {
		pluginStore.SetOnChange(func() {
			if pluginStore.Available() != nil {
				skills.SetPluginSkillSelections(nil)
				return
			}
			skills.SetPluginSkillSelections(pluginStore.EnabledSkillSelections())
			applyPluginMCPServers(manager, pluginStore)
		})
		applyPluginMCPServers(manager, pluginStore)
	}
	syncSkillsForWorkspace := func(workspace runtime.WorkspaceConfig) {
		skillRegistry.ReloadLocalSources(skillSourceDirsForWorkspace(workspace))
	}
	skillManager := skillruntime.NewManager(skillRegistry, skillruntime.StdLogger{})
	skillManager.RefreshTools()

	acpManager := buildACPManager(defaultWorkspace)
	delegateexternalagent.SetManager(acpManager)
	configureacprunner.SetRegistry(acpManager.Registry())

	var runService *runtime.RunService
	runService = runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			modelCfg := modelConfigFromProfile(profile)
			workspaceRoot := workspace.WorkspaceRoot
			if workspaceRoot == "" {
				workspaceRoot = defaultWorkspace
			}
			recallManager, recallErr := memory.NewRecallManager(workspaceRoot, llm.Config{
				APIKey: modelCfg.APIKey, BaseURL: modelCfg.BaseURL, InferenceEndpoint: modelCfg.InferenceEndpoint, Model: modelCfg.Model,
				Protocol: llm.Protocol(modelCfg.Protocol), Temperature: modelCfg.Temperature,
				ProviderID: modelCfg.ProviderID, ProfileID: modelCfg.ProfileID, CredentialRevision: modelCfg.CredentialRevision,
				MaxTokens: modelCfg.MaxTokens, AuthScheme: modelCfg.AuthScheme, ContextWindow: modelCfg.ContextWindow,
			})
			if recallErr != nil {
				log.Printf("[memory] automatic recall disabled: %v", recallErr)
			}
			agentCfg := agentcore.AgentConfig{
				MaxThoughts:           0,
				EntryPoint:            "awaken-core",
				ModelConfig:           modelCfg,
				CompactModelConfig:    purposeModelConfig(profile, "compact_summary"),
				SubagentModelConfig:   purposeModelConfig(profile, "subagent"),
				HookPromptModelConfig: purposeModelConfig(profile, "hook_prompt"),
				HookAgentModelConfig:  purposeModelConfig(profile, "hook_agent"),
				SkillSummary: skills.FormatRuntimeSkillSummary(
					skillRegistry.PreviewLocalSources(skillSourceDirsForWorkspace(workspace)),
				),
				MemoryContextProvider: func() (string, error) {
					if recallManager != nil {
						return recallManager.Context()
					}
					return memory.BuildIndexContext(workspaceRoot)
				},
				ArchivedHistoryService: runService,
				MemoryRecallStart: func(ctx context.Context, query string, recentTools []string) {
					if recallManager != nil {
						recallManager.StartTurnWithTools(ctx, query, recentTools)
					}
				},
				MemoryRecallSetAuthority: func(authority agentcore.ModelCallAuthority) {
					if recallManager != nil {
						recallManager.SetModelInvoker(func(ctx context.Context, cfg llm.Config, req llm.Request, retryIndex int, auxiliaryFallback bool) (*llm.Response, error) {
							if strings.TrimSpace(req.AffinityScopeID) == "" {
								req.AffinityScopeID = authority.ThreadID
							}
							return agentcore.InvokeRecordedModel(ctx, authority, cfg, req, retryIndex, auxiliaryFallback)
						})
					}
				},
				MemoryRecallEnd: func() {
					if recallManager != nil {
						recallManager.EndTurn()
					}
				},
				MemoryRecallDrain: func() []agentcore.MemoryRecallReceipt {
					if recallManager == nil {
						return nil
					}
					receipts := recallManager.DrainPromotedReceipts()
					result := make([]agentcore.MemoryRecallReceipt, 0, len(receipts))
					for _, receipt := range receipts {
						result = append(result, agentcore.MemoryRecallReceipt{MemoryID: receipt.MemoryID, Scope: string(receipt.Scope), Type: string(receipt.Type), Revision: receipt.Revision, Bytes: receipt.Bytes})
					}
					return result
				},
				MemoryRecallReset: func() {
					if recallManager != nil {
						recallManager.ResetAfterCompaction()
					}
				},
				MemoryRecallMarkCompacted: func(memoryIDs []string) {
					if recallManager != nil {
						recallManager.MarkCompacted(memoryIDs)
					}
				},
				CaptureMemoryRecallSnapshot: func() *agentcore.MemoryRecallSnapshot {
					if recallManager == nil {
						return nil
					}
					snapshot := recallManager.Snapshot(workspaceRoot)
					if len(snapshot.Items) == 0 {
						return nil
					}
					items := make([]agentcore.MemoryRecallSnapshotItem, 0, len(snapshot.Items))
					for _, item := range snapshot.Items {
						items = append(items, agentcore.MemoryRecallSnapshotItem{
							MemoryID: item.MemoryID, Scope: item.Scope, Type: item.Type,
							Revision: item.Revision, Bytes: item.Bytes, Content: item.Content,
						})
					}
					return &agentcore.MemoryRecallSnapshot{WorkspaceRoot: snapshot.WorkspaceRoot, Items: items}
				},
				Workspace: agentcore.WorkspaceConfig{
					ScopeID:                 workspace.ScopeID,
					ProjectID:               workspace.ProjectID,
					ProjectName:             workspace.ProjectName,
					SessionWorkspace:        workspace.SessionWorkspace,
					Root:                    workspace.WorkspaceRoot,
					ReadableRoots:           workspace.ReadableRoots,
					WritableRoots:           workspace.WritableRoots,
					OutputRoots:             workspace.OutputRoots,
					AttachedFiles:           workspace.AttachedFiles,
					ProtectedRoots:          workspace.ProtectedRoots,
					ProtectedRootExceptions: workspace.ProtectedRootExceptions,
					TemporaryWorkRoot:       workspace.TemporaryWorkRoot,
				},
				ExecutionProfile:       workspace.ExecutionProfile,
				ModelContentStore:      runtime.NewToolResultStore(),
				ToolResultOffloadStore: runtime.NewToolResultStore(),
				ContextEpochStore:      runtime.NewContextEpochStore(),
				HookSecretResolver:     hookSecrets,
			}
			return agentcore.NewAgentWithThreadID(agentCfg, threadID)
		},
		func(agent *agentcore.Agent) runtime.Executor {
			return runtime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return agent.RunStreamWithEvents(ctx, input, sink)
			})
		},
	)
	manager.SetElicitationHandler(func(ctx context.Context, request mcpclient.ElicitationRequest) (mcpclient.ElicitationResult, error) {
		identity, ok := tool.ToolExecutionIdentityFromContext(ctx)
		if !ok || strings.TrimSpace(identity.RunID) == "" {
			return mcpclient.ElicitationResult{Action: "cancel"}, nil
		}
		mode := strings.TrimSpace(request.Mode)
		if mode == "" {
			mode = "form"
		}
		recoveries := []string{"submit_form", "cancel"}
		if mode == "url" {
			recoveries = []string{"open_url", "confirm_done", "cancel"}
		}
		answer, err := runService.AwaitExternalInteractionForm(ctx, identity.RunID, agentcore.AwakenEvent{
			Type: agentcore.ExternalInteractionRequestedType, Timestamp: time.Now().UTC(),
			InteractionID:   fmt.Sprintf("mcp_elicitation_%d", time.Now().UnixNano()),
			InteractionKind: "mcp-elicitation", InteractionTitle: "MCP 请求确认",
			InteractionSafeMessage: request.Message, InteractionVerificationURL: request.URL,
			InteractionProvider: request.ServerName, InteractionRecoveries: recoveries,
			InteractionFormSchema: request.RequestedSchema,
		})
		if err != nil {
			return mcpclient.ElicitationResult{}, err
		}
		switch answer.Resolution {
		case agentcore.ExternalInteractionCompleted:
			return mcpclient.ElicitationResult{Action: "accept", Content: answer.Content}, nil
		case agentcore.ExternalInteractionCancelled:
			return mcpclient.ElicitationResult{Action: "cancel"}, nil
		default:
			return mcpclient.ElicitationResult{Action: "decline"}, nil
		}
	})
	if hookStore, hookErr := hookruntime.NewStore(desktopdata.AwakenHomeRoot()); hookErr != nil {
		log.Printf("[hooks] configuration store unavailable: %v", hookErr)
	} else {
		runService.SetHookStore(hookStore)
		configurehook.SetStore(hookStore)
	}
	runService.SetSkillActivator(skillManager.Activator())
	if pluginStore != nil {
		runService.SetComposerPluginValidator(func(name, digest string) error {
			_, err := pluginStore.ResolveComposerSelection(name, digest)
			return err
		})
		runService.SetPluginRuleProvider(func(tokens []runtime.ComposerToken, workspace runtime.WorkspaceConfig) ([]instructions.ExtraSource, error) {
			var manual []pluginrules.Selection
			var filePaths []string
			for _, token := range tokens {
				switch token.Type {
				case "rule":
					selection := pluginrules.Selection{PluginName: strings.TrimSpace(token.Payload.PluginName), PluginDigest: strings.TrimSpace(token.Payload.PluginDigest), LogicalID: strings.TrimSpace(token.Payload.RuleLogicalID)}
					if selection.PluginName == "" || selection.PluginDigest == "" || selection.LogicalID == "" {
						return nil, runtime.ErrInvalidInput
					}
					if err := pluginStore.ResolveRuleSelection(selection.PluginName, selection.PluginDigest, selection.LogicalID); err != nil {
						return nil, err
					}
					manual = append(manual, selection)
				case "file":
					if token.Payload.RelativePath != "" {
						filePaths = append(filePaths, token.Payload.RelativePath)
					} else if token.Payload.Path != "" && workspace.WorkspaceRoot != "" {
						if relative, err := filepath.Rel(workspace.WorkspaceRoot, token.Payload.Path); err == nil {
							filePaths = append(filePaths, relative)
						}
					}
				}
			}
			return pluginStore.RuleInstructionSources(manual, filePaths), nil
		})
		runService.SetPluginCommandExpander(func(name, digest, logicalID string, arguments []string) (string, error) {
			expansion, err := pluginStore.ExpandCommand(name, digest, logicalID, arguments)
			if err != nil {
				return "", err
			}
			return expansion.Prompt, nil
		})
		runService.SetPluginHookProvider(func(workspace runtime.WorkspaceConfig) ([]hookruntime.CompiledHook, []hookruntime.SourceReceipt, []hookruntime.Diagnostic, error) {
			return pluginStore.CompilePluginHooks(workspace.WorkspaceRoot)
		})
		runService.SetPluginMCPToolExecutor(hookruntime.MCPToolExecutorFunc(func(ctx context.Context, hook hookruntime.CompiledHook, arguments map[string]any) (string, error) {
			identity, err := pluginStore.ResolveHookMCPTool(hook.PluginName, hook.PluginDigest, hook.Command.MCPServer, hook.Command.MCPTool)
			if err != nil {
				return "", err
			}
			result, err := manager.CallTool(ctx, mcpclient.McpToolName(identity, hook.Command.MCPTool), arguments)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return "", err
			}
			if len(encoded) > 8*1024 {
				encoded = append(encoded[:8*1024], []byte("...")...)
			}
			return string(encoded), nil
		}))
		runService.SetPluginAgentResolver(func(name, digest, logicalID string) (runtime.PluginAgentSelection, error) {
			role, err := pluginStore.ResolveAgentSelection(name, digest, logicalID)
			if err != nil {
				return runtime.PluginAgentSelection{}, err
			}
			return runtime.PluginAgentSelection{AgentType: role.Type, ReadOnly: role.ReadOnly}, nil
		})
		runService.SetPluginRuntimeLeaseProvider(func() (string, func(), error) {
			lease, err := pluginStore.AcquireRuntimeLease()
			if err != nil {
				return "", nil, err
			}
			return lease.Generation, func() { pluginStore.ReleaseRuntimeLease(lease.ID) }, nil
		})
	}

	// Load user Computer Use settings once at startup. Defaults are safe and
	// disable Computer Use until the user explicitly turns it on.
	var computerSettings computer.Settings
	if computerSettingsStore, settingsErr := computer.NewSettingsStore(); settingsErr == nil {
		if doc, docErr := computerSettingsStore.Get(); docErr == nil {
			computerSettings = doc.Value
		} else {
			log.Printf("[computer] failed to read settings, using defaults: %v", docErr)
			computerSettings = computer.DefaultSettings()
		}
	} else {
		log.Printf("[computer] settings store unavailable, using defaults: %v", settingsErr)
		computerSettings = computer.DefaultSettings()
	}
	runService.SetComputerSettings(computerSettings)

	var computerVault computer.MediaVault
	if userDataDir != "" {
		vaultRoot := filepath.Join(userDataDir, "computer-media")
		vault, vaultErr := computer.NewFileMediaVault(computer.VaultOptions{
			RootDir: vaultRoot,
			OnAccess: func(ctx context.Context, sessionID computer.SessionID, ref computer.MediaRef) {
				log.Printf("[computer] media accessed session=%s key=%s sha256=%s", sessionID, ref.StorageKey, ref.SHA256)
			},
			OnDelete: func(ctx context.Context, sessionID computer.SessionID, ref computer.MediaRef, reason string) {
				log.Printf("[computer] media deleted session=%s key=%s sha256=%s reason=%s", sessionID, ref.StorageKey, ref.SHA256, reason)
			},
			Retention: computer.DefaultMediaRetentionOptions(),
		})
		if vaultErr != nil {
			log.Printf("[computer] failed to create session media store: %v", vaultErr)
		} else {
			computerVault = vault
			runService.SetComputerMediaVault(vault)
			defer vault.Close()
		}
	}

	// CUA Host manager: shared across runs, auto-discovers the host script, and
	// falls back to unavailable driver when the host is not installed.
	cuaHostManager := cua.NewHostManager(cua.HostManagerOptions{
		StartupTimeout:  30 * time.Second,
		HealthInterval:  30 * time.Second,
		RestartCooldown: 5 * time.Second,
		OnStateChange: func(available bool, err error) {
			if available {
				log.Println("[cua-host-manager] host available")
			} else {
				log.Printf("[cua-host-manager] host unavailable: %v", err)
			}
		},
	})
	defer cuaHostManager.Close()

	if cuaFactory := buildCUADriverFactory(cuaHostManager, computerVault); cuaFactory != nil {
		runService.SetComputerDriverFactory(cuaFactory)
	}
	runService.SetComputerHostProbe(computer.HostProbeFunc(func(ctx context.Context) (computer.HostStatus, error) {
		return probeCUAHost(ctx, cuaHostManager)
	}))
	runService.SetComputerHostPermissionRequester(computer.HostPermissionRequesterFunc(func(ctx context.Context) (computer.HostStatus, error) {
		return requestCUAHostPermissions(ctx, cuaHostManager)
	}))
	skillManager.SetOnActivated(func(plan *skillruntime.SkillActivationPlan) {
		if plan == nil {
			return
		}
		log.Printf("[skills] activated %s status=%s tools=%d receipts=%d",
			plan.SkillID, plan.CompatibilityStatus, len(plan.EffectiveTools), len(plan.ResourceReceipts))
	})
	// Core supports starting without any workspace; runs still require one at
	// StartRun time. Only configure the service default when the host
	// provided a real path.
	if defaultWorkspace != "" {
		if err := runService.SetDefaultWorkspace(runtime.WorkspaceConfig{WorkspaceRoot: defaultWorkspace}); err != nil {
			log.Fatalf("failed to configure default workspace: %v", err)
		}
	}
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	if projectService := runService.ProjectService(); projectService != nil {
		go projectService.RunCleanupWorker(cleanupCtx)
	}

	// Scheduled tasks: open the repository fail-closed, wire the executor
	// against RunService and run startup recovery before HTTP becomes ready.
	var schedSvc *scheduledtask.Service
	var sched *scheduledtask.Scheduler
	if home := desktopdata.AwakenHomeRoot(); home != "" {
		genDir, _, err := filestore.ReadCurrentGeneration(home)
		if err != nil {
			log.Fatalf("scheduled tasks: resolve store generation: %v", err)
		}
		repo, err := scheduledtask.OpenFileRepository(genDir, scheduledtask.RealClock{})
		if err != nil {
			log.Fatalf("scheduled tasks: open repository: %v", err)
		}
		profileStore := modelprofile.NewUserStore(home)
		validator := scheduledtask.Validator{
			CheckProfile: func(profileID string) error {
				_, err := profileStore.Resolve(profileID)
				return err
			},
		}
		if projectService := runService.ProjectService(); projectService != nil {
			validator.CheckProject = func(projectID string) error {
				projects, err := projectService.ListProjects(false)
				if err != nil {
					return err
				}
				for _, project := range projects {
					if project.ProjectID == projectID {
						return nil
					}
				}
				return errors.New("scheduled tasks: project unavailable")
			}
		}
		executor := scheduledtask.NewRunServiceExecutor(runService, profileStore)
		schedSvc = scheduledtask.NewService(repo, validator, executor)
		sched = scheduledtask.NewScheduler(scheduledtask.SchedulerConfig{
			Repo:      repo,
			Executor:  executor,
			Validator: validator,
			Emit:      schedSvc.PublishEvent,
		})
		schedSvc.SetScheduler(sched)
	} else {
		log.Println("scheduled tasks disabled: AWAKEN_HOME unavailable")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}
	actualAddr := listener.Addr().(*net.TCPAddr)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", actualAddr.Port)

	shutdownCh := make(chan struct{}, 1)
	shutdownOnce := sync.OnceFunc(func() {
		select {
		case shutdownCh <- struct{}{}:
		default:
		}
	})

	var leaseManager *runtime.LeaseManager
	var acpSessionManager *runtime.ACPSessionManager
	var control *server.ControlConfig
	if userDataDir != "" {
		runtimeIdentity, err = runtime.GenerateRuntimeIdentity(userDataDir, *ownerMode, baseURL)
		if err != nil {
			log.Fatalf("generate runtime identity: %v", err)
		}
		if err := runtime.CompareAndSwapManifest(userDataDir, runtimeIdentity.InstanceID, runtimeIdentity); err != nil {
			if errors.Is(err, runtime.ErrManifestOwnedByOther) {
				log.Fatalf("runtime manifest owned by another instance: %v", err)
			}
			log.Fatalf("write runtime manifest: %v", err)
		}
		log.Printf("[runtime] manifest written for instance %s at %s", runtimeIdentity.InstanceID, baseURL)
		if *ownerMode == "headless" {
			leaseManager = runtime.NewLeaseManager(func() {
				shutdownOnce()
			}, runtime.WithActiveRunCountFunc(runService.ActiveRunCount))
		} else {
			// Desktop mode still exposes control lease routes for Desktop attach.
			leaseManager = runtime.NewLeaseManager(nil)
		}
		acpSessionManager = runtime.NewACPSessionManager(runService, runService.ProjectService())
		control = &server.ControlConfig{
			Identity:                 runtimeIdentity,
			LeaseManager:             leaseManager,
			ACPSessionManager:        acpSessionManager,
			ModelProfileStore:        modelprofile.NewUserStore(desktopdata.AwakenHomeRoot()),
			NetworkRoute:             routeManager,
			ReloadAgentPlanMediaTool: mediagenerate.ReloadRuntimeTool,
			PluginAuth:               pluginAuth,
			ConnectorAuth:            connectorAuth,
		}
	}

	srv := &http.Server{
		Handler: server.NewRouterWithScheduledTasks(
			runService,
			infoProvider,
			manager,
			connRegistry,
			skillRegistry,
			acpManager,
			defaultWorkspace,
			syncSkillsForWorkspace,
			control,
			schedSvc,
			pluginStore,
			shutdownOnce,
		),
	}

	// Start the scheduler after the event listener is wired into the router so
	// startup recovery events can be buffered and replayed instead of dropped.
	if sched != nil {
		if err := sched.Start(); err != nil {
			log.Fatalf("scheduled tasks: start scheduler: %v", err)
		}
	}

	if leaseManager != nil {
		leaseCtx, cancelLeases := context.WithCancel(context.Background())
		defer cancelLeases()
		go leaseManager.Run(leaseCtx)
	}

	go func() {
		log.Printf("awaken-core listening on http://%s", baseURL)
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("awaken-core server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-shutdownCh:
	}

	log.Println("awaken-core shutting down")
	cancelCleanup()
	bashtool.CancelAllTasks()
	if acpManager != nil {
		acpCtx, acpCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer acpCancel()
		if err := acpManager.CloseAll(acpCtx); err != nil {
			log.Printf("awaken-core acp close error: %v", err)
		}
	}
	if sched != nil {
		sched.Stop()
	}
	if schedSvc != nil {
		if err := schedSvc.Close(); err != nil {
			log.Printf("awaken-core scheduled tasks close error: %v", err)
		}
	}
	if err := runService.FlushTranscripts(); err != nil {
		log.Printf("awaken-core flush transcripts error: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("awaken-core shutdown error: %v", err)
	}
	if userDataDir != "" && runtimeIdentity != nil {
		if err := runtime.DeleteManifestIfOwned(userDataDir, runtimeIdentity.InstanceID); err != nil {
			log.Printf("awaken-core delete manifest error: %v", err)
		}
	}
	if leaseManager != nil {
		leaseManager.Stop()
	}
}

func modelConfigFromProfile(profile *runtime.ModelProfileConfig) agentcore.ModelConfig {
	if profile == nil {
		return agentcore.ModelConfig{}
	}
	return agentcore.ModelConfig{
		APIKey:              profile.APIKey,
		BaseURL:             profile.BaseURL,
		InferenceEndpoint:   profile.InferenceEndpoint,
		Model:               profile.ModelID,
		CapabilityModelID:   profile.CapabilityModelID,
		EndpointSelector:    profile.EndpointSelector,
		ModelSelectorKind:   profile.ModelSelectorKind,
		Protocol:            profile.Protocol,
		ProviderID:          profile.ProviderID,
		ProfileID:           profile.ProfileID,
		CredentialRevision:  profile.CredentialRevision,
		Temperature:         profile.Temperature,
		MaxTokens:           profile.MaxTokens,
		AuthScheme:          profile.AuthScheme,
		ContextWindow:       profile.ContextWindow,
		MaxInputTokens:      profile.MaxInputTokens,
		SupportsPromptCache: profile.SupportsPromptCache,
		ReasoningEffort:     profile.ReasoningEffort,
		Capabilities: tool.ModelCapabilities{
			SupportsVision:      profile.SupportsVision,
			SupportsNativePDF:   profile.SupportsNativePDF,
			SupportsTools:       profile.SupportsTools,
			SupportsComputerUse: profile.SupportsComputerUse,
		},
	}
}

func purposeModelConfig(profile *runtime.ModelProfileConfig, purpose string) agentcore.ModelConfig {
	if profile == nil {
		return agentcore.ModelConfig{}
	}
	return modelConfigFromProfile(profile.PurposeProfiles[purpose])
}

// buildMCPManager loads project and user MCP configs, merges them, and
// starts the manager. Failures are logged but never fatal: the runtime
// stays up even if no MCP server is configured. The manager is still returned
// with an empty server set so connectors can inject MCP configs dynamically.
func buildMCPManager(workspace string, pluginStore *plugins.Store, inProcRegistry *mcpclient.InProcRegistry, credentialStore mcpclient.CredentialStore) *mcpclient.Manager {
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		log.Printf("[mcp] user config load failed: %v", err)
	}
	var projectCfg mcpclient.ConfigFile
	if workspace != "" {
		projectCfg, err = mcpclient.LoadProjectConfig(workspace)
		if err != nil {
			log.Printf("[mcp] project config load failed: %v", err)
		}
	}
	var pluginScoped []mcpclient.ScopedMcpServerConfig
	if pluginStore != nil && pluginStore.Available() == nil {
		pluginScoped = pluginStore.PluginMCPServers()
	}
	merged := mcpclient.MergeScoped(
		pluginScoped,
		mcpclient.ToScoped(userCfg, mcpclient.ScopeUser),
		mcpclient.ToScoped(projectCfg, mcpclient.ScopeProject),
		nil,
		mcpclient.StdLogger{},
	)
	options := []mcpclient.ManagerOption{mcpclient.WithLogger(mcpclient.StdLogger{})}
	if inProcRegistry != nil {
		options = append(options, mcpclient.WithInProcRegistry(inProcRegistry))
	}
	manager := mcpclient.NewManager(options...)
	if credentialStore == nil {
		credentialStore = mcpclient.NewPersistentCredentialStore()
	}
	manager.SetCredentialStore(credentialStore)
	if len(merged) > 0 {
		go manager.Start(context.Background(), merged)
	}
	return manager
}

// buildConnectorRegistry loads the connector sources (builtin + local) and
// returns a connector registry backed by the MCP manager. Returns nil when
// no MCP manager is available, since connector lifecycle depends on it.
func buildConnectorRegistry(manager *mcpclient.Manager, _ string, legacyCLIReview []string) *connectors.Registry {
	if manager == nil {
		return nil
	}
	ctx := context.Background()

	localSource, err := connectors.NewLocalSource(connectors.StdLogger{})
	if err != nil {
		log.Printf("[connectors] local source load failed: %v", err)
	}

	var sources []connectors.ConnectorSource
	sources = append(sources, connectors.NewBuiltinSource(connectors.BuiltinOptions{
		MicrosoftClientID: resolvedMicrosoftOutlookClientID(),
	}))
	if localSource != nil {
		sources = append(sources, localSource)
	}
	sources = append(sources, connectors.NewNoopTeamSource())

	logger := connectors.StdLogger{}
	registry := connectors.NewRegistry(manager, logger)
	registry.SetCLIControlToken(os.Getenv("AWAKEN_DESKTOP_CONTROL_TOKEN"))
	if err := registry.LoadSources(ctx, sources); err != nil {
		log.Printf("[connectors] load sources failed: %v", err)
	}
	registry.SetLegacyCLIReviewRequired(legacyCLIReview)
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		log.Printf("[connectors] user mcp config load failed: %v", err)
	} else {
		registry.RestoreBuiltinOwnership(userCfg)
	}

	log.Printf("[connectors] loaded %d connector definitions", len(registry.List()))
	return registry
}

func resolvedMicrosoftOutlookClientID() string {
	if value := strings.TrimSpace(os.Getenv("AWAKEN_MICROSOFT_CLIENT_ID")); value != "" {
		return value
	}
	return strings.TrimSpace(microsoftOutlookClientID)
}

func buildSkillRegistry(manager *mcpclient.Manager, workspace string) *skills.SkillRegistry {
	logger := mcpclient.StdLogger{}
	registry := skills.NewSkillRegistry(logger)
	if home, err := os.UserHomeDir(); err == nil {
		registry.SetDisabledStorePath(filepath.Join(home, ".awaken", "skills", "disabled-skills.json"))
	}
	registry.LoadFromSources(skills.LocalSourceDirs(workspace))

	if manager != nil {
		bridge := skills.NewBridge(logger, manager)
		go bridge.Start(context.Background(), manager, registry)
	}

	log.Printf("[skills] loaded %d skill definitions", len(registry.ListAll()))
	return registry
}

func applyPluginMCPServers(manager *mcpclient.Manager, store *plugins.Store) {
	if manager == nil || store == nil || store.Available() != nil {
		return
	}
	if err := plugins.ReconcileMCPServers(context.Background(), manager, store.PluginMCPServers()); err != nil {
		log.Printf("[plugins] reconcile MCP runtime: %v", err)
	}
}

func skillSourceDirsForWorkspace(workspace runtime.WorkspaceConfig) map[skills.SourceOrigin]string {
	if workspace.SessionWorkspace {
		return skills.LocalSourceDirs("")
	}
	projectRoots := make([]string, 0, len(workspace.ReadableRoots)+1)
	projectRoots = append(projectRoots, workspace.WorkspaceRoot)
	projectRoots = append(projectRoots, workspace.ReadableRoots...)
	return skills.LocalSourceDirsForProjectRoots(projectRoots)
}

// buildACPManager loads the user-scoped runner config and starts the ACP manager.
// The manager is created even if no runners are configured so routes degrade
// gracefully. ACP runners are intentionally user-scoped only; project-level
// runner configs are not loaded or merged.
func buildACPManager(defaultWorkspace string) *acp.Manager {
	_ = defaultWorkspace
	userPath := ""
	if home := desktopdata.AwakenHomeRoot(); home != "" {
		userPath = acp.ConfigPath(home)
	}
	registry := acp.NewRegistry(userPath)
	if err := registry.Load(); err != nil {
		log.Printf("[acp] registry load failed: %v", err)
	}
	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return protocol.NewClient(ctx, snapshot, cwd)
	})
	manager.SetPermissionAwaiter(acp.NewChanPermissionAwaiter())
	log.Printf("[acp] loaded %d runners", len(registry.All()))
	return manager
}

// resolveDefaultWorkspace returns the startup default workspace. When the
// host sets AWAKEN_DEFAULT_WORKSPACE (even to an empty value) that value is
// authoritative: an empty value means "no default workspace" and Core starts
// without one instead of guessing cwd or home. The cwd fallback only applies
// to standalone launches where the variable is not set at all.
func resolveDefaultWorkspace() string {
	if defaultWorkspace, ok := os.LookupEnv("AWAKEN_DEFAULT_WORKSPACE"); ok {
		return strings.TrimSpace(defaultWorkspace)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
	return cwd
}

// probeCUAHost returns the global availability of the CUA Host sidecar for
// settings and readiness checks. It reuses the shared HostManager client when
// already healthy, or attempts a short-lived start when not.
func probeCUAHost(ctx context.Context, manager *cua.HostManager) (computer.HostStatus, error) {
	status := computer.HostStatus{Available: false, Healthy: false}
	if manager == nil {
		status.Message = "host manager not configured"
		return status, nil
	}

	scriptPath, _, err := manager.ResolvedHostPaths()
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}
	status.HostScriptPath = scriptPath
	status.NodePath = manager.ResolvedNodePath()

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := manager.GetClient(probeCtx)
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}

	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	defer healthCancel()
	hr, err := client.Health(healthCtx)
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}

	status.Available = true
	status.Healthy = hr.Healthy
	status.Version = hr.Version
	status.ProtocolVersion = hr.ProtocolVersion
	status.OS = hr.OS
	status.TCCScreenRecording = hr.TCCScreenRecording
	status.TCCAccessibility = hr.TCCAccessibility
	status.Message = hr.Message
	return status, nil
}

// requestCUAHostPermissions executes the macOS permission request inside the
// CUA Host process, then projects the refreshed Host-owned TCC state.
func requestCUAHostPermissions(ctx context.Context, manager *cua.HostManager) (computer.HostStatus, error) {
	status := computer.HostStatus{Available: false, Healthy: false}
	if manager == nil {
		status.Message = "host manager not configured"
		return status, nil
	}

	scriptPath, _, err := manager.ResolvedHostPaths()
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}
	status.HostScriptPath = scriptPath
	status.NodePath = manager.ResolvedNodePath()

	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := manager.GetClient(requestCtx)
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}
	hr, err := client.RequestPermissions(requestCtx)
	if err != nil {
		return status, err
	}
	status.Available = true
	status.Healthy = hr.Healthy
	status.Version = hr.Version
	status.ProtocolVersion = hr.ProtocolVersion
	status.OS = hr.OS
	status.TCCScreenRecording = hr.TCCScreenRecording
	status.TCCAccessibility = hr.TCCAccessibility
	status.Message = hr.Message
	return status, nil
}

// buildCUADriverFactory returns a factory for the CUA driver backed by the
// shared host manager. The manager auto-discovers the host script (env var,
// bundled resources, or PoC workspace) and starts the host on first use. If the
// host cannot be started, the factory falls back to the unavailable driver so
// that runs can still start without Computer Use. The legacy AWAKEN_CUA_ENABLED
// environment variable is no longer required.
func buildCUADriverFactory(manager *cua.HostManager, vault computer.MediaVault) func(computer.Binding) (computer.Driver, error) {
	return func(bind computer.Binding) (computer.Driver, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := manager.GetClient(ctx)
		if err != nil {
			log.Printf("[cua] host unavailable, falling back to unavailable driver: %v", err)
			return computer.NewUnavailableDriver(), nil
		}
		return cua.NewDriver(cua.DriverOptions{
			Host:       client,
			MediaVault: vault,
		})
	}
}
