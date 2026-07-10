/** Display projection of canonical ToolOutcome.toolStatus. */
export type ToolDisplayState =
  | "active"
  | "done"
  | "error"
  | "denied"
  | "cancelled"
  | "interrupted"
  | "blocked"
  | "unsupported";

/**
 * Maps canonical toolStatus to ToolDisplayState.
 * Never infer status from content emptiness or Chinese error substrings.
 * Callers must pass "active" themselves when only ToolCall exists (no ToolResult).
 */
export function toolDisplayStateFromStatus(
  toolStatus?: string,
): ToolDisplayState {
  switch ((toolStatus ?? "").trim()) {
    case "succeeded":
      return "done";
    case "failed":
      return "error";
    case "denied":
      return "denied";
    case "cancelled":
      return "cancelled";
    case "interrupted":
      return "interrupted";
    case "needs_auth":
    case "needs_runtime":
    case "review_required":
      return "blocked";
    case "unsupported":
      return "unsupported";
    case "":
      // Missing toolStatus on a result event: do not treat empty content as error.
      return "done";
    default:
      return "done";
  }
}
