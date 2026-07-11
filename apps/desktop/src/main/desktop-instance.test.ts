import { describe, expect, it } from "vitest";
import {
  desktopCredentialApplicationName,
  desktopInstanceApplicationId,
  desktopInstanceIdentity,
  desktopUserDataPathSegments,
} from "./desktop-instance.js";

describe("desktop instance identity", () => {
  it("freezes the application name used by macOS Safe Storage", () => {
    expect(desktopCredentialApplicationName).toBe("Awaken");
  });

  it("uses one stable application identity for development and packaged launches", () => {
    expect(desktopInstanceIdentity(true)).toEqual({
      schemaVersion: 1,
      applicationId: desktopInstanceApplicationId,
      launchMode: "development",
    });
    expect(desktopInstanceIdentity(false)).toEqual({
      schemaVersion: 1,
      applicationId: desktopInstanceApplicationId,
      launchMode: "packaged",
    });
  });

  it("freezes the cross-platform userData namespace used by encrypted storage", () => {
    expect(desktopUserDataPathSegments).toEqual(["@awaken", "desktop"]);
  });
});
