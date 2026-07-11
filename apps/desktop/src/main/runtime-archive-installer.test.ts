import type { ChildProcess } from "node:child_process";
import { EventEmitter } from "node:events";
import fs from "node:fs/promises";
import { dirname, join } from "node:path";
import { PassThrough } from "node:stream";
import { describe, expect, it } from "vitest";
import { installBundledPrimaryRuntimeArchive } from "./runtime-archive-installer.js";

async function writeBundledFixture(
  root: string,
  options: {
    active?: string;
    platform?: "darwin" | "windows";
    arch?: string;
  } = {},
) {
  const platform = options.platform ?? "darwin";
  const arch = options.arch ?? "arm64";
  const active = options.active ?? `0.3.0/${platform}-${arch}/rev-1`;
  const archiveDirectory = join(root, "runtime-archive");
  await fs.mkdir(join(root, "runtime-installer"), { recursive: true });
  await fs.mkdir(archiveDirectory, { recursive: true });
  const archive = "primary-runtime.zip";
  await fs.writeFile(join(archiveDirectory, archive), "archive");
  await fs.writeFile(
    join(archiveDirectory, `${archive}.manifest.json`),
    JSON.stringify({
      schemaVersion: 1,
      runtimeName: "primary-runtime",
      active,
      platform,
      arch,
      archive,
      rootDirectory: "rev-1",
      integrity: {
        algorithm: "sha256",
        scope: "archive",
        trustBoundary: "packaged-host",
      },
      ...(platform === "darwin"
        ? {
            signing: {
              mode: "developer-id",
              teamIdentifier: "AWAKENTEAM",
              bundleIdentifier: "com.awaken.runtime.libreoffice-renderer",
            },
          }
        : {}),
    }),
  );
  await fs.writeFile(
    join(root, "runtime-installer", "install-primary-runtime-archive.mjs"),
    "// fixture",
  );
  return active;
}

describe("installBundledPrimaryRuntimeArchive", () => {
  it("skips development and builds without a bundled release archive", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    try {
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: false,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "/Electron",
        }),
      ).resolves.toEqual({ status: "not_packaged" });
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "/Electron",
          platform: "darwin",
          arch: "arm64",
          hostTeamIdentifier: "AWAKENTEAM",
        }),
      ).resolves.toEqual({ status: "not_bundled" });
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("runs the signed-resource installer once and verifies activation", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    const userData = join(root, "user-data");
    const active = await writeBundledFixture(root);
    const calls: string[][] = [];
    try {
      const result = await installBundledPrimaryRuntimeArchive({
        packaged: true,
        resourcesPath: root,
        userDataDir: userData,
        electronExecutable: "/Electron",
        platform: "darwin",
        arch: "arm64",
        hostTeamIdentifier: "AWAKENTEAM",
        spawnInstaller: (command: string, args: string[]) => {
          calls.push([command, ...args]);
          const child = new EventEmitter() as ChildProcess;
          child.stdout = new PassThrough();
          child.stderr = new PassThrough();
          queueMicrotask(async () => {
            const current = join(userData, "runtimes", "primary-runtime", "current.json");
            await fs.mkdir(dirname(current), { recursive: true });
            await fs.writeFile(current, JSON.stringify({ active }));
            child.emit("exit", 0, null);
          });
          return child;
        },
      });
      expect(result).toEqual({ status: "installed", active });
      expect(calls).toHaveLength(1);
      expect(calls[0][0]).toBe("/Electron");

      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: userData,
          electronExecutable: "/Electron",
          platform: "darwin",
          arch: "arm64",
          hostTeamIdentifier: "AWAKENTEAM",
          spawnInstaller: (() => {
            throw new Error("installer must not run twice");
          }) as never,
        }),
      ).resolves.toEqual({ status: "already_active", active });
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("installs a Windows archive without passing macOS signing arguments", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    const userData = join(root, "user-data");
    const active = await writeBundledFixture(root, {
      platform: "windows",
      arch: "x64",
    });
    const calls: string[][] = [];
    try {
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: userData,
          electronExecutable: "C:\\Program Files\\Awaken\\Awaken.exe",
          platform: "win32",
          arch: "x64",
          spawnInstaller: (command: string, args: string[]) => {
            calls.push([command, ...args]);
            const child = new EventEmitter() as ChildProcess;
            child.stdout = new PassThrough();
            child.stderr = new PassThrough();
            queueMicrotask(async () => {
              const current = join(userData, "runtimes", "primary-runtime", "current.json");
              await fs.mkdir(dirname(current), { recursive: true });
              await fs.writeFile(current, JSON.stringify({ active }));
              child.emit("exit", 0, null);
            });
            return child;
          },
        }),
      ).resolves.toEqual({ status: "installed", active });
      expect(calls).toHaveLength(1);
      expect(calls[0]).not.toContain("--expected-team-id");
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("rejects a Windows archive that does not declare SHA-256 host-package integrity", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    try {
      await writeBundledFixture(root, { platform: "windows", arch: "x64" });
      const manifestPath = join(
        root,
        "runtime-archive",
        "primary-runtime.zip.manifest.json",
      );
      const manifest = JSON.parse(await fs.readFile(manifestPath, "utf8"));
      delete manifest.integrity;
      await fs.writeFile(manifestPath, JSON.stringify(manifest));
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "Awaken.exe",
          platform: "win32",
          arch: "x64",
        }),
      ).rejects.toThrow("digest contract");
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("rejects ambiguous and traversal-bearing manifests", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    try {
      await writeBundledFixture(root);
      await fs.writeFile(
        join(root, "runtime-archive", "second.manifest.json"),
        JSON.stringify({
          schemaVersion: 1,
          runtimeName: "primary-runtime",
          active: "other",
          platform: "darwin",
          arch: "arm64",
          archive: "other.zip",
        }),
      );
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "/Electron",
          platform: "darwin",
          arch: "arm64",
          hostTeamIdentifier: "AWAKENTEAM",
        }),
      ).rejects.toThrow("exactly one");
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("rejects unsafe active and extracted root paths before spawning the installer", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    try {
      await writeBundledFixture(root, { platform: "windows", arch: "x64" });
      const manifestPath = join(
        root,
        "runtime-archive",
        "primary-runtime.zip.manifest.json",
      );
      const manifest = JSON.parse(await fs.readFile(manifestPath, "utf8"));
      manifest.rootDirectory = "../outside";
      await fs.writeFile(manifestPath, JSON.stringify(manifest));
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "Awaken.exe",
          platform: "win32",
          arch: "x64",
          spawnInstaller: (() => {
            throw new Error("unsafe manifest must be rejected before spawn");
          }) as never,
        }),
      ).rejects.toThrow("unsafe path");
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });

  it("rejects a runtime signed by a different team than the packaged host", async () => {
    const root = await fs.mkdtemp(join(process.cwd(), ".runtime-archive-test-"));
    try {
      await writeBundledFixture(root);
      await expect(
        installBundledPrimaryRuntimeArchive({
          packaged: true,
          resourcesPath: root,
          userDataDir: root,
          electronExecutable: "/Electron",
          platform: "darwin",
          arch: "arm64",
          hostTeamIdentifier: "DIFFERENTTEAM",
        }),
      ).rejects.toThrow("same trusted team");
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });
});
