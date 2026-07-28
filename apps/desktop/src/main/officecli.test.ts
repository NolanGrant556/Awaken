import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  officeCliTargetName,
  resolveOfficeCliBinaryPath,
  resolveOfficeCliManifestPath,
} from "./officecli.js";

describe("OfficeCLI runtime paths", () => {
  it("resolves only the bundled current platform target in development", () => {
    const paths = {
      packaged: false,
      resourcesPath: "/package/resources",
      appPath: "/repo/apps/desktop",
      cachePath: "/cache",
      platform: "darwin" as const,
      arch: "arm64" as const,
    };
    expect(officeCliTargetName(paths.platform, paths.arch)).toBe("darwin-arm64");
    expect(resolveOfficeCliManifestPath(paths)).toBe(
      path.join(paths.appPath, "resources", "officecli-release.json"),
    );
    expect(resolveOfficeCliBinaryPath(paths)).toBe(
      path.join(paths.appPath, "resources", "bin", "darwin-arm64", "officecli"),
    );
  });

  it("resolves the packaged Windows executable without consulting PATH", () => {
    const binary = resolveOfficeCliBinaryPath({
      packaged: true,
      resourcesPath: "C:\\Program Files\\Awaken\\resources",
      appPath: "C:\\Program Files\\Awaken\\resources\\app.asar",
      cachePath: "C:\\cache",
      platform: "win32",
      arch: "x64",
    });
    expect(binary).toContain(path.join("bin", "win32-x64", "officecli.exe"));
  });
});
