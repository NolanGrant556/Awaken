import { spawnSync } from "node:child_process";
import { basename } from "node:path";

export function parsePsRows(output) {
  return String(output || "")
    .split("\n")
    .map((line) => line.trim().match(/^(\d+)\s+(.*)$/))
    .filter(Boolean)
    .map((match) => ({ pid: Number(match[1]), command: match[2].trim() }));
}

export function isPackagedAwakenCommand(command, platform = process.platform) {
  const normalized = String(command || "").trim().replaceAll("\\", "/");
  if (!normalized) {
    return false;
  }

  if (platform === "darwin") {
    return normalized.endsWith("/Awaken.app/Contents/MacOS/Awaken");
  }
  if (platform === "win32") {
    return basename(normalized).toLowerCase() === "awaken.exe";
  }
  return normalized.endsWith("/Awaken");
}

export function isUsableDevDesktopWindow(window) {
  if (!window || (window.name && window.name !== "Awaken Desktop")) {
    return false;
  }
  return (
    window.width >= 1000 &&
    window.height >= 700 &&
    window.x + window.width > 200 &&
    window.y + window.height > 200
  );
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });
  return {
    ok: result.status === 0,
    stdout: result.stdout?.trim() ?? "",
    stderr: result.stderr?.trim() ?? "",
    error: result.error,
  };
}

export function listPackagedAwakenProcesses(options = {}) {
  const platform = options.platform ?? process.platform;
  const runCommand = options.runCommand ?? run;

  if (platform === "win32") {
    const script = [
      "$ErrorActionPreference = 'Stop'",
      "$items = Get-CimInstance Win32_Process -Filter \"Name = 'Awaken.exe'\" | Select-Object ProcessId,ExecutablePath",
      "if ($null -eq $items) { '[]' } else { @($items) | ConvertTo-Json -Compress }",
    ].join("; ");
    const result = runCommand("powershell.exe", [
      "-NoLogo",
      "-NoProfile",
      "-NonInteractive",
      "-Command",
      script,
    ]);
    if (!result.ok) {
      throw new Error(
        `无法检查 Windows Awaken 实例：${result.stderr || result.error?.message || "PowerShell failed"}`,
      );
    }
    const parsed = JSON.parse(result.stdout || "[]");
    return (Array.isArray(parsed) ? parsed : [parsed])
      .map((item) => ({
        pid: Number(item?.ProcessId),
        command: String(item?.ExecutablePath || "Awaken.exe"),
      }))
      .filter((item) => Number.isInteger(item.pid) && item.pid > 0);
  }

  const result = runCommand("ps", ["-axo", "pid=,comm="]);
  if (!result.ok) {
    throw new Error(
      `无法检查 Awaken 实例：${result.stderr || result.error?.message || "ps failed"}`,
    );
  }
  return parsePsRows(result.stdout).filter((row) =>
    isPackagedAwakenCommand(row.command, platform),
  );
}
