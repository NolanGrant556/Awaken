export const EMBEDDED_BROWSER_PARTITION = "persist:awaken-browser";

export const EMBEDDED_BROWSER_CHANNELS = {
  navigate: "awaken:embedded-browser:navigate",
  back: "awaken:embedded-browser:back",
  forward: "awaken:embedded-browser:forward",
  reload: "awaken:embedded-browser:reload",
  stop: "awaken:embedded-browser:stop",
  close: "awaken:embedded-browser:close",
  closeThread: "awaken:embedded-browser:close-thread",
  getState: "awaken:embedded-browser:get-state",
  setViewport: "awaken:embedded-browser:set-viewport",
  setActive: "awaken:embedded-browser:set-active",
  stateChanged: "awaken:embedded-browser:state-changed",
  guestHttpNavigate: "awaken:embedded-browser:guest-http-navigate",
  requestShow: "awaken:embedded-browser:request-show",
} as const;

export type EmbeddedBrowserState = {
  url: string;
  title: string;
  canGoBack: boolean;
  canGoForward: boolean;
  loading: boolean;
  secure: boolean;
  hasPage: boolean;
};

export type EmbeddedBrowserViewRect = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type EmbeddedBrowserTabRef = {
  viewId: string;
  threadId: string;
};

export type EmbeddedBrowserStatePayload = EmbeddedBrowserTabRef & {
  state: EmbeddedBrowserState;
};

export type EmbeddedBrowserGuestHttpNavigate = {
  previewId: string;
  url: string;
};

export type EmbeddedBrowserShowRequest = {
  threadId: string;
  viewId: string;
};

export type EmbeddedBrowserAddressFailureReason =
  | "empty"
  | "unsupported_scheme"
  | "invalid_url";

export type EmbeddedBrowserAddressResult =
  | { ok: true; url: string }
  | { ok: false; reason: EmbeddedBrowserAddressFailureReason };

export type EmbeddedBrowserActionKind = "observe" | "mutate" | "navigate";

export const EMPTY_EMBEDDED_BROWSER_STATE: EmbeddedBrowserState = {
  url: "",
  title: "",
  canGoBack: false,
  canGoForward: false,
  loading: false,
  secure: false,
  hasPage: false,
};

const EXTERNAL_SCHEMES = new Set(["mailto:", "tel:"]);

export function normalizeBrowserAddressInput(
  input: string,
): EmbeddedBrowserAddressResult {
  const trimmed = input.trim();
  if (!trimmed) return { ok: false, reason: "empty" };

  const hasScheme = /^[a-z][a-z0-9+.-]*:/i.test(trimmed);
  const candidate = hasScheme ? trimmed : `https://${trimmed}`;

  let url: URL;
  try {
    url = new URL(candidate);
  } catch {
    return { ok: false, reason: "invalid_url" };
  }

  if (url.protocol !== "http:" && url.protocol !== "https:") {
    return { ok: false, reason: "unsupported_scheme" };
  }

  return { ok: true, url: url.toString() };
}

export function parseNavigable(input: string): string | null {
  const result = normalizeBrowserAddressInput(input);
  return result.ok ? result.url : null;
}

export function safeExternalUrl(url: string): string | null {
  const scheme = url.slice(0, url.indexOf(":") + 1).toLowerCase();
  return EXTERNAL_SCHEMES.has(scheme) ? url : null;
}

export function viewportBounds(
  rect: EmbeddedBrowserViewRect | null,
): EmbeddedBrowserViewRect | null {
  if (!rect) return null;
  if (
    ![rect.x, rect.y, rect.width, rect.height].every((value) =>
      Number.isFinite(value),
    )
  ) {
    return null;
  }
  const width = Math.max(0, Math.round(rect.width));
  const height = Math.max(0, Math.round(rect.height));
  if (width <= 0 || height <= 0) return null;
  return {
    x: Math.max(0, Math.round(rect.x)),
    y: Math.max(0, Math.round(rect.y)),
    width,
    height,
  };
}

export function browserActionAllowed(
  kind: EmbeddedBrowserActionKind,
  view: { shown: boolean; hasViewport: boolean },
): boolean {
  if (!view.shown) return false;
  return kind === "mutate" ? view.hasViewport : true;
}

export function deriveBrowserState(snapshot: {
  url: string;
  title: string;
  canGoBack: boolean;
  canGoForward: boolean;
  loading: boolean;
}): EmbeddedBrowserState {
  return {
    url: snapshot.url,
    title: snapshot.title,
    canGoBack: snapshot.canGoBack,
    canGoForward: snapshot.canGoForward,
    loading: snapshot.loading,
    secure: /^https:\/\//i.test(snapshot.url),
    hasPage: snapshot.url !== "" && !snapshot.url.startsWith("about:"),
  };
}

export function originOf(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return "";
  }
}

export function createIdleEmbeddedBrowserClient(): EmbeddedBrowserBridge {
  const idle = async () => EMPTY_EMBEDDED_BROWSER_STATE;
  return {
    navigate: idle,
    back: idle,
    forward: idle,
    reload: idle,
    stop: idle,
    close: async () => undefined,
    closeThread: async () => undefined,
    getState: idle,
    setViewport: async () => undefined,
    setActive: async () => undefined,
    onStateChanged: () => () => undefined,
    onGuestHttpNavigate: () => () => undefined,
    onRequestShow: () => () => undefined,
  };
}

export type EmbeddedBrowserBridge = {
  navigate: (
    ref: EmbeddedBrowserTabRef,
    url: string,
  ) => Promise<EmbeddedBrowserState>;
  back: (ref: EmbeddedBrowserTabRef) => Promise<EmbeddedBrowserState>;
  forward: (ref: EmbeddedBrowserTabRef) => Promise<EmbeddedBrowserState>;
  reload: (ref: EmbeddedBrowserTabRef) => Promise<EmbeddedBrowserState>;
  stop: (ref: EmbeddedBrowserTabRef) => Promise<EmbeddedBrowserState>;
  close: (ref: EmbeddedBrowserTabRef) => Promise<void>;
  closeThread: (threadId: string) => Promise<void>;
  getState: (ref: EmbeddedBrowserTabRef) => Promise<EmbeddedBrowserState>;
  setViewport: (
    ref: EmbeddedBrowserTabRef,
    rect: EmbeddedBrowserViewRect | null,
  ) => Promise<void>;
  setActive: (ref: EmbeddedBrowserTabRef | null) => Promise<void>;
  onStateChanged: (
    handler: (payload: EmbeddedBrowserStatePayload) => void,
  ) => () => void;
  onGuestHttpNavigate: (
    handler: (payload: EmbeddedBrowserGuestHttpNavigate) => void,
  ) => () => void;
  onRequestShow: (
    handler: (payload: EmbeddedBrowserShowRequest) => void,
  ) => () => void;
};
