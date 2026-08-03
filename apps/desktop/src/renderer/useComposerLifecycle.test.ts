import { describe, expect, it } from "vitest";
import { composerRuntimePanelsForStatus } from "./useComposerLifecycle";

describe("composer runtime presentation", () => {
  it("keeps run failures in the conversation narrative instead of duplicating an error card", () => {
    expect(
      composerRuntimePanelsForStatus({
        desktopNeedsRestart: false,
        desktopStarting: false,
      }),
    ).toEqual([]);
  });
});
