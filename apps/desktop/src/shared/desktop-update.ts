// Desktop 应用更新的共享契约：状态机快照、错误码、receipt 与 IPC channel。
// 设计权威：[状态、命令与必须保持的规则](docs/01_桌面工作台/应用生命周期与更新/实现方案.md#状态命令与必须保持的规则)、[Shared / Preload IPC](docs/01_桌面工作台/应用生命周期与更新/实现方案.md#shared--preload-ipc)。
// Main 是状态唯一 owner；Renderer 只投影本文件定义的快照，不自行比较版本或选择资产。

export type DesktopUpdatePhase =
  | "disabled"
  | "idle"
  | "checking"
  | "current"
  | "available"
  | "downloading"
  | "downloaded"
  | "installing"
  | "error";

export type DesktopUpdateTarget = "darwin-arm64" | "darwin-x64" | "win32-x64";

export type DesktopUpdateErrorCode =
  | "network_unavailable"
  | "rate_limited"
  | "metadata_invalid"
  | "unsupported_platform"
  | "artifact_missing"
  | "download_failed"
  | "insufficient_space"
  | "checksum_mismatch"
  | "signature_invalid"
  | "active_runs"
  | "shutdown_failed"
  | "install_launch_failed";

export type DesktopUpdateOperationStage = "check" | "download" | "install";

export type DesktopUpdateSnapshot = {
  operationId: string;
  phase: DesktopUpdatePhase;
  currentVersion: string;
  availableVersion?: string;
  target?: DesktopUpdateTarget;
  releaseName?: string;
  releaseDate?: string;
  percent?: number;
  transferredBytes?: number;
  totalBytes?: number;
  bytesPerSecond?: number;
  errorCode?: DesktopUpdateErrorCode;
  errorMessage?: string;
  retryFrom?: "check" | "download";
  checkedAt?: string;
};

/**
 * 已下载更新的提示 receipt，保存在 Electron userData 下。
 * 它不是"已验证"凭证：不存 artifact path、URL、checksum 或任何凭据，
 * 跨进程恢复时只用于说明上次看到过哪个版本。
 */
export type DesktopUpdateReceipt = {
  schemaVersion: 1;
  version: string;
  target: DesktopUpdateTarget;
  releaseTag: string;
  downloadedAt: string;
};

/** Core `GET /v1/runtime/activity` 的只读响应（route 契约由 09 协议域拥有）。 */
export type DesktopRuntimeActivity = {
  active_run_count: number;
  cancelling_run_count: number;
  observed_at: string;
};

export function isDesktopRuntimeActivity(value: unknown): value is DesktopRuntimeActivity {
  if (!value || typeof value !== "object") return false;
  const activity = value as Partial<DesktopRuntimeActivity>;
  if (
    !Number.isInteger(activity.active_run_count) ||
    !Number.isInteger(activity.cancelling_run_count) ||
    (activity.active_run_count ?? -1) < 0 ||
    (activity.cancelling_run_count ?? -1) < 0 ||
    (activity.cancelling_run_count ?? 0) > (activity.active_run_count ?? 0)
  ) {
    return false;
  }
  if (typeof activity.observed_at !== "string" || activity.observed_at.length === 0) {
    return false;
  }
  return Number.isFinite(Date.parse(activity.observed_at));
}

export const DESKTOP_UPDATE_CHANNELS = {
  getState: "awaken:desktop-update:get-state",
  check: "awaken:desktop-update:check",
  download: "awaken:desktop-update:download",
  restartAndInstall: "awaken:desktop-update:restart-and-install",
  stateChanged: "awaken:desktop-update:state-changed",
} as const;

const STRICT_SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

export function isStableSemver(version: string): boolean {
  return STRICT_SEMVER_PATTERN.test(version);
}

/**
 * 严格 SemVer X.Y.Z 比较（stable 链路不消费 prerelease）。
 * 返回正数表示 a > b，0 表示相等，负数表示 a < b；任一非法时返回 null。
 */
export function compareStableSemver(a: string, b: string): number | null {
  if (!isStableSemver(a) || !isStableSemver(b)) {
    return null;
  }
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) {
    if (pa[index] !== pb[index]) {
      return pa[index] - pb[index];
    }
  }
  return 0;
}

