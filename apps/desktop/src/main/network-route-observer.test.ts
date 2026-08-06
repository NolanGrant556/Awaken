import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import type { ChildProcess } from "node:child_process";
import { describe, expect, it, vi } from "vitest";
import { isProxyChangeLine, NetworkRouteObserver } from "./network-route-observer.js";

function fakeChild(): ChildProcess {
  const child = new EventEmitter() as ChildProcess;
  Object.assign(child, {
    stdin: new PassThrough(),
    stdout: new PassThrough(),
    stderr: new PassThrough(),
    killed: false,
    kill: vi.fn(() => true),
  });
  return child;
}

describe("NetworkRouteObserver", () => {
  it("recognizes only proxy change notifications", () => {
    expect(isProxyChangeLine("darwin", "changedKey[0] = State:/Network/Global/Proxies")).toBe(true);
    expect(isProxyChangeLine("darwin", "changedKey[0] = State:/Network/Global/IPv4")).toBe(false);
    expect(isProxyChangeLine("win32", "proxy-change")).toBe(true);
    expect(isProxyChangeLine("win32", "registry-error")).toBe(false);
  });

  it("subscribes through scutil and forwards macOS proxy changes", () => {
    const child = fakeChild();
    const spawnProcess = vi.fn(() => child);
    const onChange = vi.fn();
    const observer = new NetworkRouteObserver({
      platform: "darwin",
      spawnProcess,
      onChange,
    });
    observer.start();

    expect(spawnProcess).toHaveBeenCalledWith("/usr/sbin/scutil", [], expect.objectContaining({ windowsHide: true }));
    (child.stdout as PassThrough).write("notification callback:\n  changedKey[0] = State:/Network/Global/Proxies\n");
    expect(onChange).toHaveBeenCalledWith("native-proxy-change");
    observer.dispose();
    expect(child.kill).toHaveBeenCalled();
  });

  it("uses the packaged Windows registry observer", () => {
    const child = fakeChild();
    const spawnProcess = vi.fn(() => child);
    const onChange = vi.fn();
    const observer = new NetworkRouteObserver({
      platform: "win32",
      windowsHelperPath: "C:\\Awaken\\awaken-network-watch.exe",
      spawnProcess,
      onChange,
    });
    observer.start();
    (child.stdout as PassThrough).write("proxy-change\r\n");

    expect(spawnProcess).toHaveBeenCalledWith(
      "C:\\Awaken\\awaken-network-watch.exe",
      [],
      expect.any(Object),
    );
    expect(onChange).toHaveBeenCalledTimes(1);
    observer.dispose();
  });
});
