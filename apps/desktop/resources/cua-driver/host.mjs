// Reference CUA Host for Awaken computer.Driver PoC.
//
// This is a Node.js sidecar that loads @trycua/cua-driver, listens for JSON-RPC
// style commands on stdin, and writes results to stdout. It is intentionally
// minimal and only supports the subset of the SDK needed to prove the Go
// adapter can drive a real CUA runtime through a child process.
//
// The process exits on stdin EOF, SIGTERM, or an unrecoverable SDK error.

import {
  CuaDriver,
  CaptureScope,
  ActionTarget,
  ClickButton,
  ScrollDirection,
  ScrollBy,
  StartSessionInput,
  EndSessionInput,
  GetDesktopStateInput,
  GetSessionStateInput,
  ClickInput,
  DragInput,
  HotkeyInput,
  MoveCursorInput,
  PressKeyInput,
  ScrollInput,
  TypeTextInput,
  currentMacOsPermissionStatus,
  requestMacOsPermissions,
} from "@trycua/cua-driver";
import { randomUUID } from "node:crypto";

const SCOPE_MAP = {
  auto: CaptureScope.Auto,
  window: CaptureScope.Window,
  desktop: CaptureScope.Desktop,
};

const BUTTON_MAP = {
  left: ClickButton.Left,
  right: ClickButton.Right,
  middle: ClickButton.Middle,
};

const DIRECTION_MAP = {
  up: ScrollDirection.Up,
  down: ScrollDirection.Down,
  left: ScrollDirection.Left,
  right: ScrollDirection.Right,
};

const BY_MAP = {
  line: ScrollBy.Line,
  page: ScrollBy.Page,
};

function primaryDesktopTarget() {
  return ActionTarget.Desktop.new({ displayId: "primary" });
}

function exactWindowTarget(pid, windowID) {
  if (!Number.isSafeInteger(pid) || pid <= 0) {
    throw new Error("window target pid must be a positive integer");
  }
  if (!Number.isSafeInteger(windowID) || windowID <= 0) {
    throw new Error("window target windowID must be a positive safe integer");
  }
  return { kind: "window", pid, window_id: windowID };
}

function toolResultEnvelope(result) {
  return {
    text: result.text,
    images: result.images.map((img) => ({
      mime_type: img.mimeType,
      data_base64: img.dataBase64,
    })),
    structured_json: result.structuredJson,
    raw_json: result.rawJson,
    is_error: result.isError,
    error_code: result.errorCode,
    degraded: result.degraded,
  };
}

function parseStructuredJSON(result) {
  if (typeof result.structuredJson !== "string" || result.structuredJson === "") {
    return {};
  }
  try {
    return JSON.parse(result.structuredJson);
  } catch {
    return {};
  }
}

