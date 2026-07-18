#!/usr/bin/env node
import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { existsSync } from "node:fs";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath, pathToFileURL } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const designSystemDir = join(rootDir, "Awaken Design System");
const fixtureMainPath = join(rootDir, "scripts", "mermaid-electron-fixture-main.cjs");
const viteEntryPath = join(rootDir, "node_modules", "vite", "bin", "vite.js");
const loopbackHost = "127.0.0.1";
const defaultTimeoutMs = 60_000;

export const MERMAID_ELECTRON_FIXTURE = Object.freeze({
  route: "/#/components/awaken-assistant-message",
  wideStateLabel: "Mermaid 滚动容器",
  largeStateLabel: "大型 Mermaid 缩放",
});

function usage() {
  console.log(`Run the Mermaid native-pointer regression in an isolated Electron window.

Usage:
  npm run test:mermaid-electron
  npm run test:mermaid-electron -- --artifacts /tmp/awaken-mermaid-e2e

Options:
  --artifacts <dir>   Preserve screenshots and result.json in this directory.
                      Defaults to a new directory under the system temp folder.
  --timeout-ms <ms>   Startup and interaction timeout. Defaults to ${defaultTimeoutMs}.
  --help              Show this help.

The regression uses the maintained AwakenAssistantMessage Gallery state as a
fixture, launches a separate Electron main process with temporary userData,
and connects to a loopback-only CDP port. It never opens a real business thread.
`);
}

export function parseArgs(argv) {
  const args = {
    artifactsDir: null,
    timeoutMs: defaultTimeoutMs,
    help: false,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--help" || arg === "-h") {
      args.help = true;
      continue;
    }
    if (arg === "--artifacts") {
      args.artifactsDir = argv[index + 1] ?? "";
      index += 1;
      continue;
    }
    if (arg === "--timeout-ms") {
      args.timeoutMs = Number(argv[index + 1] ?? Number.NaN);
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  if (args.artifactsDir !== null && !args.artifactsDir.trim()) {
    throw new Error("--artifacts must not be empty.");
  }
  if (!Number.isFinite(args.timeoutMs) || args.timeoutMs < 5_000) {
    throw new Error("--timeout-ms must be a number of at least 5000.");
  }
  return args;
}

export function assertLoopbackHttpOrigin(value) {
  const url = new URL(value);
  if (url.protocol !== "http:" || url.hostname !== loopbackHost) {
    throw new Error(`Expected a loopback HTTP URL, received: ${value}`);
  }
  return url;
}

export function parseTranslate(transform) {
  const match = String(transform).match(
    /^translate\((-?\d+(?:\.\d+)?)px,\s*(-?\d+(?:\.\d+)?)px\)$/,
  );
  if (!match) {
    throw new Error(`Unexpected Mermaid pan transform: ${transform}`);
  }
  return { x: Number(match[1]), y: Number(match[2]) };
}

function electronExecutable() {
  if (process.platform === "darwin") {
    return join(rootDir, "node_modules", "electron", "dist", "Electron.app", "Contents", "MacOS", "Electron");
  }
  if (process.platform === "win32") {
    return join(rootDir, "node_modules", "electron", "dist", "electron.exe");
  }
  return join(rootDir, "node_modules", "electron", "dist", "electron");
}

async function freeLoopbackPort() {
  const server = createServer();
  await new Promise((resolveListen, rejectListen) => {
    server.once("error", rejectListen);
    server.listen(0, loopbackHost, resolveListen);
  });
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : null;
  await new Promise((resolveClose, rejectClose) => {
    server.close((error) => (error ? rejectClose(error) : resolveClose()));
  });
  if (!port) {
    throw new Error("Could not allocate a loopback port.");
  }
  return port;
}

function trackedChild(command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: rootDir,
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
    detached: process.platform !== "win32",
    ...options,
  });
  const output = [];
  const capture = (chunk) => {
    output.push(String(chunk));
    if (output.length > 80) {
      output.shift();
    }
  };
  child.stdout?.on("data", capture);
  child.stderr?.on("data", capture);
  return { child, output };
}

