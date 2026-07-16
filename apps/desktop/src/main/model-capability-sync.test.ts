import { describe, expect, it } from "vitest";
import { parseProviderCapabilities } from "./model-capability-sync.js";

describe("model capability sync", () => {
  it("parses Kimi Code model metadata field names", () => {
    expect(
      parseProviderCapabilities({
        data: [
          {
            id: "k3",
            context_length: 1_048_576,
            supports_image_in: true,
            supports_video_in: true,
          },
        ],
      }),
    ).toEqual({
      k3: {
        context_window: 1_048_576,
        supports_vision: true,
        supports_video: true,
      },
    });
  });
});
