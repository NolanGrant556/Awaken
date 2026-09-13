import type { BrowserWindow, WebContents } from "electron";
import {
  browserActionAllowed,
  deriveBrowserState,
  EMPTY_EMBEDDED_BROWSER_STATE,
  localHtmlPartition,
  normalizeBrowserAddressInput,
  originOf,
  parseNavigable,
  type EmbeddedBrowserActionKind,
  type EmbeddedBrowserState,
  type EmbeddedBrowserTabRef,
  type EmbeddedBrowserViewRect,
} from "../../shared/embedded-browser.js";
import { EmbeddedBrowserViewController } from "./controller.js";

const VIEWPORT_RESTORE_WAIT_MS = 4000;

export type EmbeddedBrowserCreateViewOptions = {
  allowFile: boolean;
  partition?: string;
  guest: WebContents;
};

export type EmbeddedBrowserViewManagerOptions = {
  getWindow: () => BrowserWindow | null;
  onState: (viewId: string, threadId: string, state: EmbeddedBrowserState) => void;
  onRequestShow: (threadId: string, viewId: string) => void;
  createView?: (
    window: BrowserWindow,
    onState: (state: EmbeddedBrowserState) => void,
    options: EmbeddedBrowserCreateViewOptions,
  ) => EmbeddedBrowserViewController;
};

type ViewRecord = {
  ref: EmbeddedBrowserTabRef;
  controller: EmbeddedBrowserViewController;
};

type PendingNavigation = {
  ref: EmbeddedBrowserTabRef;
  url: string;
  allowFile: boolean;
};

function viewKey(ref: EmbeddedBrowserTabRef): string {
  return `${ref.threadId}::${ref.viewId}`;
}

function optimisticState(url: string): EmbeddedBrowserState {
  return deriveBrowserState({
    url,
    title: "",
    canGoBack: false,
    canGoForward: false,
    loading: true,
  });
}

export class EmbeddedBrowserViewManager {
  private readonly views = new Map<string, ViewRecord>();
  private readonly savedPages = new Map<string, {
    ref: EmbeddedBrowserTabRef;
    state: EmbeddedBrowserState;
    admittedOrigin: string;
  }>();
  private readonly pendingNav = new Map<string, PendingNavigation>();
  private activeKey: string | null = null;
  private pendingActive: EmbeddedBrowserTabRef | null = null;

  constructor(private readonly options: EmbeddedBrowserViewManagerOptions) {}

