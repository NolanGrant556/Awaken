import { describe, expect, it } from "vitest";
import {
  classifyDesktopUpdateError,
  compareStableSemver,
  evaluateDesktopUpdateReceipt,
  isDesktopRuntimeActivity,
  isStableDesktopUpdateIdentity,
  isDesktopUpdateActionablePhase,
  isStableSemver,
  resolveDesktopUpdateTarget,
} from "./desktop-update.js";

describe("isStableSemver / compareStableSemver", () => {
  it("接受严格 X.Y.Z，拒绝 0.0.0 之外的非 SemVer 与 prerelease", () => {
    expect(isStableSemver("0.1.0")).toBe(true);
    expect(isStableSemver("1.20.3")).toBe(true);
    expect(isStableSemver("1.2")).toBe(false);
    expect(isStableSemver("1.2.3-beta.1")).toBe(false);
    expect(isStableSemver("v1.2.3")).toBe(false);
    expect(isStableSemver("01.2.3")).toBe(false);
  });

  it("比较版本高低", () => {
    expect(compareStableSemver("0.2.0", "0.1.0")).toBeGreaterThan(0);
    expect(compareStableSemver("0.1.0", "0.2.0")).toBeLessThan(0);
    expect(compareStableSemver("1.0.0", "1.0.0")).toBe(0);
    expect(compareStableSemver("1.10.0", "1.9.9")).toBeGreaterThan(0);
    expect(compareStableSemver("1.0.0", "bad")).toBeNull();
  });
});

describe("resolveDesktopUpdateTarget", () => {
  it("首发范围：darwin arm64/x64 与 win32 x64", () => {
    expect(resolveDesktopUpdateTarget("darwin", "arm64")).toBe("darwin-arm64");
    expect(resolveDesktopUpdateTarget("darwin", "x64")).toBe("darwin-x64");
    expect(resolveDesktopUpdateTarget("win32", "x64")).toBe("win32-x64");
  });

  it("Windows arm64 与 Linux 禁用更新器", () => {
    expect(resolveDesktopUpdateTarget("win32", "arm64")).toBeNull();
    expect(resolveDesktopUpdateTarget("linux", "x64")).toBeNull();
  });
});

describe("stable updater identity / runtime activity", () => {
  it("只有显式 stable 的正式受支持安装包启用更新", () => {
    const base = {
      packaged: true,
      devServer: false,
      channel: "stable",
      version: "0.1.0",
      platform: "darwin",
      arch: "arm64",
    };
    expect(isStableDesktopUpdateIdentity(base)).toBe(true);
    expect(isStableDesktopUpdateIdentity({ ...base, channel: "disabled" })).toBe(false);
    expect(isStableDesktopUpdateIdentity({ ...base, packaged: false })).toBe(false);
    expect(isStableDesktopUpdateIdentity({ ...base, version: "0.2.0-beta.1" })).toBe(false);
    expect(isStableDesktopUpdateIdentity({ ...base, platform: "linux" })).toBe(false);
  });

  it("activity 必须是非负整数、cancelling 子集且时间可解析", () => {
    expect(
      isDesktopRuntimeActivity({
        active_run_count: 1,
        cancelling_run_count: 1,
        observed_at: "2026-08-18T00:00:00Z",
      }),
    ).toBe(true);
    expect(
      isDesktopRuntimeActivity({
        active_run_count: 0,
        cancelling_run_count: 1,
        observed_at: "2026-08-18T00:00:00Z",
      }),
    ).toBe(false);
    expect(
      isDesktopRuntimeActivity({
        active_run_count: -1,
        cancelling_run_count: 0,
        observed_at: "invalid",
      }),
    ).toBe(false);
  });
});

describe("evaluateDesktopUpdateReceipt", () => {
  const validReceipt = JSON.stringify({
    schemaVersion: 1,
    version: "0.2.0",
    target: "darwin-arm64",
    releaseTag: "v0.2.0",
    downloadedAt: "2026-08-18T00:00:00.000Z",
  });

  it("版本更高且 target 一致时保留", () => {
    expect(evaluateDesktopUpdateReceipt(validReceipt, "0.1.0", "darwin-arm64")).toBe("keep");
  });

  it("不可解析、target 不一致或版本不高于当前时丢弃", () => {
    expect(evaluateDesktopUpdateReceipt("not json", "0.1.0", "darwin-arm64")).toBe("discard");
    expect(evaluateDesktopUpdateReceipt(validReceipt, "0.1.0", "darwin-x64")).toBe("discard");
    expect(evaluateDesktopUpdateReceipt(validReceipt, "0.2.0", "darwin-arm64")).toBe("discard");
    expect(evaluateDesktopUpdateReceipt(validReceipt, "0.3.0", "darwin-arm64")).toBe("discard");
    expect(
      evaluateDesktopUpdateReceipt(JSON.stringify({ schemaVersion: 2 }), "0.1.0", "darwin-arm64"),
    ).toBe("discard");
  });
});

describe("classifyDesktopUpdateError", () => {
  it("按错误内容分类", () => {
    expect(classifyDesktopUpdateError("check", new Error("net::ERR_INTERNET_DISCONNECTED"))).toBe(
      "network_unavailable",
    );
    expect(classifyDesktopUpdateError("check", new Error("rate limit exceeded"))).toBe(
      "rate_limited",
    );
    expect(classifyDesktopUpdateError("download", new Error("sha512 checksum mismatch"))).toBe(
      "checksum_mismatch",
    );
    expect(classifyDesktopUpdateError("download", new Error("signature verification failed"))).toBe(
      "signature_invalid",
    );
    expect(classifyDesktopUpdateError("download", new Error("ENOSPC: no space left on device"))).toBe(
      "insufficient_space",
    );
    expect(classifyDesktopUpdateError("download", new Error("404 Not Found"))).toBe(
      "artifact_missing",
    );
    expect(classifyDesktopUpdateError("check", new Error("Cannot parse latest-mac.yml"))).toBe(
      "metadata_invalid",
    );
  });

  it("无法识别时按阶段回退", () => {
    expect(classifyDesktopUpdateError("download", new Error("boom"))).toBe("download_failed");
    expect(classifyDesktopUpdateError("install", new Error("boom"))).toBe("install_launch_failed");
    expect(classifyDesktopUpdateError("check", new Error("boom"))).toBe("metadata_invalid");
  });
});

describe("isDesktopUpdateActionablePhase", () => {
  it("只有 available/downloading/downloaded/error 显示侧栏入口", () => {
    expect(isDesktopUpdateActionablePhase("available")).toBe(true);
    expect(isDesktopUpdateActionablePhase("downloading")).toBe(true);
    expect(isDesktopUpdateActionablePhase("downloaded")).toBe(true);
    expect(isDesktopUpdateActionablePhase("error")).toBe(true);
    expect(isDesktopUpdateActionablePhase("disabled")).toBe(false);
    expect(isDesktopUpdateActionablePhase("idle")).toBe(false);
    expect(isDesktopUpdateActionablePhase("checking")).toBe(false);
    expect(isDesktopUpdateActionablePhase("current")).toBe(false);
    expect(isDesktopUpdateActionablePhase("installing")).toBe(false);
  });
});
