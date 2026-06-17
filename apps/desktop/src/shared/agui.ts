export type AguiEventType =
  | "RUN_STARTED"
  | "RUN_FINISHED"
  | "RUN_ERROR"
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
  | "TOOL_CALL_RESULT";

export type AguiRunOutcome = {
  type: string;
};

export type AguiEvent = {
  type: AguiEventType;
  timestamp?: number;
  threadId?: string;
  runId?: string;
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
  outcome?: AguiRunOutcome;
};
