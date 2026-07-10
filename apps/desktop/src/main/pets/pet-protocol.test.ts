import { describe, expect, it } from "vitest";
import { parsePetAssetUrl } from "./pet-protocol.js";

describe("parsePetAssetUrl", () => {
  it("parses cache-busted spritesheet URLs", () => {
    expect(
      parsePetAssetUrl(
        "awaken-pet://asset/codex-custom%3Amy-pet/spritesheet?v=abc123",
      ),
    ).toEqual({
      petId: "codex-custom:my-pet",
      assetName: "spritesheet",
    });
  });

  it("rejects malformed or extra path segments", () => {
    expect(parsePetAssetUrl("awaken-pet://asset/%/spritesheet")).toBeNull();
    expect(
      parsePetAssetUrl("awaken-pet://asset/pet/spritesheet/extra"),
    ).toBeNull();
  });
});