function childOutput(processHandle) {
  return processHandle.output.join("").trim();
}

async function terminateChild(processHandle) {
  if (!processHandle?.child || processHandle.child.exitCode !== null) {
    return;
  }

  const { child } = processHandle;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
      stdio: "ignore",
      windowsHide: true,
    });
    return;
  }

  try {
    process.kill(-child.pid, "SIGTERM");
  } catch (error) {
    if (error?.code !== "ESRCH") {
      throw error;
    }
    return;
  }

  await Promise.race([once(child, "exit"), delay(3_000)]);
  if (child.exitCode === null) {
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch (error) {
      if (error?.code !== "ESRCH") {
        throw error;
      }
    }
  }
}

async function waitForHttp(url, processHandle, timeoutMs) {
  assertLoopbackHttpOrigin(url);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (processHandle.child.exitCode !== null) {
      throw new Error(
        `Fixture server exited before it became ready.\n${childOutput(processHandle)}`,
      );
    }
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Startup polling owns this transient failure.
    }
    await delay(200);
  }
  throw new Error(`Timed out waiting for fixture server: ${url}`);
}

async function waitForCdpTarget(debugOrigin, targetUrl, processHandle, timeoutMs) {
  assertLoopbackHttpOrigin(debugOrigin);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (processHandle.child.exitCode !== null) {
      throw new Error(
        `Electron exited before the CDP target became ready.\n${childOutput(processHandle)}`,
      );
    }
    try {
      const response = await fetch(`${debugOrigin}/json/list`);
      if (response.ok) {
        const targets = await response.json();
        const page = targets.find(
          (target) => target.type === "page" && target.url === targetUrl,
        );
        if (page?.webSocketDebuggerUrl) {
          return page;
        }
      }
    } catch {
      // Electron and DevTools start independently; keep polling.
    }
    await delay(200);
  }
  throw new Error(`Timed out waiting for Electron CDP target: ${targetUrl}`);
}

class CdpClient {
  constructor(webSocketDebuggerUrl, timeoutMs) {
    this.nextId = 1;
    this.pending = new Map();
    this.timeoutMs = timeoutMs;
    this.socket = new WebSocket(webSocketDebuggerUrl);
    this.opened = false;
    this.socket.addEventListener("open", () => {
      this.opened = true;
    });
    this.socket.addEventListener("message", (event) => {
      const message = JSON.parse(String(event.data));
      if (!message.id || !this.pending.has(message.id)) {
        return;
      }
      const pending = this.pending.get(message.id);
      this.pending.delete(message.id);
      clearTimeout(pending.timer);
      if (message.error) {
        pending.reject(new Error(message.error.message));
      } else {
        pending.resolve(message.result);
      }
    });
    this.socket.addEventListener("close", () => {
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer);
        pending.reject(new Error("CDP socket closed before the command completed."));
      }
      this.pending.clear();
    });
  }

  async ready() {
    if (this.opened || this.socket.readyState === WebSocket.OPEN) {
      return;
    }
    await Promise.race([
      once(this.socket, "open"),
      delay(this.timeoutMs, undefined, { ref: false }).then(() => {
        throw new Error("Timed out connecting to the Electron CDP socket.");
      }),
    ]);
  }

  send(method, params = {}) {
    const id = this.nextId;
    this.nextId += 1;
    return new Promise((resolveCommand, rejectCommand) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        rejectCommand(new Error(`CDP command timed out: ${method}`));
      }, this.timeoutMs);
      timer.unref();
      this.pending.set(id, { resolve: resolveCommand, reject: rejectCommand, timer });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }

  async evaluate(expression) {
    const response = await this.send("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    });
    if (response.exceptionDetails) {
      throw new Error(
        `Electron evaluation failed: ${response.exceptionDetails.exception?.description ?? response.exceptionDetails.text}`,
      );
    }
    return response.result.value;
  }

  close() {
    this.socket.close();
  }
}

