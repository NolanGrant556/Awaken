import { describe, expect, it } from "vitest";
import { compactEventDetails } from "./compact-event";

describe("compact wire compatibility", () => {
  it("prefers current structured fields over aliases and message text", () => {
    expect(compactEventDetails({
      type: "AWAKEN_COMPACT_FINISHED",
      contextCompactMode: "manual", compactMode: "auto", contextCompactBoundaryId: "new",
      compactBoundaryId: "old", contextCompactPreEstimateTokens: 100,
      contextCompactPostEstimateTokens: 20, compactPostEstimate: 30,
      message: "compact finished: legacy boundary=text estimated_tokens=40",
    })).toMatchObject({
      mode: "manual", boundaryId: "new", preCompactEstimatedTokens: 100,
      postCompactEstimatedTokens: 20, fallbackState: "structured"
    });
  });
  it("accepts older aliases, zero token counts, and text-only records", () => {
    expect(compactEventDetails({
      type: "AWAKEN_COMPACT_FINISHED", compactMode: "auto",
      compactBoundaryId: "old", compactPostEstimate: 0
    })).toMatchObject({ boundaryId: "old", postCompactEstimatedTokens: 0, fallbackState: "legacy" });
    expect(compactEventDetails({
      type: "AWAKEN_COMPACT_FINISHED",
      message: "compact finished: manual boundary=v1 estimated_tokens=12"
    }, 90))
      .toMatchObject({ mode: "manual", boundaryId: "v1", preCompactEstimatedTokens: 90, postCompactEstimatedTokens: 12 });
  });
});
