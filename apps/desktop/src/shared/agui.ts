export type AguiEventType =
  | "RUN_STARTED"
  | "RUN_FINISHED"
  | "RUN_ERROR"
  | "RUN_STATUS_CHANGED"
  | "AWAKEN_TURN_STARTED"
  | "AWAKEN_TURN_FINISHED"
  | "TEXT_MESSAGE_START"
  | "TEXT_MESSAGE_CONTENT"
  | "TEXT_MESSAGE_END"
  | "REASONING_START"
  | "REASONING_MESSAGE_START"
  | "REASONING_MESSAGE_CONTENT"
  | "REASONING_MESSAGE_END"
  | "REASONING_END"
  | "TOOL_CALL_START"
  | "TOOL_CALL_ARGS_DELTA"
  | "TOOL_CALL_ARGS"
  | "TOOL_CALL_END"
  | "TOOL_PROGRESS"
  | "TOOL_CALL_RESULT"
  | "AWAKEN_TOOL_PERMISSION_DECIDED"
  | "AWAKEN_COMMAND_OUTPUT"
  | "AWAKEN_PATCH_UPDATED"
  | "AWAKEN_TURN_DIFF"
  | "AWAKEN_CONTEXT_ESTIMATED"
  | "AWAKEN_CONTEXT_WARNING"
  | "AWAKEN_CONTEXT_BLOCKED"
  | "AWAKEN_COMPACT_STARTED"
  | "AWAKEN_COMPACT_FINISHED"
  | "AWAKEN_COMPACT_FAILED"
  | "AWAKEN_TOOL_PAIRING_REPAIRED"
  | "AWAKEN_AUTH_REQUIRED"
  | "AWAKEN_RUNTIME_PREPARE"
  | "AWAKEN_RUNTIME_PREPARE_PROGRESS"
  | "AWAKEN_RUNTIME_READY"
  | "AWAKEN_RUNTIME_REPAIR_FAILED"
  | "AWAKEN_RUNTIME_MISSING"
  | "AWAKEN_SKILL_DISCOVERED"
  | "AWAKEN_SKILL_INVOKED"
  | "AWAKEN_SKILL_CONTENT_LOADED"
  | "AWAKEN_SKILL_EXECUTION_CONTEXT"
  | "AWAKEN_SKILL_FAILED_TO_LOAD"
  | "AWAKEN_HOOK_STARTED"
  | "AWAKEN_HOOK_COMPLETED"
  | "AWAKEN_USER_QUESTION_REQUESTED"
  | "AWAKEN_USER_QUESTION_RESOLVED"
  | "AWAKEN_EXTERNAL_INTERACTION_REQUESTED"
  | "AWAKEN_EXTERNAL_INTERACTION_RESOLVED"
  | "AWAKEN_ACP_SESSION_CHANGED"
  | "AWAKEN_ACP_TURN_CHANGED"
  | "AWAKEN_ACP_TOOL_CHANGED"
  | "AWAKEN_ACP_PERMISSION_REQUESTED"
  | "AWAKEN_ACP_PERMISSION_RESOLVED"
  | "AWAKEN_ARTIFACT_CREATED"
  | "AWAKEN_ARTIFACT_UPDATED"
  | "AWAKEN_TOOL_RECOVERY_REQUIRED"
  | "AWAKEN_TOOL_RETRY_BLOCKED"
  | "AWAKEN_MEMORY_RECALLED"
  | "AWAKEN_MEMORY_CREATED"
  | "AWAKEN_MEMORY_UPDATED"
  | "AWAKEN_MEMORY_SUPERSEDED"
  | "AWAKEN_MEMORY_DELETED"
  | "AWAKEN_MEMORY_REJECTED"
  | "AWAKEN_CONTEXT_SNAPSHOT"
	| "AWAKEN_INSTRUCTION_SNAPSHOT"
  | "AWAKEN_MODEL_USAGE"
  | "AWAKEN_SUBAGENT_CHANGED"
  | "AWAKEN_SUBAGENT_ACTIVITY"
  | "AWAKEN_SUBAGENT_MESSAGE_QUEUED"
  | "AWAKEN_MEMORY_DREAM_COMPLETED"
  | "AWAKEN_MEMORY_DREAM_FAILED"
  | "AWAKEN_MEMORY_DREAM_CANCELLED"
  | "AWAKEN_MEMORY_DREAM_INTERRUPTED"
  | "AWAKEN_SCHEDULED_NOTICE"
  | "AWAKEN_PLAN_UPDATED"
  | "AWAKEN_COMPUTER_REVIEW_REQUESTED"
  | "AWAKEN_COMPUTER_REVIEW_RESOLVED"
  | "AWAKEN_COMPUTER_SESSION_CHANGED"
  | "AWAKEN_COMPUTER_VERIFICATION_COMPLETED"
  | "AWAKEN_COMPUTER_HANDOFF_STARTED"
  | "AWAKEN_COMPUTER_HANDOFF_ENDED"
  | "AWAKEN_PROVIDER_RECONNECT"
  | "AWAKEN_UNKNOWN_EVENT";

