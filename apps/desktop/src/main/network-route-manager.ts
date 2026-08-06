import { createConnection } from "node:net";

export type NetworkRoutePolicy = "auto" | "proxy" | "direct";

export type NetworkProxyRoute = {
  proxies: string[];
  direct: boolean;
};

export type NetworkRouteSnapshot = {
  schema_version: 1;
  generation: number;
  policy: NetworkRoutePolicy;
  http: NetworkProxyRoute;
  https: NetworkProxyRoute;
  observed_at: string;
};

type ProxyEndpointProbe = (host: string, port: number) => Promise<boolean>;

type NetworkRouteManagerOptions = {
  platform: NodeJS.Platform;
  resolveProxy: (url: string) => Promise<string>;
  forceReloadProxyConfig?: () => Promise<void>;
  environment?: NodeJS.ProcessEnv;
  probeEndpoint?: ProxyEndpointProbe;
  watchdogIntervalMs?: number;
  operationCoalesceMs?: number;
  signalDebounceMs?: number;
  now?: () => Date;
  onSnapshotChanged?: (snapshot: NetworkRouteSnapshot) => void | Promise<void>;
  onError?: (error: unknown) => void;
};

const proxyEnvironmentKeys = [
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
  "NO_PROXY",
  "http_proxy",
  "https_proxy",
  "all_proxy",
  "no_proxy",
] as const;
const probeUrls = {
  http: "http://www.example.com/",
  https: "https://www.example.com/",
} as const;
const failClosedProxy = "http://127.0.0.1:1";

export class NetworkRouteManager {
  private readonly environment: NodeJS.ProcessEnv;
  private readonly probeEndpoint: ProxyEndpointProbe;
  private readonly now: () => Date;
  private readonly policy: NetworkRoutePolicy;
  private snapshot: NetworkRouteSnapshot;
  private endpointAvailability = new Map<string, boolean>();
  private watchdogTimer?: ReturnType<typeof setInterval>;
  private signalTimer?: ReturnType<typeof setTimeout>;
  private initialized = false;
  private dirty = true;
  private lastRefreshAtMs = Number.NEGATIVE_INFINITY;
  private reloadRequested = false;
  private refreshPromise?: Promise<NetworkRouteSnapshot>;

  constructor(private readonly options: NetworkRouteManagerOptions) {
    this.environment = { ...(options.environment ?? process.env) };
    this.probeEndpoint = options.probeEndpoint ?? probeTcpEndpoint;
    this.now = options.now ?? (() => new Date());
    this.policy = parsePolicy(this.environment.AWAKEN_PROXY_POLICY);
    this.snapshot = {
      schema_version: 1,
      generation: 0,
      policy: this.policy,
      http: { proxies: [], direct: true },
      https: { proxies: [], direct: true },
      observed_at: this.now().toISOString(),
    };
  }

  async initialize(): Promise<NetworkRouteSnapshot> {
    const snapshot = await this.refresh("initialize");
    const interval = this.options.watchdogIntervalMs ?? 60_000;
    if (interval > 0 && !this.watchdogTimer) {
      this.watchdogTimer = setInterval(() => {
        void this.refresh("watchdog", true).catch((error) => this.options.onError?.(error));
      }, interval);
      this.watchdogTimer.unref?.();
    }
    return snapshot;
  }

  async refresh(_reason = "manual", forceReloadProxyConfig = false): Promise<NetworkRouteSnapshot> {
    if (forceReloadProxyConfig) this.reloadRequested = true;
    if (this.refreshPromise) {
      const snapshot = await this.refreshPromise;
      return this.reloadRequested ? this.refresh(_reason) : snapshot;
    }
    this.refreshPromise = this.refreshUntilSettled().finally(() => {
      this.refreshPromise = undefined;
    });
    return this.refreshPromise;
  }