async function listOnScreenWindows() {
  let lastError = "unknown list_windows failure";
  for (let attempt = 1; attempt <= 2; attempt += 1) {
    try {
      const result = await windowDiscoveryDriver.callTool(
        "list_windows",
        JSON.stringify({ on_screen_only: true }),
      );
      if (result.isError) {
        lastError = result.text || result.errorCode || "CUA list_windows failed";
      } else {
        const parsed = parseStructuredJSON(result);
        if (Array.isArray(parsed.windows)) {
          return parsed.windows.map((window) => ({
            target_ref: `pid:${window.pid}/window:${window.window_id}`,
            pid: window.pid,
            app_name: window.app_name || "",
            window_id: String(window.window_id),
            window_title: window.title || "",
            minimized: Boolean(window.minimized),
            z_index: Number.isInteger(window.z_index) ? window.z_index : null,
          }));
        }
        lastError = "CUA list_windows omitted its windows array";
      }
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    if (attempt < 2) {
      if (isEndedSessionMessage(lastError)) {
        await replaceWindowDiscoveryDriver();
      }
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  throw new Error(`desktop window discovery failed after retry: ${lastError}`);
}

let driver;
let windowDiscoveryDriver;
let shuttingDown = false;
const sessions = new Map();

function isEndedSessionMessage(message) {
  const normalized = String(message || "").toLowerCase();
  return (
    normalized.includes("session") &&
    (normalized.includes("has ended") ||
      normalized.includes("already ended") ||
      normalized.includes("session ended"))
  );
}

async function replaceWindowDiscoveryDriver() {
  const expiredDriver = windowDiscoveryDriver;
  windowDiscoveryDriver = CuaDriver.create(undefined);
  if (expiredDriver) {
    try {
      await expiredDriver.shutdown();
    } catch (error) {
      log("warn", `expired window discovery driver shutdown error: ${error.message}`);
    }
    try {
      expiredDriver.uniffiDestroy();
    } catch (error) {
      log("warn", `expired window discovery driver destroy error: ${error.message}`);
    }
  }
}

function log(level, message) {
  // Keep stderr separate from stdout JSON-RPC traffic.
  console.error(`[host ${level}] ${message}`);
}

function sendResponse(id, result) {
  const line = JSON.stringify({ id, result });
  process.stdout.write(line + "\n");
}

function sendError(id, code, message, detail) {
  const payload = { code, message };
  if (detail !== undefined) {
    payload.detail = String(detail);
  }
  const line = JSON.stringify({ id, error: payload });
  process.stdout.write(line + "\n");
}

function captureScopeFromString(scope) {
  const mapped = SCOPE_MAP[scope];
  if (mapped === undefined) {
    throw new Error(`unknown capture scope: ${scope}`);
  }
  return mapped;
}

async function handleHealth() {
  const metadata = await driver.metadata();
  const permissions = currentMacOsPermissionStatus();
  return {
    healthy: driver.isAvailable(),
    version: metadata.driverVersion,
    protocol_version: 2,
    os: process.platform,
    tcc_screen_recording: Boolean(permissions.screenRecording),
    tcc_accessibility: Boolean(permissions.accessibility),
    message: "CUA embedded host",
  };
}

async function handleRequestPermissions() {
  if (process.platform === "darwin") {
    // TCC consent belongs to the process that captures the screen and injects
    // input. Request both permissions from this Host process instead of from
    // Electron Main so macOS grants the identity that actually uses CUA.
    requestMacOsPermissions();
  }
  return await handleHealth();
}

async function handleStartSession({ sessionID, captureScope, fenceToken, controlEpoch }) {
  if (!Number.isSafeInteger(fenceToken) || fenceToken <= 0) {
    throw new Error("fenceToken must be a positive integer");
  }
  if (!Number.isSafeInteger(controlEpoch) || controlEpoch <= 0) {
    throw new Error("controlEpoch must be a positive integer");
  }
  const input = StartSessionInput.new({
    session: sessionID,
    captureScope: captureScopeFromString(captureScope),
  });
  const output = await driver.startSession(input);
  sessions.set(sessionID, {
    state: "ready",
    fenceToken,
    controlEpoch,
    targetGeneration: 1,
    coordinateScaleX: 1,
    coordinateScaleY: 1,
    observations: new Map(),
    preparedActions: new Map(),
    actionResults: new Map(),
  });
  return {
    host_session_ref: output.state.session,
    capture_scope: String(captureScope),
    active: output.active,
    revived: output.revived,
  };
}

async function handleGetDesktopState({ sessionID }) {
  const record = requireSession(sessionID);
  if (record.state !== "ready") {
    throw new Error(`session ${sessionID} cannot capture while ${record.state}`);
  }
  const input = GetDesktopStateInput.new({ session: sessionID });
  const result = await driver.getDesktopState(input);
  const coordinateSpace = updateCoordinateScale(
    record,
    result.text,
    result.structuredJson,
  );
  const observationToken = randomUUID();
  record.observations.set(observationToken, {
    capturedAt: Date.now(),
    targetGeneration: record.targetGeneration,
    target: { kind: "desktop", displayId: "primary" },
  });
  return {
    ...toolResultEnvelope(result),
    observation_token: observationToken,
    coordinate_space: coordinateSpace,
    available_windows: await listOnScreenWindows(),
  };
}

async function handleGetWindowState({ sessionID, pid, windowID }) {
  const record = requireSession(sessionID);
  if (record.state !== "ready") {
    throw new Error(`session ${sessionID} cannot capture while ${record.state}`);
  }
  const target = exactWindowTarget(pid, windowID);
  const result = await driver.callTool(
    "get_window_state",
    JSON.stringify({
      session: sessionID,
      pid: target.pid,
      window_id: target.window_id,
      include_screenshot: true,
    }),
  );
  const parsed = parseStructuredJSON(result);
  const screenshotWidth = Number(parsed.screenshot_width) || 0;
  const screenshotHeight = Number(parsed.screenshot_height) || 0;
  record.coordinateScaleX = 1;
  record.coordinateScaleY = 1;
  const observationToken = randomUUID();
  record.observations.set(observationToken, {
    capturedAt: Date.now(),
    targetGeneration: record.targetGeneration,
    target,
  });
  return {
    ...toolResultEnvelope(result),
    observation_token: observationToken,
    coordinate_space: {
      unit: "window_pixel",
      width: screenshotWidth,
      height: screenshotHeight,
      source_screenshot_width: screenshotWidth,
      source_screenshot_height: screenshotHeight,
      scale_x: 1,
      scale_y: 1,
    },
    target: {
      pid: target.pid,
      window_id: String(target.window_id),
      coordinate_space: "window",
    },
  };
}

function updateCoordinateScale(record, text, structuredJson) {
  let screenWidth = 0;
  let screenHeight = 0;
  let screenshotWidth = 0;
  let screenshotHeight = 0;
  let platformScaleFactor = 1;
  if (typeof structuredJson === "string" && structuredJson !== "") {
    try {
      const parsed = JSON.parse(structuredJson);
      screenWidth = Number(parsed.screen_width) || 0;
      screenHeight = Number(parsed.screen_height) || 0;
      screenshotWidth = Number(parsed.screenshot_width) || 0;
      screenshotHeight = Number(parsed.screenshot_height) || 0;
      platformScaleFactor = Number(parsed.scale_factor) || 1;
    } catch {
      // Fall through to the bounded human-readable state parser below.
    }
  }
  if (
    (screenWidth <= 0 ||
      screenHeight <= 0 ||
      screenshotWidth <= 0 ||
      screenshotHeight <= 0) &&
    typeof text === "string"
  ) {
    const dimensions = text.match(
      /screenshot\s+(\d+)x(\d+)\s+px\s+\(screen\s+(\d+)x(\d+)\s+pts/i,
    );
    if (dimensions) {
      screenshotWidth = Number(dimensions[1]);
      screenshotHeight = Number(dimensions[2]);
      screenWidth = Number(dimensions[3]);
      screenHeight = Number(dimensions[4]);
    }
  }

  // The CUA Rust executable is manifested Per-Monitor V2, but Awaken embeds the
  // N-API module in node.exe. On Windows that host remains DPI-virtualized: the
  // screenshot is in logical desktop pixels while SendInput consumes physical
  // screen pixels. The CUA-reported scale_factor can be virtualized to 1 too,
  // so Desktop supplies the authoritative primary-display scale via the Host
  // environment. The package is pinned, so keep this adapter contract explicit
  // instead of inferring awareness from screen_width == screenshot_width (both
  // values are virtualized together in node.exe).
  if (process.platform === "win32") {
    const desktopScaleFactor = Number(
      process.env.AWAKEN_CUA_WINDOWS_SCALE_FACTOR,
    );
    const inputScaleFactor =
      Number.isFinite(desktopScaleFactor) &&
      desktopScaleFactor >= 1 &&
      desktopScaleFactor <= 4
        ? desktopScaleFactor
        : platformScaleFactor;
    record.coordinateScaleX = inputScaleFactor;
    record.coordinateScaleY = inputScaleFactor;
    return {
      unit: "logical_pixel",
      width: screenshotWidth || screenWidth,
      height: screenshotHeight || screenHeight,
      source_screenshot_width: screenshotWidth,
      source_screenshot_height: screenshotHeight,
      scale_x: record.coordinateScaleX,
      scale_y: record.coordinateScaleY,
    };
  }

  // Preserve the existing macOS (and fallback platform) behavior: the model
  // sees screen points while the driver receives backing pixels.
  if (screenshotWidth > 0 && screenWidth > 0) {
    record.coordinateScaleX = screenshotWidth / screenWidth;
  }
  if (screenshotHeight > 0 && screenHeight > 0) {
    record.coordinateScaleY = screenshotHeight / screenHeight;
  }
  return {
    unit: "screen_point",
    width: screenWidth,
    height: screenHeight,
    source_screenshot_width: screenshotWidth,
    source_screenshot_height: screenshotHeight,
    scale_x: record.coordinateScaleX,
    scale_y: record.coordinateScaleY,
  };
}

async function handleGetSessionState({ sessionID }) {
  const record = requireSession(sessionID);
  const input = { session: sessionID };
  const state = await driver.getSessionState(input);
  return {
    session: state.session,
    capture_scope: String(state.captureScope),
    effective_scope: String(state.effectiveScope),
    desktop_unlocked: state.desktopUnlocked,
    host_state: record.state,
    fence_token: record.fenceToken,
    control_epoch: record.controlEpoch,
  };
}

async function handleEndSession({ sessionID }) {
  const input = EndSessionInput.new({ session: sessionID });
  const output = await driver.endSession(input);
  sessions.delete(sessionID);
  return { session: output.session, active: output.active };
}

function requireSession(sessionID) {
  const record = sessions.get(sessionID);
  if (!record) {
    throw new Error(`session ${sessionID} not found`);
  }
  return record;
}

function requirePositiveInteger(value, name) {
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return value;
}

function validateAuthority(record, { fenceToken, controlEpoch, targetGeneration }) {
  if (requirePositiveInteger(fenceToken, "fenceToken") !== record.fenceToken) {
    throw new Error("stale fence token");
  }
  if (requirePositiveInteger(controlEpoch, "controlEpoch") !== record.controlEpoch) {
    throw new Error("stale control epoch");
  }
  if (requirePositiveInteger(targetGeneration, "targetGeneration") !== record.targetGeneration) {
    throw new Error("stale target generation");
  }
  if (record.state !== "ready") {
    throw new Error(`session cannot perform while ${record.state}`);
  }
}

async function handleRegisterPreparedAction(params) {
  const record = requireSession(params.sessionID);
  validateAuthority(record, params);
  if (typeof params.actionID !== "string" || params.actionID === "") {
    throw new Error("actionID is required");
  }
  if (typeof params.actionDigest !== "string" || !params.actionDigest.startsWith("sha256:")) {
    throw new Error("actionDigest is invalid");
  }
  if (typeof params.observationID !== "string" || params.observationID === "") {
    throw new Error("observationID is required");
  }
  const observation = record.observations.get(params.observationToken);
  if (!observation || observation.targetGeneration !== params.targetGeneration) {
    throw new Error("observation token is stale or unknown");
  }
  const expiresAt = Date.parse(params.expiresAt);
  if (!Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
    throw new Error("prepared action is already expired");
  }
  record.preparedActions.set(params.actionID, {
    actionDigest: params.actionDigest,
    observationID: params.observationID,
    observationToken: params.observationToken,
    fenceToken: params.fenceToken,
    controlEpoch: params.controlEpoch,
    targetGeneration: params.targetGeneration,
    target: observation.target,
    expiresAt,
    consumed: false,
  });
  return { registered: true };
}

async function handlePerformChecked(params, signal) {
  const record = requireSession(params.sessionID);
  validateAuthority(record, params);
  const previous = record.actionResults.get(params.actionID);
  if (previous) {
    return previous;
  }
  const prepared = record.preparedActions.get(params.actionID);
  if (!prepared) {
    throw new Error("prepared action is not registered");
  }
  if (
    prepared.consumed ||
    prepared.actionDigest !== params.actionDigest ||
    prepared.observationID !== params.observationID ||
    prepared.fenceToken !== params.fenceToken ||
    prepared.controlEpoch !== params.controlEpoch ||
    prepared.targetGeneration !== params.targetGeneration ||
    prepared.expiresAt <= Date.now()
  ) {
    throw new Error("prepared action authorization is stale or mismatched");
  }
  try {
    validatePerformParameters(
      params.action,
      params.parameters || {},
      prepared.target,
    );
  } catch (err) {
    // Parameter validation happens before the prepared action reaches the CUA
    // driver. This is a confirmed no-input failure, not an unknown outcome.
    prepared.consumed = true;
    const failed = {
      text: err instanceof Error ? err.message : String(err),
      images: [],
      structured_json: "",
      raw_json: "",
      is_error: true,
      degraded: false,
      status: "failed",
      error_code: "invalid_action_parameters",
      error_category: "validation",
    };
    record.actionResults.set(params.actionID, failed);
    return failed;
  }
  prepared.consumed = true;
  try {
    const result = await handlePerform(
      { ...params, authorizedTarget: prepared.target },
      signal,
    );
    const normalized = { ...result, status: result.is_error ? "failed" : "succeeded" };
    record.actionResults.set(params.actionID, normalized);
    return normalized;
  } catch (err) {
    if (signal.aborted) {
      throw err;
    }
    // Once the CUA operation has been invoked, an exception cannot prove that
    // no OS input occurred. Preserve that ambiguity and make retries return the
    // same unknown result instead of injecting again.
    const unknown = {
      text: err instanceof Error ? err.message : String(err),
      images: [],
      structured_json: "",
      raw_json: "",
      is_error: true,
      degraded: true,
      status: "unknown_outcome",
    };
    record.actionResults.set(params.actionID, unknown);
    return unknown;
  }
}

async function handleQuiesce({ sessionID, controlEpoch }) {
  const record = requireSession(sessionID);
  const nextEpoch = requirePositiveInteger(controlEpoch, "controlEpoch");
  if (nextEpoch <= record.controlEpoch) {
    throw new Error("controlEpoch must increase");
  }
  // This method itself is processed by the single serialized request queue.
  // Reaching it proves all earlier performChecked calls have completed.
  record.state = "taken_over";
  record.controlEpoch = nextEpoch;
  record.observations.clear();
  record.preparedActions.clear();
  return { quiesced: true, control_epoch: nextEpoch };
}

async function handleResume({ sessionID, fenceToken, controlEpoch }) {
  const record = requireSession(sessionID);
  const nextFence = requirePositiveInteger(fenceToken, "fenceToken");
  const nextEpoch = requirePositiveInteger(controlEpoch, "controlEpoch");
  if (nextFence < record.fenceToken || nextEpoch <= record.controlEpoch) {
    throw new Error("resume authority is stale");
  }
  record.fenceToken = nextFence;
  record.controlEpoch = nextEpoch;
  record.state = "ready";
  record.observations.clear();
  record.preparedActions.clear();
  return { resumed: true, fence_token: nextFence, control_epoch: nextEpoch };
}

function requireNumber(value, name) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new Error(`${name} must be a finite number`);
  }
  return value;
}

function optionalNumber(value, name) {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new Error(`${name} must be a finite number`);
  }
  return value;
}

function optionalBigInt(value, name) {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value === "bigint") {
    return value;
  }
  if (typeof value === "number" && Number.isInteger(value)) {
    return BigInt(value);
  }
  throw new Error(`${name} must be an integer`);
}