export type AguiMemoryReceipt = {
  memoryId: string;
  scope: string;
  memoryType: string;
  revision: number;
  bytes?: number;
  action?: string;
  status?: string;
  errorCode?: string;
};

export type AguiArtifactStatus = "ready" | "pending" | "error";

export type AguiArtifactRole = "deliverable" | "preview" | "support";

export type AguiACPPermissionOption = {
  optionId: string;
  name: string;
  kind?: string;
};

export type AguiRunOutcome = {
  type: string;
};

export type AguiProviderReconnectPhase =
  | "waiting_for_network"
  | "reconnecting"
  | "recovered";

export type AguiProviderReconnect = {
  phase: AguiProviderReconnectPhase;
  attempt?: number;
  maxAttempts?: number;
  detail?: string;
  resetInProgress?: boolean;
};

export type AguiRunPlanItem = {
  step: string;
  activeForm: string;
  status: string;
};

export type AguiContextContributionKind =
  | "system_prompt"
  | "rules_project"
  | "runtime_context"
  | "builtin_tools"
  | "mcp"
  | "skills"
  | "memory"
  | "conversation"
  // v1 contribution kinds remain readable for old SemanticJournal records.
  | "mcp_dynamic_tools"
  | "skills_memory"
  | "subagent_definitions"
  | "conversation_history"
  | "tool_results"
  | "attachments_media"
  | "current_input"
  | "other";

export type AguiContextUsageContribution = {
  kind: AguiContextContributionKind;
  estimatedTokens: number;
  sourceCount: number;
};

export type AguiContextUsageSnapshot = {
  schemaVersion: number;
  snapshotId: string;
  estimatorId: string;
  basis: "current_projection" | "sent_request";
  requestId?: string;
  modelId?: string;
  capabilityModelId?: string;
  endpointSelector?: string;
  modelSelectorKind?: "model" | "endpoint";
  protocolFamily?: string;
  contextEpochId?: string;
  contextEpochBoundaryReason?: string;
  requestShapeDigest?: string;
  modelWindowTokens: number;
  outputReserveTokens: number;
  effectiveInputWindowTokens: number;
  estimatedInputTokens: number;
  warningThresholdTokens: number;
  autoCompactThresholdTokens: number;
  compactTargetTokens: number;
  contributions?: AguiContextUsageContribution[];
  generatedAt: string;
};

export type InstructionDiagnostic = {
	code: string;
	message: string;
	scope: "global" | "project";
	displayPath: string;
};

export type InstructionSourceSummary = {
	scope: "global" | "project";
	displayPath: string;
	scopeRoot?: string;
	digest?: string;
	originalBytes: number;
	appliedBytes: number;
	truncated: boolean;
	status: "applied" | "skipped";
};

export type InstructionSnapshotSummary = {
	snapshotId: string;
	capturedAt: string;
	partial: boolean;
	sources: InstructionSourceSummary[];
	diagnostics: InstructionDiagnostic[];
};

export type AguiModelUsageRecord = {
  schemaVersion: number;
  requestId: string;
  purpose: string;
  providerId?: string;
  profileId?: string;
  credentialRevision?: number;
  modelId?: string;
  capabilityModelId?: string;
  endpointSelector?: string;
  modelSelectorKind?: "model" | "endpoint";
  protocolFamily?: string;
  endpointContractId?: string;
  reasoningPolicy?: string;
  cacheMode?: string;
  cacheKeyApplied?: boolean;
  contextEpochId?: string;
  prefixDigest?: string;
  invalidationReason?: string;
  retryIndex?: number;
  auxiliaryFallback?: boolean;
  sentEstimatedInputTokens?: number;
  inputTokens?: number;
  cachedInputTokens?: number;
  uncachedInputTokens?: number;
  cacheWriteTokens?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  totalTokens?: number;
  rawTypedUsage?: AguiRawModelUsage;
  usageComplete: boolean;
  calculatedCostUsd?: number;
  pricingSnapshot?: AguiPricingSnapshot;
  currency?: string;
  providerRequestId?: string;
  latencyMs: number;
  status: "succeeded" | "failed" | "cancelled" | "interrupted";
  errorCode?: string;
  completedAt: string;
};

