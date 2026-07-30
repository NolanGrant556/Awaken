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
  DesktopScope,
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

let driver;
let shuttingDown = false;
const sessions = new Map();

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
  });
  return {
    text: result.text,
    images: result.images.map((img) => ({
      mime_type: img.mimeType,
      data_base64: img.dataBase64,
    })),
    structured_json: result.structuredJson,
    raw_json: result.rawJson,
    is_error: result.isError,
    degraded: result.degraded,
    observation_token: observationToken,
    coordinate_space: coordinateSpace,
  };
}

function updateCoordinateScale(record, text, structuredJson) {
  let screenWidth = 0;
  let screenHeight = 0;
  let screenshotWidth = 0;
  let screenshotHeight = 0;
  if (typeof structuredJson === "string" && structuredJson !== "") {
    try {
      const parsed = JSON.parse(structuredJson);
      screenWidth = Number(parsed.screen_width) || 0;
      screenHeight = Number(parsed.screen_height) || 0;
      screenshotWidth = Number(parsed.screenshot_width) || 0;
      screenshotHeight = Number(parsed.screenshot_height) || 0;
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
  prepared.consumed = true;
  try {
    const result = await handlePerform(params, signal);
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
  if (typeof value !== "number") {
    throw new Error(`${name} must be a number`);
  }
  return value;
}

function optionalNumber(value, name) {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== "number") {
    throw new Error(`${name} must be a number`);
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

async function handlePerform({ sessionID, action, parameters }, signal) {
  const record = requireSession(sessionID);
  const params = parameters || {};
  const scope = DesktopScope.Desktop;
  let result;
  const desktopX = (value, name) =>
    requireNumber(value, name) * record.coordinateScaleX;
  const desktopY = (value, name) =>
    requireNumber(value, name) * record.coordinateScaleY;
  signal.throwIfAborted();

  switch (action) {
    case "click":
    case "double_click": {
      const count = action === "double_click" ? 2 : (params.count ?? 1);
      const button = BUTTON_MAP[params.button] ?? ClickButton.Left;
      const input = ClickInput.new({
        session: sessionID,
        x: desktopX(params.x, "x"),
        y: desktopY(params.y, "y"),
        scope,
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
        scope,
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
        scope,
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
        scope,
        modifiers: optionalStringArray(params.modifiers, "modifiers"),
      });
      result = await driver.pressKey(input);
      break;
    }
    case "hotkey": {
      const input = HotkeyInput.new({
        session: sessionID,
        keys: optionalStringArray(params.keys, "keys") || [],
        scope,
      });
      result = await driver.hotkey(input);
      break;
    }
    case "type_text": {
      const input = TypeTextInput.new({
        session: sessionID,
        text: optionalString(params.text, "text") || "",
        scope,
      });
      result = await driver.typeText(input);
      break;
    }
    case "move_cursor": {
      const input = MoveCursorInput.new({
        session: sessionID,
        x: desktopX(params.x, "x"),
        y: desktopY(params.y, "y"),
        scope,
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

  return {
    text: result.text,
    images: result.images.map((img) => ({
      mime_type: img.mimeType,
      data_base64: img.dataBase64,
    })),
    structured_json: result.structuredJson,
    raw_json: result.rawJson,
    is_error: result.isError,
    degraded: result.degraded,
  };
}

async function dispatch(method, params, signal) {
  switch (method) {
    case "health":
      return await handleHealth();
    case "startSession":
      return await handleStartSession(params);
    case "getDesktopState":
      return await handleGetDesktopState(params);
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