function expressionFor(fn, ...args) {
  return `(${fn.toString()})(${args.map((arg) => JSON.stringify(arg)).join(",")})`;
}

async function evaluateFunction(cdp, fn, ...args) {
  return cdp.evaluate(expressionFor(fn, ...args));
}

async function pollValue(cdp, fn, args, predicate, label, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastValue;
  while (Date.now() < deadline) {
    lastValue = await evaluateFunction(cdp, fn, ...args);
    if (predicate(lastValue)) {
      return lastValue;
    }
    await delay(100);
  }
  throw new Error(`${label} did not become ready. Last value: ${JSON.stringify(lastValue)}`);
}

function stateFigureSnapshot(stateLabel) {
  const article = [...document.querySelectorAll(".ads-component-demo-state")].find(
    (candidate) => candidate.querySelector("h3")?.textContent?.trim() === stateLabel,
  );
  if (!article) {
    return { ready: false, reason: "state-not-found" };
  }
  const figure = article.querySelector(".assistant-mermaid-diagram");
  const svg = figure?.querySelector(".assistant-mermaid-diagram-svg svg");
  if (!figure || !svg) {
    return { ready: false, reason: "diagram-not-ready" };
  }
  const style = getComputedStyle(figure);
  const figureRect = figure.getBoundingClientRect();
  const svgRect = svg.getBoundingClientRect();
  return {
    ready: true,
    overflowX: style.overflowX,
    overflowY: style.overflowY,
    clientWidth: figure.clientWidth,
    clientHeight: figure.clientHeight,
    scrollWidth: figure.scrollWidth,
    scrollHeight: figure.scrollHeight,
    figureWidth: figureRect.width,
    svgWidth: svgRect.width,
    hasVisibleInnerScrollbar:
      ((style.overflowX === "auto" || style.overflowX === "scroll") &&
        figure.scrollWidth > figure.clientWidth + 1) ||
      ((style.overflowY === "auto" || style.overflowY === "scroll") &&
        figure.scrollHeight > figure.clientHeight + 1),
  };
}

function pageIdentity() {
  return {
    title: document.title,
    url: location.href,
    readyState: document.readyState,
    brand: document.querySelector(".ads-gallery-brand span")?.textContent?.trim() ?? "",
    userAgent: navigator.userAgent,
    viewport: { width: innerWidth, height: innerHeight, devicePixelRatio },
  };
}

function stateButtonRect(stateLabel, ariaLabel) {
  const article = [...document.querySelectorAll(".ads-component-demo-state")].find(
    (candidate) => candidate.querySelector("h3")?.textContent?.trim() === stateLabel,
  );
  if (!article) {
    return null;
  }
  article.scrollIntoView({ block: "center", inline: "nearest" });
  const button = [...article.querySelectorAll("button")].find(
    (candidate) => candidate.getAttribute("aria-label") === ariaLabel,
  );
  if (!button) {
    return null;
  }
  const rect = button.getBoundingClientRect();
  return {
    x: rect.x + rect.width / 2,
    y: rect.y + rect.height / 2,
    width: rect.width,
    height: rect.height,
  };
}

function globalButtonRect(ariaLabel) {
  const button = [...document.querySelectorAll("button")].find(
    (candidate) => candidate.getAttribute("aria-label") === ariaLabel,
  );
  if (!button) {
    return null;
  }
  const rect = button.getBoundingClientRect();
  return {
    x: rect.x + rect.width / 2,
    y: rect.y + rect.height / 2,
    width: rect.width,
    height: rect.height,
  };
}