  async prepareForNetworkOperation(reason: string): Promise<NetworkRouteSnapshot> {
    const elapsed = this.now().getTime() - this.lastRefreshAtMs;
    const coalesceMs = Math.max(0, this.options.operationCoalesceMs ?? 500);
    if (!this.dirty && elapsed >= 0 && elapsed <= coalesceMs) {
      return this.currentSnapshot();
    }
    try {
      return await this.refresh(`operation:${reason}`, true);
    } catch (error) {
      // Proxy discovery must not make an operation fail before the Core or CLI
      // can use the last known-good route. The watchdog and the next operation
      // will retry discovery.
      this.options.onError?.(error);
      return this.currentSnapshot();
    }
  }

  notifySystemChange(reason: string): void {
    this.scheduleSignalRefresh(`system:${reason}`);
  }

  notifyTransportFailure(surface: string): void {
    this.scheduleSignalRefresh(`transport:${surface}`, 0);
  }

  currentSnapshot(): NetworkRouteSnapshot {
    return structuredClone(this.snapshot);
  }

  runtimeEnvironment(): NodeJS.ProcessEnv {
    return {
      AWAKEN_PROXY_POLICY: this.policy,
      AWAKEN_SYSTEM_PROXY_SNAPSHOT: JSON.stringify(this.snapshot),
    };
  }

  childEnvironment(source: NodeJS.ProcessEnv = this.environment): NodeJS.ProcessEnv {
    const result: NodeJS.ProcessEnv = { ...source };
    for (const key of proxyEnvironmentKeys) delete result[key];

    const httpProxy = this.effectiveProxy("http");
    const httpsProxy = this.effectiveProxy("https");
    if (httpProxy) {
      result.HTTP_PROXY = httpProxy;
      result.http_proxy = httpProxy;
    }
    if (httpsProxy) {
      result.HTTPS_PROXY = httpsProxy;
      result.https_proxy = httpsProxy;
    }
    if (httpProxy && httpProxy === httpsProxy) {
      result.ALL_PROXY = httpProxy;
      result.all_proxy = httpProxy;
    }

    const noProxy = mergeNoProxy(source.NO_PROXY, source.no_proxy);
    result.NO_PROXY = noProxy;
    result.no_proxy = noProxy;
    return result;
  }

  dispose(): void {
    if (this.watchdogTimer) clearInterval(this.watchdogTimer);
    if (this.signalTimer) clearTimeout(this.signalTimer);
    this.watchdogTimer = undefined;
    this.signalTimer = undefined;
  }

  private async refreshUntilSettled(): Promise<NetworkRouteSnapshot> {
    let snapshot = this.currentSnapshot();
    do {
      const reloadProxyConfig = this.reloadRequested;
      this.reloadRequested = false;
      snapshot = await this.refreshInternal(reloadProxyConfig);
    } while (this.reloadRequested);
    return snapshot;
  }

  private async refreshInternal(reloadProxyConfig: boolean): Promise<NetworkRouteSnapshot> {
    if (reloadProxyConfig) {
      try {
        await this.options.forceReloadProxyConfig?.();
      } catch (error) {
        this.options.onError?.(new NetworkRouteError(
          "NETWORK_PROXY_CONFIG_RELOAD_FAILED",
          error instanceof Error ? error.message : "系统代理配置重载失败",
        ));
      }
    }
    const [httpResult, httpsResult] = await Promise.allSettled([
      this.options.resolveProxy(probeUrls.http),
      this.options.resolveProxy(probeUrls.https),
    ]);
    if (httpResult.status === "rejected" && httpsResult.status === "rejected") {
      if (!this.initialized) {
        await this.refreshEndpointAvailability(this.snapshot.http, this.snapshot.https);
        this.initialized = true;
        this.snapshot = {
          ...this.snapshot,
          generation: 1,
          observed_at: this.now().toISOString(),
        };
        this.recordSuccessfulObservation();
        const fallback = this.currentSnapshot();
        await this.options.onSnapshotChanged?.(fallback);
        return fallback;
      }
      throw new NetworkRouteError("NETWORK_PROXY_RESOLUTION_FAILED", "系统代理解析失败");
    }

    const nextHTTP = httpResult.status === "fulfilled"
      ? parseElectronProxyResult(httpResult.value)
      : this.snapshot.http;
    const nextHTTPS = httpsResult.status === "fulfilled"
      ? parseElectronProxyResult(httpsResult.value)
      : this.snapshot.https;
    const endpointAvailabilityChanged = await this.refreshEndpointAvailability(nextHTTP, nextHTTPS);

    const semanticChanged = !this.initialized
      || !routesEqual(this.snapshot.http, nextHTTP)
      || !routesEqual(this.snapshot.https, nextHTTPS)
      || this.snapshot.policy !== this.policy
      || endpointAvailabilityChanged;
    if (!semanticChanged) {
      this.recordSuccessfulObservation();
      return this.currentSnapshot();
    }

    this.initialized = true;
    this.snapshot = {
      schema_version: 1,
      generation: Math.max(1, this.snapshot.generation + 1),
      policy: this.policy,
      http: nextHTTP,
      https: nextHTTPS,
      observed_at: this.now().toISOString(),
    };
    this.recordSuccessfulObservation();
    const snapshot = this.currentSnapshot();
    await this.options.onSnapshotChanged?.(snapshot);
    return snapshot;
  }

