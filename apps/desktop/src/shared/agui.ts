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
  | "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED"
  | "AWAKEN_MASS_MESSAGE_REVIEW_RESOLVED"
  | "AWAKEN_USER_QUESTION_REQUESTED"
  | "AWAKEN_USER_QUESTION_RESOLVED"
  | "AWAKEN_ARTIFACT_CREATED"
  | "AWAKEN_ARTIFACT_UPDATED"
  | "AWAKEN_TOOL_RECOVERY_REQUIRED"
  | "AWAKEN_TOOL_RETRY_BLOCKED"
  | "AWAKEN_UNKNOWN_EVENT";

export type AguiApprovalRiskTone = "safe" | "warning" | "danger";

export type AguiApprovalDecision = "approved" | "rejected";

export type AguiArtifactStatus = "ready" | "pending" | "error";

export type AguiRunOutcome = {
  type: string;
};

export type AguiEvent = {
  type: AguiEventType;
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
  content?: string;
  result?: string;
  progress?: string;
  commandStream?: string;
  patch?: string;
  patchSummary?: string;
  affectedPaths?: string[];
  diff?: string;
  truncated?: boolean;
  message?: string;
  code?: string;
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
  /** Original tool name when the decision was a transform (e.g. "shell"). */
  transformedFrom?: string;
  /** Rewritten tool name when the decision was a transform (e.g. "apply_patch"). */
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
  authProvider?: string;
  authScope?: string;
  authLink?: string;
  runtimeName?: string;
  runtimeComponent?: string;
  runtimeRequiredVersion?: string;
  runtimeDetectedVersion?: string;
  runtimePrepareStatus?: string;
  runtimeMissingReason?: string;
  reviewId?: string;
  reviewDecision?: "approved" | "rejected";
  questionId?: string;
  questionAnswer?: string;
  massMessageChannel?: string;
  massMessageRecipientScope?: string;
  massMessageRecipientCount?: number;
  massMessageBodyPreview?: string;
  massMessageTemplateId?: string;
  massMessageAttachmentNames?: string[];
  massMessageAccount?: string;
  massMessageSourceTool?: string;
  /** Legacy transcript-only fields. New runs must not emit generic approval events. */
  approvalId?: string;
  approvalTitle?: string;
  approvalDescription?: string;
  approvalRiskTone?: AguiApprovalRiskTone;
  approvalDecision?: AguiApprovalDecision;
  approvalAffectedPaths?: string[];
  approvalPatchSummary?: string;
  artifactId?: string;
  artifactTitle?: string;
  artifactDescription?: string;
  artifactKind?: string;
  artifactPath?: string;
  artifactContent?: string;
  artifactStatus?: AguiArtifactStatus;
  outcome?: AguiRunOutcome;
  rawType?: string;
  rawEvent?: Record<string, unknown>;
};