function expandedSnapshot() {
  const scroll = document.querySelector(".assistant-mermaid-expanded-scroll");
  const frame = document.querySelector(".assistant-mermaid-expanded-zoom-frame");
  const zoom = document.querySelector(".assistant-mermaid-zoom-value");
  if (!scroll || !frame || !zoom) {
    return { open: false };
  }
  const rect = scroll.getBoundingClientRect();
  const frameRect = frame.getBoundingClientRect();
  const svgRect = frame.querySelector("svg")?.getBoundingClientRect() ?? null;
  const visibleSvgWidth = svgRect
    ? Math.max(0, Math.min(rect.right, svgRect.right) - Math.max(rect.left, svgRect.left))
    : 0;
  const visibleSvgHeight = svgRect
    ? Math.max(0, Math.min(rect.bottom, svgRect.bottom) - Math.max(rect.top, svgRect.top))
    : 0;
  return {
    open: true,
    transform: frame.style.transform,
    zoomLabel: zoom.textContent?.trim() ?? "",
    isPanning: scroll.classList.contains("is-panning"),
    x: rect.x + rect.width / 2,
    y: rect.y + rect.height / 2,
    width: rect.width,
    height: rect.height,
    scrollLeft: scroll.scrollLeft,
    scrollTop: scroll.scrollTop,
    frameRect: {
      x: frameRect.x,
      y: frameRect.y,
      width: frameRect.width,
      height: frameRect.height,
    },
    svgRect: svgRect
      ? { x: svgRect.x, y: svgRect.y, width: svgRect.width, height: svgRect.height }
      : null,
    visibleSvgWidth,
    visibleSvgHeight,
  };
}

function setExpandedZoom(value) {
  const slider = document.querySelector(".assistant-mermaid-zoom-range");
  if (!(slider instanceof HTMLInputElement)) {
    return false;
  }
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  if (!setter) {
    return false;
  }
  setter.call(slider, String(value));
  slider.dispatchEvent(new Event("input", { bubbles: true, composed: true }));
  return true;
}

async function nativeClick(cdp, rect) {
  if (!rect || rect.width <= 0 || rect.height <= 0) {
    throw new Error(`Cannot click an unavailable Electron element: ${JSON.stringify(rect)}`);
  }
  const x = Math.round(rect.x);
  const y = Math.round(rect.y);
  await cdp.send("Input.dispatchMouseEvent", {
    type: "mousePressed",
    x,
    y,
    button: "left",
    buttons: 1,
    clickCount: 1,
    pointerType: "mouse",
  });
  await cdp.send("Input.dispatchMouseEvent", {
    type: "mouseReleased",
    x,
    y,
    button: "left",
    buttons: 0,
    clickCount: 1,
    pointerType: "mouse",
  });
}

async function nativeDrag(cdp, start, delta, steps = 12) {
  const startX = Math.round(start.x);
  const startY = Math.round(start.y);
  await cdp.send("Input.dispatchMouseEvent", {
    type: "mousePressed",
    x: startX,
    y: startY,
    button: "left",
    buttons: 1,
    clickCount: 1,
    pointerType: "mouse",
  });
  for (let step = 1; step <= steps; step += 1) {
    await cdp.send("Input.dispatchMouseEvent", {
      type: "mouseMoved",
      x: Math.round(start.x + (delta.x * step) / steps),
      y: Math.round(start.y + (delta.y * step) / steps),
      button: "left",
      buttons: 1,
      pointerType: "mouse",
    });
    await delay(20);
  }
  await cdp.send("Input.dispatchMouseEvent", {
    type: "mouseReleased",
    x: Math.round(start.x + delta.x),
    y: Math.round(start.y + delta.y),
    button: "left",
    buttons: 0,
    clickCount: 1,
    pointerType: "mouse",
  });
}

function assertNear(actual, expected, label, tolerance = 1) {
  if (Math.abs(actual - expected) > tolerance) {
    throw new Error(`${label}: expected ${expected}, received ${actual}`);
  }
}

async function captureScreenshot(cdp, path) {
  const result = await cdp.send("Page.captureScreenshot", {
    format: "png",
    fromSurface: true,
  });
  await writeFile(path, Buffer.from(result.data, "base64"));
}