function optionalString(value, name) {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== "string") {
    throw new Error(`${name} must be a string`);
  }
  return value;
}

function optionalStringArray(value, name) {
  if (value === undefined) {
    return undefined;
  }
  if (!Array.isArray(value) || value.some((v) => typeof v !== "string")) {
    throw new Error(`${name} must be an array of strings`);
  }
  return value;
}

function requireNonEmptyString(value, name) {
  const text = optionalString(value, name);
  if (text === undefined || text.trim() === "") {
    throw new Error(`${name} must be a non-empty string`);
  }
  return text;
}

function requireNonEmptyStringArray(value, name) {
  const values = optionalStringArray(value, name);
  if (!values || values.length === 0 || values.some((item) => item.trim() === "")) {
    throw new Error(`${name} must be a non-empty array of strings`);
  }
  return values;
}

function requireScreenCoordinate(value, name) {
  const number = requireNumber(value, name);
  if (!Number.isInteger(number) || number < 0) {
    throw new Error(`${name} must be a non-negative integer`);
  }
  return number;
}

function validatePerformParameters(action, parameters, authorizedTarget) {
  switch (action) {
    case "click":
    case "double_click":
      if (
        authorizedTarget?.kind === "window" &&
        (typeof parameters.element_token === "string" ||
          (parameters.snapshot_id !== undefined && parameters.element_index !== undefined))
      ) {
        return;
      }
      requireScreenCoordinate(parameters.x, "x");
      requireScreenCoordinate(parameters.y, "y");
      return;
    case "scroll":
    case "move_cursor":
      requireScreenCoordinate(parameters.x, "x");
      requireScreenCoordinate(parameters.y, "y");
      return;
    case "drag":
      requireScreenCoordinate(parameters.from_x, "from_x");
      requireScreenCoordinate(parameters.from_y, "from_y");
      requireScreenCoordinate(parameters.to_x, "to_x");
      requireScreenCoordinate(parameters.to_y, "to_y");
      return;
    case "key":
      requireNonEmptyString(parameters.key, "key");
      return;
    case "hotkey":
      {
        const keys = requireNonEmptyStringArray(parameters.keys, "keys");
        if (
          process.platform === "win32" &&
          keys.some((key) =>
            ["win", "windows", "meta", "cmd", "command"].includes(key.trim().toLowerCase()),
          )
        ) {
          throw new Error(
            "Windows Shell global hotkeys are not reliably supported by the current desktop CUA route: the upstream driver can report the keys as sent without the Shell handling them; use a fresh screenshot and click the visible system UI instead",
          );
        }
      }
      return;
    case "type_text":
      requireNonEmptyString(parameters.text, "text");
      return;
    case "wait": {
      const ms = optionalNumber(parameters.ms, "ms");
      if (ms !== undefined && (!Number.isInteger(ms) || ms < 0)) {
        throw new Error("ms must be a non-negative integer");
      }
      return;
    }
    default:
      throw new Error(`unsupported action: ${action}`);
  }
}

