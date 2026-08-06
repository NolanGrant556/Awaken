import { describe, expect, it, vi } from "vitest";
import { NetworkRouteManager, parseElectronProxyResult } from "./network-route-manager.js";

describe("NetworkRouteManager", () => {
  it("parses Chromium proxy directives without accepting unsupported values", () => {
    expect(parseElectronProxyResult("PROXY 127.0.0.1:7897; SOCKS5 127.0.0.1:1080; DIRECT")).toEqual({
      proxies: ["http://127.0.0.1:7897/", "socks5://127.0.0.1:1080"],
      direct: true,
    });
    expect(parseElectronProxyResult("QUIC proxy.example:443")).toEqual({ proxies: [], direct: true });
  });

  it.each(["darwin", "win32"] as const)(
    "removes an unavailable inherited loopback proxy on %s",
    async (platform) => {
      const manager = new NetworkRouteManager({
        platform,
        environment: {
          PATH: platform === "win32" ? "C:\\Windows\\System32" : "/usr/bin",
          HTTP_PROXY: "http://127.0.0.1:7897",
          HTTPS_PROXY: "http://127.0.0.1:7897",
        },
        resolveProxy: async () => "DIRECT",
        probeEndpoint: async () => false,
        watchdogIntervalMs: 0,
      });
      await manager.initialize();
      const environment = manager.childEnvironment();
      expect(environment.HTTP_PROXY).toBeUndefined();
      expect(environment.HTTPS_PROXY).toBeUndefined();
      expect(environment.NO_PROXY).toContain("127.0.0.1");
      expect(environment.PATH).toBeDefined();
    },
  );

  it("restores the inherited proxy when its listener returns", async () => {
    let reachable = false;
    const changed = vi.fn();
    const manager = new NetworkRouteManager({
      platform: "darwin",
      environment: { HTTPS_PROXY: "http://127.0.0.1:7897" },
      resolveProxy: async () => "DIRECT",
      probeEndpoint: async () => reachable,
      watchdogIntervalMs: 0,
      onSnapshotChanged: changed,
    });
    await manager.initialize();
    expect(manager.childEnvironment().HTTPS_PROXY).toBeUndefined();
    reachable = true;
    const snapshot = await manager.refresh();
    expect(manager.childEnvironment().HTTPS_PROXY).toBe("http://127.0.0.1:7897/");
    expect(snapshot.generation).toBe(2);
    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("publishes semantic system proxy changes for hot Core synchronization", async () => {
    let resolved = "DIRECT";
    const changed = vi.fn();
    const manager = new NetworkRouteManager({
      platform: "win32",
      resolveProxy: async () => resolved,
      probeEndpoint: async () => true,
      watchdogIntervalMs: 0,
      onSnapshotChanged: changed,
    });
    await manager.initialize();
    resolved = "PROXY 127.0.0.1:7890; DIRECT";
    const snapshot = await manager.refresh();
    expect(snapshot.https.proxies).toEqual(["http://127.0.0.1:7890/"]);
    expect(snapshot.generation).toBe(2);
    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("keeps proxy-only policy fail closed when no proxy is configured", async () => {
    const manager = new NetworkRouteManager({
      platform: "win32",
      environment: { AWAKEN_PROXY_POLICY: "proxy" },
      resolveProxy: async () => "DIRECT",
      watchdogIntervalMs: 0,
    });
    await manager.initialize();
    expect(manager.childEnvironment().HTTPS_PROXY).toBe("http://127.0.0.1:1");
  });

  it("refreshes before a network operation and coalesces adjacent operations", async () => {
    let nowMs = 1_000;
    const resolveProxy = vi.fn(async () => "DIRECT");
    const forceReloadProxyConfig = vi.fn(async () => undefined);
    const manager = new NetworkRouteManager({
      platform: "darwin",
      resolveProxy,
      forceReloadProxyConfig,
      watchdogIntervalMs: 0,
      operationCoalesceMs: 500,
      now: () => new Date(nowMs),
    });
    await manager.initialize();

    nowMs += 100;
    await manager.prepareForNetworkOperation("model-run");
    expect(forceReloadProxyConfig).not.toHaveBeenCalled();

    nowMs += 501;
    await manager.prepareForNetworkOperation("mcp-connect");
    expect(forceReloadProxyConfig).toHaveBeenCalledTimes(1);
    expect(resolveProxy).toHaveBeenCalledTimes(4);
  });

  it("reacts to a native system signal without waiting for the watchdog", async () => {
    vi.useFakeTimers();
    try {
      let resolved = "DIRECT";
      const changed = vi.fn();
      const forceReloadProxyConfig = vi.fn(async () => undefined);
      const manager = new NetworkRouteManager({
        platform: "win32",
        resolveProxy: async () => resolved,
        forceReloadProxyConfig,
        watchdogIntervalMs: 0,
        signalDebounceMs: 25,
        onSnapshotChanged: changed,
      });
      await manager.initialize();
      resolved = "PROXY 127.0.0.1:7890; DIRECT";

      manager.notifySystemChange("native-proxy-change");
      await vi.advanceTimersByTimeAsync(25);
      await manager.refresh();

      expect(forceReloadProxyConfig).toHaveBeenCalledTimes(1);
      expect(manager.currentSnapshot().https.proxies).toEqual(["http://127.0.0.1:7890/"]);
      expect(changed).toHaveBeenCalledTimes(2);
      manager.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it("uses a 60-second watchdog instead of a 3-second polling loop", async () => {
    vi.useFakeTimers();
    try {
      const resolveProxy = vi.fn(async () => "DIRECT");
      const manager = new NetworkRouteManager({
        platform: "darwin",
        resolveProxy,
      });
      await manager.initialize();
      expect(resolveProxy).toHaveBeenCalledTimes(2);

      await vi.advanceTimersByTimeAsync(59_999);
      expect(resolveProxy).toHaveBeenCalledTimes(2);
      await vi.advanceTimersByTimeAsync(1);
      expect(resolveProxy).toHaveBeenCalledTimes(4);
      manager.dispose();
    } finally {
      vi.useRealTimers();
    }
  });
});