  private recordSuccessfulObservation(): void {
    this.dirty = false;
    this.lastRefreshAtMs = this.now().getTime();
  }

  private scheduleSignalRefresh(reason: string, debounceMs = this.options.signalDebounceMs ?? 150): void {
    this.dirty = true;
    this.reloadRequested = true;
    if (this.signalTimer) return;
    this.signalTimer = setTimeout(() => {
      this.signalTimer = undefined;
      void this.refresh(reason, true).catch((error) => this.options.onError?.(error));
    }, Math.max(0, debounceMs));
    this.signalTimer.unref?.();
  }

  private async refreshEndpointAvailability(...routes: NetworkProxyRoute[]): Promise<boolean> {
    const proxyUrls = new Set<string>();
    for (const value of environmentProxyCandidates(this.environment)) proxyUrls.add(value);
    for (const route of routes) for (const value of route.proxies) proxyUrls.add(value);

    const observations = await Promise.all([...proxyUrls].map(async (value) => {
      const parsed = safeProxyURL(value, true);
      if (!parsed || !isLoopbackHost(parsed.hostname)) return [value, true] as const;
      const port = proxyPort(parsed);
      if (!port) return [value, false] as const;
      return [value, await this.probeEndpoint(parsed.hostname, port)] as const;
    }));
    const nextAvailability = new Map(observations);
    const changed = nextAvailability.size !== this.endpointAvailability.size
      || [...nextAvailability].some(([key, value]) => this.endpointAvailability.get(key) !== value);
    this.endpointAvailability = nextAvailability;
    return changed;
  }

  private effectiveProxy(scheme: "http" | "https"): string | undefined {
    if (this.policy === "direct") return undefined;
    const environmentProxy = environmentProxyForScheme(this.environment, scheme);
    if (environmentProxy) {
      const parsed = safeProxyURL(environmentProxy, true);
      if (
        this.policy === "proxy"
        || !parsed
        || !isLoopbackHost(parsed.hostname)
        || this.endpointAvailability.get(environmentProxy) !== false
      ) {
        return environmentProxy;
      }
    }

    const route = scheme === "https" ? this.snapshot.https : this.snapshot.http;
    for (const candidate of route.proxies) {
      const parsed = safeProxyURL(candidate, false);
      if (!parsed || !isLoopbackHost(parsed.hostname) || this.endpointAvailability.get(candidate) !== false) {
        return candidate;
      }
    }
    if (this.policy === "proxy") {
      return environmentProxy ?? route.proxies[0] ?? failClosedProxy;
    }
    return undefined;
  }
}

export class NetworkRouteError extends Error {
  constructor(readonly code: string, message: string) {
    super(message);
  }
}