function abortableDelay(ms, signal) {
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal.reason ?? new Error("request cancelled"));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

function windowActionArguments(sessionID, action, params, authorizedTarget) {
  const deliveryMode = optionalString(params.delivery_mode, "delivery_mode") || "background";
  if (deliveryMode !== "background" && deliveryMode !== "foreground") {
    throw new Error("delivery_mode must be background or foreground");
  }
  const args = {
    session: sessionID,
    target: authorizedTarget,
    delivery_mode: deliveryMode,
  };
  if (params.element_token !== undefined) {
    args.element_token = optionalString(params.element_token, "element_token");
  }
  if (params.snapshot_id !== undefined) {
    args.snapshot_id = optionalString(params.snapshot_id, "snapshot_id");
  }
  if (params.element_index !== undefined) {
    args.element_index = requireNumber(params.element_index, "element_index");
  }
  switch (action) {
    case "click":
    case "double_click":
      args.x = params.x === undefined ? undefined : requireNumber(params.x, "x");
      args.y = params.y === undefined ? undefined : requireNumber(params.y, "y");
      args.button = optionalString(params.button, "button") || "left";
      args.count = action === "double_click" ? 2 : (params.count ?? 1);
      return { tool: "click", args };
    case "scroll":
      args.x = params.x === undefined ? undefined : requireNumber(params.x, "x");
      args.y = params.y === undefined ? undefined : requireNumber(params.y, "y");
      args.direction = optionalString(params.direction, "direction") || "down";
      args.by = optionalString(params.by, "by") || "line";
      args.amount = optionalNumber(params.amount, "amount") ?? 1;
      return { tool: "scroll", args };
    case "drag":
      args.from_x = requireNumber(params.from_x, "from_x");
      args.from_y = requireNumber(params.from_y, "from_y");
      args.to_x = requireNumber(params.to_x, "to_x");
      args.to_y = requireNumber(params.to_y, "to_y");
      args.duration_ms = optionalNumber(params.duration_ms, "duration_ms") ?? 500;
      args.steps = optionalNumber(params.steps, "steps") ?? 10;
      args.button = optionalString(params.button, "button") || "left";
      args.modifier = optionalStringArray(params.modifier, "modifier");
      return { tool: "drag", args };
    case "key":
      args.key = optionalString(params.key, "key") || "";
      args.modifiers = optionalStringArray(params.modifiers, "modifiers");
      return { tool: "press_key", args };
    case "hotkey":
      args.keys = optionalStringArray(params.keys, "keys") || [];
      return { tool: "hotkey", args };
    case "type_text":
      args.text = optionalString(params.text, "text") || "";
      return { tool: "type_text", args };
    case "move_cursor":
      args.x = requireNumber(params.x, "x");
      args.y = requireNumber(params.y, "y");
      return { tool: "move_cursor", args };
    default:
      throw new Error(`unsupported window action: ${action}`);
  }
}

