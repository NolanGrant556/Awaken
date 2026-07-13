import { describe, expect, it } from "vitest";
import {
  desktopInstanceIdentity,
} from "./desktop-instance.js";

describe("desktop instance identity", () => {
  it("isolates development and packaged identity, credentials, and userData", () => {
    expect(desktopInstanceIdentity(true)).toEqual({
      schemaVersion: 1,
      applicationId: "com.awaken.desktop.dev",
      applicationName: "Awaken Dev",
      userDataPathSegments: ["@awaken-dev", "desktop"],
      launchMode: "development",
    });
    expect(desktopInstanceIdentity(false)).toEqual({
      schemaVersion: 1,
      applicationId: "com.awaken.desktop",
      applicationName: "Awaken",
      userDataPathSegments: ["@awaken", "desktop"],
      launchMode: "packaged",
    });
  });
});