export type AguiPricingSnapshot = {
  snapshotId: string;
  providerId: string;
  modelId: string;
  currency: string;
  unitTokens: number;
  cachedInputPerUnitUsd: number;
  uncachedInputPerUnitUsd: number;
  outputPerUnitUsd: number;
  effectiveDate: string;
  sourceUrl: string;
};

export type AguiRawModelUsage = {
  normalizerId: string;
  promptTokens?: number;
  inputTokens?: number;
  completionTokens?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  totalTokens?: number;
  cachedTokens?: number;
  promptCacheHitTokens?: number;
  promptCacheMissTokens?: number;
  cacheReadInputTokens?: number;
  cacheCreationInputTokens?: number;
};

export type AguiEvent = {
  type: AguiEventType;
	instructionSnapshot?: InstructionSnapshotSummary;
  timestamp?: number;
  seq?: number;
  /** Live SSE resume cursor; independent from durable semantic seq. */
  streamCursor?: number;
  semanticSeq?: number;
  recordId?: string;
  threadId?: string;
  runId?: string;
  turnId?: string;
  queryChainId?: string;
  depth?: number;
  turnCount?: number;
  status?: string;
  messageId?: string;
  parentRunId?: string;
  parentMessageId?: string;
  role?: string;
  name?: string;
  delta?: string;
  toolCallId?: string;
  toolCallName?: string;
  hookInvocationId?: string;
  hookId?: string;
  hookEvent?: string;
  hookSourceKind?: string;
  hookAction?: string;
  hookReasonCode?: string;
  hookReason?: string;
  hookOutputPreview?: string;
  toolPreparationKind?: "write" | "edit";
  toolPreparationPath?: string;
  toolPreparationAddedLines?: number;
  toolPreparationDeletedLines?: number;
  content?: string;
  result?: string;
  memories?: AguiMemoryReceipt[];
  progress?: string;
  commandStream?: string;
  patch?: string;
  patchSummary?: string;
  affectedPaths?: string[];
  diff?: string;
  truncated?: boolean;
  message?: string;
  code?: string;
  taskId?: string;
  executionId?: string;
  taskName?: string;
  noticeContent?: string;
  // Run-plan full snapshot, surfaced on AWAKEN_PLAN_UPDATED (contract §8.1).
  planId?: string;
  planVersion?: number;
  planExplanation?: string;
  planItems?: AguiRunPlanItem[];
  planCompletedCount?: number;
  planTotalCount?: number;
  planCurrentStep?: number;
  planAllCompleted?: boolean;
  terminalReason?: string;
  transitionReason?: string;
  contextModelWindow?: number;
  contextOutputReserve?: number;
  contextEffectiveWindow?: number;
  contextEstimatedTokens?: number;
  contextActualTokens?: number;
  contextWarningThreshold?: number;
  contextAutoCompactThreshold?: number;
  contextCompactTarget?: number;
  contextEstimatedDeltaTokens?: number;
  contextUsageSnapshot?: AguiContextUsageSnapshot;
  modelUsage?: AguiModelUsageRecord;
  contextCompactMode?: string;
  contextCompactBoundaryId?: string;
  contextCompactPreEstimateTokens?: number;
  contextCompactPostEstimateTokens?: number;
  compactMode?: string;
  compactBoundaryId?: string;
  compactPreEstimate?: number;
  compactPostEstimate?: number;
  compactSummaryModel?: string;
  compactFallbackUsed?: boolean;
  compactToolResultsPruned?: boolean;
  compactToolResultsPrunedCount?: number;
  compactPrunedSafeGroupCount?: number;
  compactPrunedMessageCount?: number;
  compactSummaryMaxTokens?: number;
  compactSummaryRetryCount?: number;
  compactSummaryRawChars?: number;
  compactSummaryParsedSections?: number;
  compactSummaryMeaningfulSections?: number;
  compactSummaryQualityIssue?: string;
  compactSummarySourceHadUser?: boolean;
  compactSummaryLatestUserPinned?: boolean;
  compactFallbackMarker?: string;
  compactFailureReason?: string;
  staticPermissionBehavior?: string;
  staticPermissionReason?: string;
  toolDecisionAction?: string;
  toolDecisionSource?: string;
  toolDecisionReason?: string;
  /** ExecutionProfile name applied to this call (Task 16 unified observability). */
  profileName?: string;
  /** Original tool name when a generic provider transform is used. */
  transformedFrom?: string;
  /** Rewritten tool name when a generic provider transform is used. */
  transformedTo?: string;
  /** Absolute paths moved to the OS recycle bin during this call. */
  trashedPaths?: string[];
  /** Populated when a guardian constraint was violated. */
  guardianViolation?: string;
  toolProgressStage?: string;
  toolProgressMessage?: string;
  toolProgressPercentage?: number;
  /** Canonical ToolOutcome status (succeeded/failed/denied/...). */
  toolStatus?: string;
  transportStatus?: string;
  semanticStatus?: string;
  processStatus?: string;
  processExitCode?: number;
  processTimedOut?: boolean;
  errorCode?: string;
  errorCategory?: string;
  retryable?: boolean;
  recoveryOptions?: string[];
  durationMs?: number;
  diagnostic?: Record<string, unknown>;
  environmentSnapshotId?: string;
  toolAvailabilitySnapshotId?: string;
  providerId?: string;
  providerSource?: string;
  capability?: string;
  capabilities?: string[];
  readiness?: string;
  dependency?: Record<string, unknown>;
  dependencies?: Array<Record<string, unknown>>;
  sideEffectState?: string;
  authProvider?: string;
  authScope?: string;
  authUrl?: string;
  runtimeName?: string;
  runtimeVersion?: string;
  runtimeComponent?: string;
  runtimeRequiredVersion?: string;
  runtimeDetectedVersion?: string;
  runtimePrepareStatus?: string;
  runtimeMissingReason?: string;
  skillId?: string;
  skillVersion?: string;
  skillSource?: string;
  skillPath?: string;
  skillStatus?: string;
  skillResource?: string;
  skillResourceSource?: string;
  skillResourceSha256?: string;
  skillResourceSize?: number;
  skillResourceMode?: string;
  skillEffectiveTools?: string[];
  skillDiagnostics?: string[];
  questionId?: string;
  questionAnswer?: string;
  interactionId?: string;
  interactionResolution?: string;
  interactionTitle?: string;
  interactionSafeMessage?: string;
  interactionVerificationUrl?: string;
  interactionQrImageUrl?: string;
  interactionProvider?: string;
  interactionRecoveries?: string[];
  subagentTaskId?: string;
  subagentAgentId?: string;
  subagentParentAgentId?: string;
  subagentParentToolCallId?: string;
  subagentAgentType?: "explore" | "plan" | "review" | "general";
  subagentDescription?: string;
  subagentStatus?: "queued" | "running" | "completed" | "failed" | "cancelling" | "cancelled" | "interrupted";
  subagentWorkspaceMode?: string;
  subagentModelProfileId?: string;
  subagentActivity?: string;
  subagentToolUseCount?: number;
  subagentEstimatedTokens?: number;
  subagentInputTokens?: number;
  subagentOutputTokens?: number;
  subagentTotalTokens?: number;
  subagentResultPreview?: string;
  subagentErrorCode?: string;
  subagentErrorMessage?: string;
  subagentSideEffectState?: string;
  subagentCreatedAt?: string;
  subagentStartedAt?: string;
  subagentFinishedAt?: string;
  // ACP fields surfaced on AWAKEN_ACP_* events.
  acpSessionId?: string;
  acpRunnerId?: string;
  acpTurnId?: string;
  acpToolCallId?: string;
  acpPermissionOptions?: AguiACPPermissionOption[];
  artifactId?: string;
  artifactGroupId?: string;
  artifactGroupIndex?: number;
  artifactTitle?: string;
  artifactDescription?: string;
  artifactKind?: string;
  artifactPath?: string;
  artifactContent?: string;
  artifactStatus?: AguiArtifactStatus;
  artifactRole?: AguiArtifactRole;
  artifactContractVersion?: number;
  artifactProducer?: string;
  outcome?: AguiRunOutcome;
  rawType?: string;
  rawEvent?: Record<string, unknown>;
  computerReviewId?: string;
  computerActionId?: string;
  computerActionDigest?: string;
  computerTargetSummary?: string;
  computerEffectSummary?: string;
  computerRisk?: string;
  computerSessionId?: string;
  computerSessionState?: string;
  computerVerificationStatus?: string;
  computerObservationId?: string;
  computerDecision?: string;
  computerActor?: string;
  computerHandoff?: boolean;
  computerEvidenceRefs?: string[];
  providerReconnect?: AguiProviderReconnect;
};