async function handlePerform({ sessionID, action, parameters, authorizedTarget }, signal) {
  const record = requireSession(sessionID);
  const params = parameters || {};
  let result;
  const desktopX = (value, name) =>
    requireNumber(value, name) * record.coordinateScaleX;
  const desktopY = (value, name) =>
    requireNumber(value, name) * record.coordinateScaleY;
  signal.throwIfAborted();

  if (authorizedTarget?.kind === "window" && action !== "wait") {
    const request = windowActionArguments(
      sessionID,
      action,
      params,
      authorizedTarget,
    );
    result = await driver.callTool(request.tool, JSON.stringify(request.args));
    signal.throwIfAborted();
    return toolResultEnvelope(result);
  }
  if (authorizedTarget?.kind !== "desktop" && action !== "wait") {
    throw new Error("prepared action has no exact authorized target");
  }
  if (params.delivery_mode === "background") {
    throw new Error("desktop target does not support background delivery");
  }

  switch (action) {
    case "click":
    case "double_click": {
      const count = action === "double_click" ? 2 : (params.count ?? 1);
      const button = BUTTON_MAP[params.button] ?? ClickButton.Left;
      const input = ClickInput.new({
        session: sessionID,
        x: desktopX(params.x, "x"),
        y: desktopY(params.y, "y"),
        target: primaryDesktopTarget(),
        button,
        count,
      });
      result = await driver.click(input);
      break;
    }
    case "scroll": {
      const input = ScrollInput.new({
        session: sessionID,
        x: desktopX(params.x, "x"),
        y: desktopY(params.y, "y"),
        direction: DIRECTION_MAP[params.direction] ?? ScrollDirection.Down,
        target: primaryDesktopTarget(),
        by: params.by ? BY_MAP[params.by] : ScrollBy.Line,
        amount: optionalBigInt(params.amount, "amount") ?? BigInt(1),
      });
      result = await driver.scroll(input);
      break;
    }
    case "drag": {
      const input = DragInput.new({
        session: sessionID,
        fromX: desktopX(params.from_x, "from_x"),
        fromY: desktopY(params.from_y, "from_y"),
        toX: desktopX(params.to_x, "to_x"),
        toY: desktopY(params.to_y, "to_y"),
        target: primaryDesktopTarget(),
        durationMs: optionalBigInt(params.duration_ms, "duration_ms") ?? BigInt(500),
        steps: optionalBigInt(params.steps, "steps") ?? BigInt(10),
        button: params.button ? BUTTON_MAP[params.button] : ClickButton.Left,
        modifier: optionalStringArray(params.modifier, "modifier"),
      });
      result = await driver.drag(input);
      break;
    }
    case "key": {
      const input = PressKeyInput.new({
        session: sessionID,
        key: optionalString(params.key, "key") || "",
        target: primaryDesktopTarget(),
        modifiers: optionalStringArray(params.modifiers, "modifiers"),
      });
      result = await driver.pressKey(input);
      break;
    }
    case "hotkey": {
      const input = HotkeyInput.new({
        session: sessionID,
        keys: optionalStringArray(params.keys, "keys") || [],
        target: primaryDesktopTarget(),
      });
      result = await driver.hotkey(input);
      break;
    }
    case "type_text": {
      const input = TypeTextInput.new({
        session: sessionID,
        text: optionalString(params.text, "text") || "",
        target: primaryDesktopTarget(),
      });
      result = await driver.typeText(input);
      break;
    }
    case "move_cursor": {
      const input = MoveCursorInput.new({
        session: sessionID,
        x: desktopX(params.x, "x"),
        y: desktopY(params.y, "y"),
        target: primaryDesktopTarget(),
      });
      result = await driver.moveCursor(input);
      break;
    }
    case "wait": {
      const ms = optionalNumber(params.ms, "ms") ?? 1000;
      await abortableDelay(ms, signal);
      result = {
        text: "waited",
        images: [],
        isError: false,
        degraded: false,
        rawJson: "{}",
      };
      break;
    }
    default:
      throw new Error(`unsupported action: ${action}`);
  }
  signal.throwIfAborted();

  return toolResultEnvelope(result);
}

