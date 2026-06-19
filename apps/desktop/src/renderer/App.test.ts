import { describe, expect, it } from "vitest";
import { resolvedDecisionForApproval } from "./App";

describe("resolvedDecisionForApproval", () => {
  it("returns undefined when both approval states are empty", () => {
    expect(resolvedDecisionForApproval(undefined, null)).toBeUndefined();
  });

  it("returns undefined when the resolved approval belongs to another approval", () => {
    expect(
      resolvedDecisionForApproval(
        { id: "approval-1" },
        { approvalId: "approval-2", decision: "approved" },
      ),
    ).toBeUndefined();
  });

  it("returns the decision for the active approval", () => {
    expect(
      resolvedDecisionForApproval(
        { id: "approval-1" },
        { approvalId: "approval-1", decision: "rejected" },
      ),
    ).toBe("rejected");
  });
});
