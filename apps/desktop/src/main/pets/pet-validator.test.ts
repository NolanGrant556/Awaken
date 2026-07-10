import { describe, expect, it, vi } from "vitest";
import { validatePetManifestJson, sanitizePetId } from "./pet-validator.js";

vi.mock("electron", () => ({
  nativeImage: {
    createFromPath: (path: string) => {
      const size = path.includes("badsize")
        ? { width: 100, height: 100 }
        : { width: 1536, height: 1872 };
      return {
        isEmpty: () => path.includes("empty"),
        getSize: () => size,
      };
    },
  },
}));

describe("validatePetManifestJson", () => {
  it("accepts a valid manifest", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "spritesheet.webp",
      },
      "my-pet",
    );
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.value.sourceId).toBe("my-pet");
    expect(result.warnings).toEqual([]);
  });

  it("defaults old manifests to sprite version 1", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "spritesheet.webp",
      },
      "my-pet",
    );
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.value.spriteVersionNumber).toBe(1);
  });

  it("accepts Codex v2 manifests", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spriteVersionNumber: 2,
        spritesheetPath: "spritesheet.webp",
      },
      "my-pet",
    );
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.value.spriteVersionNumber).toBe(2);
  });

  it("rejects unsupported sprite versions", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spriteVersionNumber: 3,
        spritesheetPath: "spritesheet.webp",
      },
      "my-pet",
    );
    expect(result.valid).toBe(false);
    if (result.valid) return;
    expect(result.error).toContain("spriteVersionNumber");
  });

  it("rejects missing fields", () => {
    const result = validatePetManifestJson({}, "my-pet");
    expect(result.valid).toBe(false);
    if (result.valid) return;
    expect(result.error).toContain("id");
  });

  it("rejects invalid id characters", () => {
    const result = validatePetManifestJson(
      {
        id: "My Pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "spritesheet.webp",
      },
      "My Pet",
    );
    expect(result.valid).toBe(false);
    if (result.valid) return;
    expect(result.error).toContain("id");
  });

  it("warns when folder name differs from id", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "spritesheet.webp",
      },
      "different-folder",
    );
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.warnings.length).toBeGreaterThan(0);
  });

  it("rejects absolute spritesheet path", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "/etc/passwd",
      },
      "my-pet",
    );
    expect(result.valid).toBe(false);
    if (result.valid) return;
    expect(result.error).toContain("绝对路径");
  });

  it("rejects escaping spritesheet path", () => {
    const result = validatePetManifestJson(
      {
        id: "my-pet",
        displayName: "My Pet",
        description: "A short sentence.",
        spritesheetPath: "../spritesheet.webp",
      },
      "my-pet",
    );
    expect(result.valid).toBe(false);
    if (result.valid) return;
    expect(result.error).toContain("逃逸");
  });
});

describe("sanitizePetId", () => {
  it("normalizes ids to lowercase hyphenated", () => {
    expect(sanitizePetId("My Pet!")).toBe("my-pet");
    expect(sanitizePetId("  hello___world  ")).toBe("hello-world");
  });
});