/** 首发支持范围：macOS arm64/x64 与 Windows x64；其余平台/架构返回 null（禁用更新器）。 */
export function resolveDesktopUpdateTarget(
  platform: string,
  arch: string,
): DesktopUpdateTarget | null {
  if (platform === "darwin" && arch === "arm64") return "darwin-arm64";
  if (platform === "darwin" && arch === "x64") return "darwin-x64";
  if (platform === "win32" && arch === "x64") return "win32-x64";
  return null;
}

export function isStableDesktopUpdateIdentity(input: {
  packaged: boolean;
  devServer: boolean;
  channel: unknown;
  version: string;
  platform: string;
  arch: string;
}): boolean {
  return (
    input.packaged &&
    !input.devServer &&
    input.channel === "stable" &&
    isStableSemver(input.version) &&
    resolveDesktopUpdateTarget(input.platform, input.arch) !== null
  );
}

export type DesktopUpdateReceiptVerdict = "keep" | "discard";

/**
 * 启动对账（设计契约 [已下载更新的恢复与再验证](docs/01_桌面工作台/应用生命周期与更新/实现方案.md#已下载更新的恢复与再验证) 第 1 步）：receipt 不可解析、target 不一致或
 * version 不高于当前版本时都必须丢弃。receipt 只描述"见过哪个版本"，
 * 不恢复 downloaded 状态，也不据此触碰 updater 缓存目录。
 */
export function evaluateDesktopUpdateReceipt(
  raw: string,
  currentVersion: string,
  currentTarget: DesktopUpdateTarget,
): DesktopUpdateReceiptVerdict {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return "discard";
  }
  const receipt = parsed as Partial<DesktopUpdateReceipt> | null;
  if (!receipt || receipt.schemaVersion !== 1) {
    return "discard";
  }
  if (typeof receipt.version !== "string" || !isStableSemver(receipt.version)) {
    return "discard";
  }
  if (receipt.target !== currentTarget) {
    return "discard";
  }
  const order = compareStableSemver(receipt.version, currentVersion);
  if (order === null || order <= 0) {
    return "discard";
  }
  return "keep";
}

const NETWORK_ERROR_PATTERN =
  /ERR_INTERNET_DISCONNECTED|ERR_NETWORK|ERR_CONNECTION|ENOTFOUND|ETIMEDOUT|ECONNREFUSED|ECONNRESET|EAI_AGAIN|net::|fetch failed|socket hang up/i;

/** 按阶段和类型归类底层错误，Renderer 不依赖英文错误字符串判断产品文案。 */
export function classifyDesktopUpdateError(
  stage: DesktopUpdateOperationStage,
  error: unknown,
): DesktopUpdateErrorCode {
  const message = error instanceof Error ? error.message : String(error ?? "");
  if (/rate limit|403 Forbidden \(rate/i.test(message)) {
    return "rate_limited";
  }
  if (/ENOSPC|no space left/i.test(message)) {
    return "insufficient_space";
  }
  if (/sha512|checksum|digests do not match/i.test(message)) {
    return "checksum_mismatch";
  }
  if (/signature|signing|not signed|codesign/i.test(message)) {
    return "signature_invalid";
  }
  if (/404|artifact.*(missing|not found)|file.*not found/i.test(message)) {
    return "artifact_missing";
  }
  if (/latest(-mac)?\.yml|cannot parse|update info|metadata/i.test(message)) {
    return "metadata_invalid";
  }
  if (NETWORK_ERROR_PATTERN.test(message)) {
    return "network_unavailable";
  }
  switch (stage) {
    case "check":
      return "metadata_invalid";
    case "download":
      return "download_failed";
    case "install":
      return "install_launch_failed";
  }
}

/** 侧栏只在可操作状态显示更新入口（设计契约 [`aw-sidebar-profile` 更新入口](docs/01_桌面工作台/应用生命周期与更新/实现方案.md#aw-sidebar-profile-更新入口)）。 */
export function isDesktopUpdateActionablePhase(phase: DesktopUpdatePhase): boolean {
  return (
    phase === "available" ||
    phase === "downloading" ||
    phase === "downloaded" ||
    phase === "error"
  );
}

export function disabledDesktopUpdateSnapshot(currentVersion: string): DesktopUpdateSnapshot {
  return {
    operationId: "disabled",
    phase: "disabled",
    currentVersion,
  };
}