export function parseElectronProxyResult(raw: string): NetworkProxyRoute {
  const proxies: string[] = [];
  let direct = false;
  for (const rawDirective of raw.split(";")) {
    const directive = rawDirective.trim();
    if (!directive) continue;
    if (directive.toUpperCase() === "DIRECT") {
      direct = true;
      continue;
    }
    const match = directive.match(/^(PROXY|HTTPS|SOCKS|SOCKS5)\s+(.+)$/i);
    if (!match) continue;
    const scheme = match[1].toUpperCase() === "HTTPS"
      ? "https"
      : match[1].toUpperCase().startsWith("SOCKS")
        ? "socks5"
        : "http";
    const candidate = safeProxyURL(`${scheme}://${match[2].trim()}`, false);
    if (candidate && !proxies.includes(candidate.toString())) proxies.push(candidate.toString());
  }
  if (proxies.length === 0 && !direct) direct = true;
  return { proxies, direct };
}

function parsePolicy(raw: string | undefined): NetworkRoutePolicy {
  switch (raw?.trim().toLowerCase()) {
    case "proxy": return "proxy";
    case "direct": return "direct";
    default: return "auto";
  }
}

function environmentProxyCandidates(environment: NodeJS.ProcessEnv): string[] {
  const candidates = new Set<string>();
  for (const scheme of ["http", "https"] as const) {
    const candidate = environmentProxyForScheme(environment, scheme);
    if (candidate) candidates.add(candidate);
  }
  return [...candidates];
}

function environmentProxyForScheme(
  environment: NodeJS.ProcessEnv,
  scheme: "http" | "https",
): string | undefined {
  const values = scheme === "https"
    ? [environment.HTTPS_PROXY, environment.https_proxy, environment.ALL_PROXY, environment.all_proxy]
    : [environment.HTTP_PROXY, environment.http_proxy, environment.ALL_PROXY, environment.all_proxy];
  for (const value of values) {
    const parsed = safeProxyURL(value, true);
    if (parsed) return parsed.toString();
  }
  return undefined;
}

function safeProxyURL(raw: string | undefined, allowCredentials: boolean): URL | undefined {
  if (!raw?.trim()) return undefined;
  try {
    const candidate = raw.includes("://") ? raw.trim() : `http://${raw.trim()}`;
    const parsed = new URL(candidate);
    if (!["http:", "https:", "socks5:", "socks5h:"].includes(parsed.protocol)) return undefined;
    if (!parsed.hostname || (!allowCredentials && (parsed.username || parsed.password))) return undefined;
    if ((parsed.pathname && parsed.pathname !== "/") || parsed.search || parsed.hash) return undefined;
    const port = proxyPort(parsed);
    if (!port) return undefined;
    parsed.pathname = "";
    return parsed;
  } catch {
    return undefined;
  }
}

function proxyPort(value: URL): number | undefined {
  const raw = value.port || (value.protocol === "http:" ? "80" : value.protocol === "https:" ? "443" : "");
  const port = Number(raw);
  return Number.isInteger(port) && port > 0 && port <= 65_535 ? port : undefined;
}

function isLoopbackHost(hostname: string): boolean {
  const normalized = hostname.replace(/^\[|\]$/g, "").toLowerCase();
  return normalized === "localhost"
    || normalized === "::1"
    || /^127(?:\.\d{1,3}){3}$/.test(normalized);
}

function mergeNoProxy(...values: Array<string | undefined>): string {
  const result: string[] = [];
  const seen = new Set<string>();
  for (const value of [...values, "localhost,127.0.0.1,::1"]) {
    for (const entry of value?.split(",") ?? []) {
      const normalized = entry.trim();
      const key = normalized.toLowerCase();
      if (!normalized || seen.has(key)) continue;
      seen.add(key);
      result.push(normalized);
    }
  }
  return result.join(",");
}

function routesEqual(left: NetworkProxyRoute, right: NetworkProxyRoute): boolean {
  return left.direct === right.direct
    && left.proxies.length === right.proxies.length
    && left.proxies.every((value, index) => value === right.proxies[index]);
}

function probeTcpEndpoint(host: string, port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = createConnection({ host, port });
    let settled = false;
    const finish = (reachable: boolean) => {
      if (settled) return;
      settled = true;
      socket.destroy();
      resolve(reachable);
    };
    socket.setTimeout(250);
    socket.once("connect", () => finish(true));
    socket.once("timeout", () => finish(false));
    socket.once("error", () => finish(false));
  });
}