async function dispatch(method, params, signal) {
  switch (method) {
    case "health":
      return await handleHealth();
    case "requestPermissions":
      return await handleRequestPermissions();
    case "startSession":
      return await handleStartSession(params);
    case "getDesktopState":
      return await handleGetDesktopState(params);
    case "getWindowState":
      return await handleGetWindowState(params);
    case "getSessionState":
      return await handleGetSessionState(params);
    case "registerPreparedAction":
      return await handleRegisterPreparedAction(params);
    case "performChecked":
      return await handlePerformChecked(params, signal);
    case "quiesce":
      return await handleQuiesce(params);
    case "resume":
      return await handleResume(params);
    case "endSession":
      return await handleEndSession(params);
    default:
      throw new Error(`unknown method: ${method}`);
  }
}

const requestQueue = [];
let queueProcessing = false;
const activeRequestControllers = new Map();
const cancelledRequestIDs = new Set();

async function processLine(line) {
  let request;
  try {
    request = JSON.parse(line);
  } catch (err) {
    sendError(null, "PARSE_ERROR", "invalid JSON", err.message);
    return;
  }
  const { id, method, params } = request;
  if (typeof method !== "string") {
    sendError(id ?? null, "INVALID_REQUEST", "method must be a string");
    return;
  }
  if (shuttingDown) {
    sendError(id, "SHUTTING_DOWN", "host is shutting down");
    return;
  }
  const requestID = String(id);
  if (cancelledRequestIDs.delete(requestID)) {
    sendError(id, "REQUEST_CANCELLED", "request cancelled");
    return;
  }
  const controller = new AbortController();
  activeRequestControllers.set(requestID, controller);
  try {
    const result = await dispatch(method, params || {}, controller.signal);
    sendResponse(id, result);
  } catch (err) {
    log("error", `${method} failed: ${err.message}`);
    sendError(id, "HOST_ERROR", err.message, err.stack);
  } finally {
    activeRequestControllers.delete(requestID);
  }
}

