export type ShellResult = {
  exitCode: number;
  timedOut: boolean;
  durationMs: number;
  stdout: string;
  stderr: string;
  truncated: boolean;
};

export function parseShellResult(content: string): ShellResult | undefined {
  const trimmed = content.trim();
  if (!trimmed || !trimmed.startsWith("{")) {
    return undefined;
  }
  try {
    const parsed = JSON.parse(trimmed);
    if (
      parsed &&
      typeof parsed === "object" &&
      typeof parsed.exit_code === "number" &&
      typeof parsed.stdout === "string" &&
      typeof parsed.stderr === "string"
    ) {
      return {
        exitCode: parsed.exit_code,
        timedOut: Boolean(parsed.timed_out),
        durationMs: typeof parsed.duration_ms === "number" ? parsed.duration_ms : 0,
        stdout: parsed.stdout,
        stderr: parsed.stderr,
        truncated: Boolean(parsed.truncated),
      };
    }
  } catch {
    // Ignore non-JSON or malformed shell results.
  }
  return undefined;
}

export function parseShellCommand(args?: string): string | undefined {
  if (!args) {
    return undefined;
  }
  try {
    const parsed = JSON.parse(args);
    if (parsed && typeof parsed === "object" && typeof parsed.command === "string") {
      return parsed.command;
    }
  } catch {
    // Fall back to returning the raw args string.
  }
  return args.trim() || undefined;
}

export function shellResultStatus(result: ShellResult): "done" | "error" {
  return result.exitCode === 0 && !result.timedOut ? "done" : "error";
}

export function shellStatusFromContent(
  toolCallName: string | undefined,
  content: string | undefined,
): "done" | "error" | undefined {
  if (toolCallName !== "shell" || !content) {
    return undefined;
  }
  const result = parseShellResult(content);
  return result ? shellResultStatus(result) : undefined;
}

export function formatShellSummary(result: ShellResult): string {
  const stdoutLength = result.stdout.length;
  const stderrLength = result.stderr.length;
  const parts = [`exit ${result.exitCode}`, `${result.durationMs}ms`];
  if (stdoutLength > 0) {
    parts.push(`${humanSize(stdoutLength)} stdout`);
  }
  if (stderrLength > 0) {
    parts.push(`${humanSize(stderrLength)} stderr`);
  }
  if (result.truncated) {
    parts.push("truncated");
  }
  return parts.join(" · ");
}

export function shellPreview(stdout: string, lines = 2): string {
  return stdout.split("\n").slice(0, lines).join("\n");
}

export function shellActionTitle(command?: string): string {
  if (!command) {
    return "Ran shell command";
  }
  const head = command.split("\n")[0].trim();
  return head.length > 60 ? `${head.slice(0, 60)}...` : head;
}

function humanSize(chars: number): string {
  if (chars < 1024) {
    return `${chars} B`;
  }
  if (chars < 1024 * 1024) {
    return `${(chars / 1024).toFixed(chars < 10240 ? 1 : 0)} KB`;
  }
  return `${(chars / (1024 * 1024)).toFixed(chars < 10485760 ? 1 : 0)} MB`;
}
