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
    protocol_version: 1,
    os: process.platform,
    tcc_screen_recording: Boolean(permissions.screenRecording),
    tcc_accessibility: Boolean(permissions.accessibility),
    message: "CUA embedded host",
  };
}

async function handleStartSession({ sessionID, captureScope }) {
  const input = StartSessionInput.new({
    session: sessionID,
    captureScope: captureScopeFromString(captureScope),
  });
  const output = await driver.startSession(input);
  return {
    host_session_ref: output.state.session,
    capture_scope: String(captureScope),
    active: output.active,
    revived: output.revived,
  };
}

async function handleGetDesktopState({ sessionID }) {
  const input = GetDesktopStateInput.new({ session: sessionID });
  const result = await driver.getDesktopState(input);
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

async function handleGetSessionState({ sessionID }) {
  const input = { session: sessionID };
  const state = await driver.getSessionState(input);
  return {
    session: state.session,
    capture_scope: String(state.captureScope),
    effective_scope: String(state.effectiveScope),
    desktop_unlocked: state.desktopUnlocked,
  };
}

async function handleEndSession({ sessionID }) {
  const input = EndSessionInput.new({ session: sessionID });
  const output = await driver.endSession(input);
  return { session: output.session, active: output.active };
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

async function handlePerform({ sessionID, action, parameters }) {
  const params = parameters || {};
  const scope = DesktopScope.Desktop;
  let result;

  switch (action) {
    case "click":
    case "double_click": {
      const count = action === "double_click" ? 2 : (params.count ?? 1);
      const button = BUTTON_MAP[params.button] ?? ClickButton.Left;
      const input = ClickInput.new({
        session: sessionID,
        x: requireNumber(params.x, "x"),
        y: requireNumber(params.y, "y"),
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
        x: requireNumber(params.x, "x"),
        y: requireNumber(params.y, "y"),
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
        fromX: requireNumber(params.from_x, "from_x"),
        fromY: requireNumber(params.from_y, "from_y"),
        toX: requireNumber(params.to_x, "to_x"),
        toY: requireNumber(params.to_y, "to_y"),
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
        x: requireNumber(params.x, "x"),
        y: requireNumber(params.y, "y"),
        scope,
      });
      result = await driver.moveCursor(input);
      break;
    }
    case "wait": {
      const ms = optionalNumber(params.ms, "ms") ?? 1000;
      await new Promise((resolve) => setTimeout(resolve, ms));
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

async function dispatch(method, params) {
  switch (method) {
    case "health":
      return await handleHealth();
    case "startSession":
      return await handleStartSession(params);
    case "getDesktopState":
      return await handleGetDesktopState(params);
    case "getSessionState":
      return await handleGetSessionState(params);
    case "endSession":
      return await handleEndSession(params);
    case "perform":
      return await handlePerform(params);
    default:
      throw new Error(`unknown method: ${method}`);
  }
}

const requestQueue = [];
let queueProcessing = false;

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
  try {
    const result = await dispatch(method, params || {});
    sendResponse(id, result);
  } catch (err) {
    log("error", `${method} failed: ${err.message}`);
    sendError(id, "HOST_ERROR", err.message, err.stack);
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
