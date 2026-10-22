import { spawn, type ChildProcess } from "node:child_process";
import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import { beforeEach, expect, it, vi } from "vitest";
import { runCliCommand } from "./manager.js";

vi.mock("node:child_process", async (importOriginal) => ({
  ...await importOriginal<typeof import("node:child_process")>(),
  spawn: vi.fn(),
}));

beforeEach(() => vi.mocked(spawn).mockReset());

function fakeChild() {
  const child = Object.assign(new EventEmitter(), {
    stdout: new PassThrough(),
    stderr: new PassThrough(),
    kill: vi.fn(),
  });
  vi.mocked(spawn).mockReturnValue(child as unknown as ChildProcess);
  return child;
}

it("waits for stdio close before returning version JSON emitted around process exit", async () => {
  const child = fakeChild();
  const pending = runCliCommand("fixture", ["version"], "/tmp", 1_000);
  child.stdout.write('{"version":');
  child.emit("exit", 0);
  await Promise.resolve();
  // Node can deliver pipe data after exit; only close proves it is drained.
  child.stdout.write('"v1.0.32"}\n');
  child.stderr.write("diagnostic tail\n");
  child.emit("close", 0);
  expect(await pending).toEqual({
    exitCode: 0,
    stdout: '{"version":"v1.0.32"}\n',
    stderr: "diagnostic tail\n",
  });
});

it("retains a failing exit code and its final stderr", async () => {
  const child = fakeChild();
  const pending = runCliCommand("fixture", [], "/tmp", 1_000);
  child.emit("exit", 7);
  await Promise.resolve();
  child.stderr.write("failed command\n");
  child.emit("close", 7);
  expect(await pending).toMatchObject({ exitCode: 7, stderr: "failed command\n" });
});

it("propagates spawn errors without waiting for an exit event", async () => {
  const child = fakeChild();
  const pending = runCliCommand("missing", [], "/tmp", 1_000);
  const error = Object.assign(new Error("spawn failed"), { code: "ENOENT" });
  child.emit("error", error);
  await expect(pending).rejects.toBe(error);
});
