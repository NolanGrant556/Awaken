export type AguiEventType =
  | "RUN_STARTED"
  | "RUN_FINISHED"
  | "RUN_ERROR"
  | "RUN_STATUS_CHANGED"
  | "TEXT_MESSAGE_START"
  | "TEXT_MESSAGE_CONTENT"
  | "TEXT_MESSAGE_END"
  | "REASONING_START"
  | "REASONING_MESSAGE_START"
  | "REASONING_MESSAGE_CONTENT"
  | "REASONING_MESSAGE_END"
  | "REASONING_END"
  | "TOOL_CALL_START"
  | "TOOL_CALL_ARGS"
  | "TOOL_CALL_END"
  | "TOOL_CALL_RESULT"
  | "AWAKEN_APPROVAL_REQUESTED"
  | "AWAKEN_APPROVAL_RESOLVED"
  | "AWAKEN_ARTIFACT_CREATED"
  | "AWAKEN_ARTIFACT_UPDATED";

export type AguiApprovalRiskTone = "safe" | "warning" | "danger";

export type AguiApprovalDecision = "approved" | "rejected";

export type AguiArtifactStatus = "ready" | "pending" | "error";

export type AguiRunOutcome = {
  type: string;
};

export type AguiEvent = {
  type: AguiEventType;
  timestamp?: number;
  threadId?: string;
  runId?: string;
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
  message?: string;
  code?: string;
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
};