async function runInteraction(cdp, artifactsDir, targetUrl, timeoutMs) {
  const checks = [];
  await cdp.send("Runtime.enable");
  await cdp.send("Page.enable");
  await cdp.send("Page.bringToFront");

  const runtime = await pollValue(
    cdp,
    pageIdentity,
    [],
    (value) =>
      value?.url === targetUrl &&
      value.readyState === "complete" &&
      value.brand === "Awaken 设计系统",
    "Electron Gallery page identity",
    timeoutMs,
  );
  const frameTree = await cdp.send("Page.getFrameTree");
  const expectedFrameUrl = new URL(targetUrl);
  expectedFrameUrl.hash = "";
  if (frameTree.frameTree?.frame?.url !== expectedFrameUrl.href) {
    throw new Error(
      `Electron main frame URL mismatch: expected ${expectedFrameUrl.href}, received ${frameTree.frameTree?.frame?.url}`,
    );
  }
  checks.push({ name: "page-identity", status: "passed", evidence: runtime });

  const inline = await pollValue(
    cdp,
    stateFigureSnapshot,
    [MERMAID_ELECTRON_FIXTURE.wideStateLabel],
    (value) => value?.ready,
    "Wide Mermaid fixture",
    timeoutMs,
  );
  if (inline.overflowX !== "hidden" || inline.overflowY !== "hidden") {
    throw new Error(`Inline Mermaid overflow must be hidden: ${JSON.stringify(inline)}`);
  }
  if (inline.hasVisibleInnerScrollbar) {
    throw new Error(`Inline Mermaid exposed an inner scrollbar: ${JSON.stringify(inline)}`);
  }
  if (inline.svgWidth > inline.clientWidth + 1) {
    throw new Error(`Inline Mermaid SVG exceeded its card width: ${JSON.stringify(inline)}`);
  }
  checks.push({ name: "inline-no-double-scrollbar", status: "passed", evidence: inline });
  await captureScreenshot(cdp, join(artifactsDir, "01-inline.png"));

  await evaluateFunction(
    cdp,
    (stateLabel) => {
      const article = [...document.querySelectorAll(".ads-component-demo-state")].find(
        (candidate) => candidate.querySelector("h3")?.textContent?.trim() === stateLabel,
      );
      article?.scrollIntoView({ block: "center", inline: "nearest" });
    },
    MERMAID_ELECTRON_FIXTURE.largeStateLabel,
  );
  await delay(250);
  const expandRect = await evaluateFunction(
    cdp,
    stateButtonRect,
    MERMAID_ELECTRON_FIXTURE.largeStateLabel,
    "Expand diagram",
  );
  await nativeClick(cdp, expandRect);
  let expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.zoomLabel === "100%",
    "Expanded Mermaid dialog",
    timeoutMs,
  );
  checks.push({ name: "expand", status: "passed", evidence: expanded });

  const dragAt100 = { x: 180, y: 120 };
  await nativeDrag(cdp, expanded, dragAt100);
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.transform !== "translate(0px, 0px)",
    "100% Mermaid pan",
    timeoutMs,
  );
  const pan100 = parseTranslate(expanded.transform);
  assertNear(pan100.x, dragAt100.x, "100% pan x");
  assertNear(pan100.y, dragAt100.y, "100% pan y");
  if (expanded.visibleSvgWidth <= 1 || expanded.visibleSvgHeight <= 1) {
    throw new Error(`100% pan moved the Mermaid SVG out of view: ${JSON.stringify(expanded)}`);
  }
  checks.push({ name: "native-pan-100", status: "passed", evidence: { ...expanded, pan: pan100 } });
  await captureScreenshot(cdp, join(artifactsDir, "02-pan-100.png"));

  const resetRect = await evaluateFunction(cdp, globalButtonRect, "Reset view");
  await nativeClick(cdp, resetRect);
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.transform === "translate(0px, 0px)" && value.zoomLabel === "100%",
    "Mermaid reset after 100% pan",
    timeoutMs,
  );
  checks.push({ name: "reset-from-pan", status: "passed", evidence: expanded });

  const zoomChanged = await evaluateFunction(cdp, setExpandedZoom, 4);
  if (!zoomChanged) {
    throw new Error("Could not set the Mermaid zoom fixture to 400%.");
  }
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.zoomLabel === "400%",
    "Mermaid 400% zoom",
    timeoutMs,
  );

  const dragAt400 = { x: -180, y: -120 };
  await nativeDrag(cdp, expanded, dragAt400);
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.transform !== "translate(0px, 0px)",
    "400% Mermaid pan",
    timeoutMs,
  );
  const pan400 = parseTranslate(expanded.transform);
  assertNear(pan400.x, dragAt400.x, "400% pan x");
  assertNear(pan400.y, dragAt400.y, "400% pan y");
  if (expanded.visibleSvgWidth <= 1 || expanded.visibleSvgHeight <= 1) {
    throw new Error(`400% pan moved the Mermaid SVG out of view: ${JSON.stringify(expanded)}`);
  }
  checks.push({ name: "native-pan-400", status: "passed", evidence: { ...expanded, pan: pan400 } });
  await captureScreenshot(cdp, join(artifactsDir, "03-pan-400.png"));

  const resetAt400Rect = await evaluateFunction(cdp, globalButtonRect, "Reset view");
  await nativeClick(cdp, resetAt400Rect);
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.transform === "translate(0px, 0px)" && value.zoomLabel === "100%",
    "Mermaid reset from 400%",
    timeoutMs,
  );
  checks.push({ name: "reset-from-400", status: "passed", evidence: expanded });

  const closeRect = await evaluateFunction(cdp, globalButtonRect, "Close expanded diagram");
  await nativeClick(cdp, closeRect);
  await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open === false,
    "Mermaid dialog close",
    timeoutMs,
  );

  const reopenRect = await evaluateFunction(
    cdp,
    stateButtonRect,
    MERMAID_ELECTRON_FIXTURE.largeStateLabel,
    "Expand diagram",
  );
  await nativeClick(cdp, reopenRect);
  expanded = await pollValue(
    cdp,
    expandedSnapshot,
    [],
    (value) => value?.open && value.transform === "translate(0px, 0px)" && value.zoomLabel === "100%",
    "Reopened Mermaid dialog",
    timeoutMs,
  );
  checks.push({ name: "close-reopen-resets-view", status: "passed", evidence: expanded });
  await captureScreenshot(cdp, join(artifactsDir, "04-reopened.png"));

  const finalCloseRect = await evaluateFunction(cdp, globalButtonRect, "Close expanded diagram");
  await nativeClick(cdp, finalCloseRect);
  return { runtime, checks };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    usage();
    return;
  }
  if (typeof WebSocket === "undefined") {
    throw new Error("This regression requires a Node.js runtime with the WebSocket global.");
  }

  const electronPath = electronExecutable();
  for (const requiredPath of [electronPath, viteEntryPath, fixtureMainPath]) {
    if (!existsSync(requiredPath)) {
      throw new Error(`Required Mermaid regression dependency is missing: ${requiredPath}`);
    }
  }

  const artifactsDir = args.artifactsDir
    ? resolve(args.artifactsDir)
    : await mkdtemp(join(tmpdir(), "awaken-mermaid-electron-artifacts-"));
  await mkdir(artifactsDir, { recursive: true });
  const userDataDir = await mkdtemp(join(tmpdir(), "awaken-mermaid-electron-user-data-"));
  const rendererPort = await freeLoopbackPort();
  const cdpPort = await freeLoopbackPort();
  const fixtureOrigin = `http://${loopbackHost}:${rendererPort}`;
  const targetUrl = `${fixtureOrigin}${MERMAID_ELECTRON_FIXTURE.route}`;
  const debugOrigin = `http://${loopbackHost}:${cdpPort}`;
  assertLoopbackHttpOrigin(fixtureOrigin);
  assertLoopbackHttpOrigin(debugOrigin);

  let vite = null;
  let electron = null;
  let cdp = null;
  const startedAt = new Date().toISOString();
  let interaction = null;
  let failure = null;

  const cleanup = async () => {
    cdp?.close();
    await terminateChild(electron);
    await terminateChild(vite);
    await rm(userDataDir, { recursive: true, force: true });
  };

  const handleSignal = (signal) => {
    void cleanup().finally(() => {
      process.kill(process.pid, signal);
    });
  };
  process.once("SIGINT", handleSignal);
  process.once("SIGTERM", handleSignal);

  try {
    vite = trackedChild(
      process.execPath,
      [viteEntryPath, "--host", loopbackHost, "--port", String(rendererPort), "--strictPort"],
      { cwd: designSystemDir },
    );
    await waitForHttp(fixtureOrigin, vite, args.timeoutMs);

    const electronEnv = { ...process.env };
    delete electronEnv.ELECTRON_RUN_AS_NODE;
    electronEnv.AWAKEN_MERMAID_E2E_URL = targetUrl;
    electronEnv.AWAKEN_MERMAID_E2E_USER_DATA = userDataDir;
    electron = trackedChild(
      electronPath,
      [
        `--remote-debugging-address=${loopbackHost}`,
        `--remote-debugging-port=${cdpPort}`,
        fixtureMainPath,
      ],
      { env: electronEnv },
    );

    const target = await waitForCdpTarget(debugOrigin, targetUrl, electron, args.timeoutMs);
    cdp = new CdpClient(target.webSocketDebuggerUrl, args.timeoutMs);
    await cdp.ready();
    interaction = await runInteraction(cdp, artifactsDir, targetUrl, args.timeoutMs);
  } catch (error) {
    failure = error instanceof Error ? error : new Error(String(error));
    if (cdp) {
      try {
        await captureScreenshot(cdp, join(artifactsDir, "failure.png"));
      } catch {
        // Preserve the primary failure; screenshot collection is best effort.
      }
    }
  } finally {
    process.removeListener("SIGINT", handleSignal);
    process.removeListener("SIGTERM", handleSignal);
    await cleanup();
  }

  const result = {
    schemaVersion: 1,
    status: failure ? "failed" : "passed",
    startedAt,
    completedAt: new Date().toISOString(),
    fixture: {
      kind: "design-system-gallery",
      scope: "component-level real Electron native-pointer regression",
      route: MERMAID_ELECTRON_FIXTURE.route,
      wideStateLabel: MERMAID_ELECTRON_FIXTURE.wideStateLabel,
      largeStateLabel: MERMAID_ELECTRON_FIXTURE.largeStateLabel,
      personalThreadUsed: false,
      notClaimed: ["Workbench thread/replay acceptance", "runtime streaming acceptance"],
    },
    isolation: {
      cdpHost: loopbackHost,
      rendererHost: loopbackHost,
      temporaryUserDataRemoved: true,
    },
    interaction,
    failure: failure
      ? {
          message: failure.message,
          stack: failure.stack,
          viteOutput: childOutput(vite),
          electronOutput: childOutput(electron),
        }
      : null,
  };
  const resultPath = join(artifactsDir, "result.json");
  await writeFile(resultPath, `${JSON.stringify(result, null, 2)}\n`);

  console.log("Mermaid Electron regression");
  console.log(`- status: ${result.status}`);
  console.log(`- fixture: ${MERMAID_ELECTRON_FIXTURE.route}`);
  console.log(`- userData: isolated temporary directory removed`);
  console.log(`- artifacts: ${artifactsDir}`);
  console.log(`- result: ${resultPath}`);
  if (failure) {
    throw failure;
  }
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : null;
if (invokedPath === import.meta.url) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.stack ?? error.message : error);
    process.exitCode = 1;
  });
}