  getState(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    const key = viewKey(ref);
    const live = this.views.get(key)?.controller.state();
    return live?.hasPage ? live : this.savedPages.get(key)?.state ?? live ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  async navigate(ref: EmbeddedBrowserTabRef, url: string): Promise<EmbeddedBrowserState> {
    const parsed = normalizeBrowserAddressInput(url);
    if (!parsed.ok) {
      throw new Error("unsupported_scheme");
    }
    const allowFile = parsed.kind === "file";
    const key = viewKey(ref);
    this.pendingNav.set(key, { ref, url: parsed.url, allowFile });
    this.activeKey = key;
    this.pendingActive = ref;
    this.hideAllExcept(key);

    const existing = this.views.get(key);
    if (existing && existing.controller.loadsLocalFiles !== allowFile) {
      existing.controller.destroy();
      this.views.delete(key);
    }

    const current = this.views.get(key);
    if (current) {
      this.pendingNav.delete(key);
      return current.controller.navigate(parsed.url);
    }

    const state = optimisticState(parsed.url);
    this.savedPages.set(key, { ref, state, admittedOrigin: "" });
    this.options.onState(ref.viewId, ref.threadId, state);
    return state;
  }

  attachGuest(
    ref: EmbeddedBrowserTabRef,
    guest: WebContents,
    allowFile: boolean,
  ): EmbeddedBrowserState {
    if (guest.isDestroyed?.()) {
      throw new Error("embedded browser guest is not available");
    }
    const key = viewKey(ref);
    const existing = this.views.get(key);
    if (
      existing &&
      existing.controller.loadsLocalFiles === allowFile &&
      existing.controller.ownsGuest(guest)
    ) {
      return existing.controller.state();
    }
    if (existing) {
      existing.controller.destroy();
      this.views.delete(key);
    }

    const window = this.options.getWindow();
    if (!window || window.isDestroyed()) {
      throw new Error("main window is not available");
    }
    const create =
      this.options.createView ??
      ((owner, onState, options) =>
        new EmbeddedBrowserViewController(owner, onState, {
          allowFile: options.allowFile,
          partition: options.partition,
          guest: options.guest,
          onHandoffHttp: options.allowFile
            ? (httpUrl) => {
                void this.navigate(ref, httpUrl);
              }
            : undefined,
        }));
    const controller = create(
      window,
      (state) => {
        if (this.views.get(key)?.controller !== controller) return;
        this.savedPages.set(key, { ref, state, admittedOrigin: controller.authorizedOrigin });
        this.options.onState(ref.viewId, ref.threadId, state);
      },
      {
        allowFile,
        partition: allowFile ? localHtmlPartition(ref.viewId) : undefined,
        guest,
      },
    );
    this.views.set(key, { ref, controller });

    const pending = this.pendingNav.get(key);
    if (pending && pending.allowFile === allowFile) {
      this.pendingNav.delete(key);
      void controller.navigate(pending.url);
    } else {
      const admittedOrigin = this.savedPages.get(key)?.admittedOrigin;
      if (admittedOrigin) controller.admitOrigin(admittedOrigin);
    }
    return controller.state();
  }

  detachGuest(ref: EmbeddedBrowserTabRef): void {
    const key = viewKey(ref);
    const record = this.views.get(key);
    if (!record) return;
    const state = record.controller.state();
    if (state.hasPage) {
      this.savedPages.set(key, { ref, state, admittedOrigin: record.controller.authorizedOrigin });
    }
    record.controller.destroy();
    this.views.delete(key);
  }

  back(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    return this.views.get(viewKey(ref))?.controller.back() ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  forward(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    return this.views.get(viewKey(ref))?.controller.forward() ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  reload(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    return this.views.get(viewKey(ref))?.controller.reload() ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  stop(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    return this.views.get(viewKey(ref))?.controller.stop() ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  setViewport(ref: EmbeddedBrowserTabRef, rect: EmbeddedBrowserViewRect | null): void {
    const record = this.views.get(viewKey(ref));
    if (!record) return;
    if (this.activeKey !== viewKey(ref)) {
      record.controller.hide();
      return;
    }
    record.controller.setBounds(rect);
  }

  setActive(ref: EmbeddedBrowserTabRef | null): void {
    this.pendingActive = ref;
    this.activeKey = ref ? viewKey(ref) : null;
    this.hideAllExcept(this.activeKey);
  }

  close(ref: EmbeddedBrowserTabRef): void {
    const key = viewKey(ref);
    const record = this.views.get(key);
    if (record) {
      record.controller.destroy();
      this.views.delete(key);
    }
    this.pendingNav.delete(key);
    this.savedPages.delete(key);
    if (this.activeKey === key) this.activeKey = null;
    if (this.pendingActive && viewKey(this.pendingActive) === key) {
      this.pendingActive = null;
    }
  }

  hideAll(): void {
    this.activeKey = null;
    this.pendingActive = null;
    this.pendingNav.clear();
    this.savedPages.clear();
    for (const record of this.views.values()) record.controller.destroy();
    this.views.clear();
  }

  closeThread(threadId: string): void {
    for (const [key, record] of this.views) {
      if (record.ref.threadId !== threadId) continue;
      record.controller.destroy();
      this.views.delete(key);
      this.pendingNav.delete(key);
      if (this.activeKey === key) this.activeKey = null;
    }
    for (const [key, saved] of this.savedPages) {
      if (saved.ref.threadId === threadId) this.savedPages.delete(key);
    }
    for (const [key, pending] of this.pendingNav) {
      if (pending.ref.threadId === threadId) this.pendingNav.delete(key);
    }
    if (this.pendingActive?.threadId === threadId) {
      this.pendingActive = null;
      this.activeKey = null;
    }
  }

  dispose(): void {
    for (const record of this.views.values()) record.controller.destroy();
    this.views.clear();
    this.pendingNav.clear();
    this.savedPages.clear();
    this.activeKey = null;
    this.pendingActive = null;
  }

  viewForAction(threadId: string, kind: EmbeddedBrowserActionKind): EmbeddedBrowserViewController | null {
    const shown = this.shownRecordForThread(threadId);
    if (!shown || shown.controller.loadsLocalFiles || !parseNavigable(shown.controller.currentURL())) return null;
    if (!browserActionAllowed(kind, shown.controller)) return null;
    return shown.controller;
  }

  async navigateForAgent(threadId: string, url: string): Promise<EmbeddedBrowserState> {
    const controller = await this.ensureVisibleForAgent(threadId, url);
    await controller.waitForNavigation();
    if (this.viewForAction(threadId, "observe") !== controller) {
      throw new Error("browser_view_not_visible");
    }
    return controller.state();
  }

  async ensureVisibleForAgent(threadId: string, url: string): Promise<EmbeddedBrowserViewController> {
    if (!parseNavigable(url)) throw new Error("unsupported_scheme");
    const existing = this.shownRecordForThread(threadId) ?? this.anyRecordForThread(threadId);
    const hint = existing?.ref.viewId ?? (this.pendingActive?.threadId === threadId ? this.pendingActive.viewId : "");
    this.options.onRequestShow(threadId, hint);
    const deadline = Date.now() + VIEWPORT_RESTORE_WAIT_MS;
    let requestedKey: string | null = null;
    while (Date.now() < deadline) {
      const ref = this.pendingActive;
      if (ref?.threadId === threadId && requestedKey !== viewKey(ref)) {
        requestedKey = viewKey(ref);
        // Publish the URL before waiting: an empty renderer tab has no guest yet.
        await this.navigate(ref, url);
      }
      const shown = this.shownRecordForThread(threadId);
      if (shown && viewKey(shown.ref) === requestedKey && !shown.controller.loadsLocalFiles) {
        return shown.controller;
      }
      await new Promise((resolve) => setTimeout(resolve, 16));
    }
    throw new Error("browser_view_not_visible");
  }

  currentOrigin(threadId: string): string {
    return originOf(this.shownRecordForThread(threadId)?.controller.currentURL() ?? "");
  }

  private shownRecordForThread(threadId: string): ViewRecord | undefined {
    if (!this.activeKey) return undefined;
    const record = this.views.get(this.activeKey);
    if (!record || record.ref.threadId !== threadId || !record.controller.shown) {
      return undefined;
    }
    return record;
  }

  private anyRecordForThread(threadId: string): ViewRecord | undefined {
    for (const record of this.views.values()) {
      if (record.ref.threadId === threadId) return record;
    }
    return undefined;
  }

  private hideAllExcept(key: string | null): void {
    for (const [id, record] of this.views) {
      if (id === key) continue;
      record.controller.hide();
    }
  }
}