async function processQueue() {
  while (requestQueue.length > 0) {
    const line = requestQueue.shift();
    try {
      await processLine(line);
    } catch (err) {
      log("error", `unhandled queue error: ${err.message}`);
    }
  }
}

function enqueueLine(line) {
  try {
    const request = JSON.parse(line);
    if (request?.method === "cancelRequest") {
      const requestID = String(request.params?.request_id ?? "");
      const controller = activeRequestControllers.get(requestID);
      if (controller) {
        controller.abort(new Error("request cancelled"));
      } else if (requestID) {
        cancelledRequestIDs.add(requestID);
      }
      sendResponse(request.id ?? null, { cancelled: Boolean(requestID) });
      return;
    }
  } catch {
    // Let the regular queue return the canonical parse error.
  }
  requestQueue.push(line);
  if (!queueProcessing) {
    queueProcessing = true;
    processQueue().finally(() => {
      queueProcessing = false;
    });
  }
}

async function main() {
  log("info", "initializing CUA driver...");
  driver = CuaDriver.create(undefined);
  // list_windows has no explicit session argument in CUA 0.22.0 and therefore
  // uses the SDK's idle-expiring implicit session. Isolate it from Awaken's
  // run-bound named sessions so discovery can be safely rebuilt without
  // invalidating an active capture/action session.
  windowDiscoveryDriver = CuaDriver.create(undefined);
  log("info", `driver available: ${driver.isAvailable()}`);

  const metadata = await driver.metadata();
  log("info", `driver version: ${metadata.driverVersion}`);

  process.stdin.setEncoding("utf8");
  let buffer = "";
  process.stdin.on("data", (chunk) => {
    buffer += chunk;
    const lines = buffer.split("\n");
    buffer = lines.pop();
    for (const line of lines) {
      if (line.trim() === "") {
        continue;
      }
      enqueueLine(line);
    }
  });

  process.stdin.on("end", () => {
    if (buffer.trim() !== "") {
      enqueueLine(buffer);
    }
    // Wait for the queue to drain before shutting down so in-flight requests
    // can still produce responses.
    const waitForQueue = () => {
      if (queueProcessing || requestQueue.length > 0) {
        setTimeout(waitForQueue, 10);
        return;
      }
      shutdown();
    };
    waitForQueue();
  });

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

async function shutdown() {
  if (shuttingDown || !driver) {
    return;
  }
  shuttingDown = true;
  log("info", "shutting down driver...");
  try {
    await driver.shutdown();
  } catch (err) {
    log("warn", `shutdown error: ${err.message}`);
  }
  try {
    await windowDiscoveryDriver?.shutdown();
  } catch (err) {
    log("warn", `window discovery shutdown error: ${err.message}`);
  }
  try {
    windowDiscoveryDriver?.uniffiDestroy();
  } catch (err) {
    log("warn", `window discovery destroy error: ${err.message}`);
  }
  try {
    driver.uniffiDestroy();
  } catch (err) {
    log("warn", `destroy error: ${err.message}`);
  }
  process.exit(0);
}

main().catch((err) => {
  log("fatal", err.message);
  process.exit(1);
});
