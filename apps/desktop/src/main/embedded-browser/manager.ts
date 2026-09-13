import type { BrowserWindow } from "electron";
import {
  browserActionAllowed,
  EMPTY_EMBEDDED_BROWSER_STATE,
  type EmbeddedBrowserActionKind,
  type EmbeddedBrowserState,
  type EmbeddedBrowserTabRef,
  type EmbeddedBrowserViewRect,
  originOf,
  parseNavigable,
} from "../../shared/embedded-browser.js";
import { EmbeddedBrowserViewController } from "./controller.js";

const VIEWPORT_RESTORE_WAIT_MS = 4000;

export type EmbeddedBrowserViewManagerOptions = {
  getWindow: () => BrowserWindow | null;
  onState: (viewId: string, threadId: string, state: EmbeddedBrowserState) => void;
  onRequestShow: (threadId: string, viewId: string) => void;
  createView?: (
    window: BrowserWindow,
    onState: (state: EmbeddedBrowserState) => void,
  ) => EmbeddedBrowserViewController;
};

type ViewRecord = {
  ref: EmbeddedBrowserTabRef;
  controller: EmbeddedBrowserViewController;
};

function viewKey(ref: EmbeddedBrowserTabRef): string {
  return `${ref.threadId}::${ref.viewId}`;
}

export class EmbeddedBrowserViewManager {
  private readonly views = new Map<string, ViewRecord>();
  private activeKey: string | null = null;
  private pendingActive: EmbeddedBrowserTabRef | null = null;

  constructor(private readonly options: EmbeddedBrowserViewManagerOptions) {}

  getState(ref: EmbeddedBrowserTabRef): EmbeddedBrowserState {
    return this.views.get(viewKey(ref))?.controller.state() ?? EMPTY_EMBEDDED_BROWSER_STATE;
  }

  async navigate(ref: EmbeddedBrowserTabRef, url: string): Promise<EmbeddedBrowserState> {
    const navigable = parseNavigable(url);
    if (!navigable) {
      throw new Error("unsupported_scheme");
    }
    const controller = this.ensure(ref);
    this.activeKey = viewKey(ref);
    this.pendingActive = ref;
    this.hideAllExcept(this.activeKey);
    if (!controller.shown) controller.hide();
    return controller.navigate(navigable);
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
    if (!record) return;
    record.controller.destroy();
    this.views.delete(key);
    if (this.activeKey === key) this.activeKey = null;
    if (this.pendingActive && viewKey(this.pendingActive) === key) {
      this.pendingActive = null;
    }
  }

  hideAll(): void {
    this.activeKey = null;
    this.pendingActive = null;
    this.hideAllExcept(null);
  }

  closeThread(threadId: string): void {
    for (const [key, record] of this.views) {
      if (record.ref.threadId !== threadId) continue;
      record.controller.destroy();
      this.views.delete(key);
      if (this.activeKey === key) this.activeKey = null;
    }
    if (this.pendingActive?.threadId === threadId) this.pendingActive = null;
  }

  dispose(): void {
    for (const record of this.views.values()) record.controller.destroy();
    this.views.clear();
    this.activeKey = null;
    this.pendingActive = null;
  }

  viewForAction(threadId: string, kind: EmbeddedBrowserActionKind): EmbeddedBrowserViewController | null {
    const shown = this.shownRecordForThread(threadId);
    if (!shown) return null;
    if (!browserActionAllowed(kind, shown.controller)) return null;
    return shown.controller;
  }

  async ensureVisibleForAgent(threadId: string): Promise<EmbeddedBrowserViewController> {
    const existing = this.shownRecordForThread(threadId) ?? this.anyRecordForThread(threadId);
    const hint = existing?.ref.viewId ?? (this.pendingActive?.threadId === threadId ? this.pendingActive.viewId : "");
    this.options.onRequestShow(threadId, hint);
    const deadline = Date.now() + VIEWPORT_RESTORE_WAIT_MS;
    while (Date.now() < deadline) {
      const shown = this.shownRecordForThread(threadId);
      if (shown) return shown.controller;
      const pending =
        this.pendingActive?.threadId === threadId ? this.pendingActive : null;
      if (pending) {
        const controller = this.ensure(pending);
        this.activeKey = viewKey(pending);
        this.hideAllExcept(this.activeKey);
        return controller;
      }
      const any = this.anyRecordForThread(threadId);
      if (any) {
        this.activeKey = viewKey(any.ref);
        this.pendingActive = any.ref;
        this.hideAllExcept(this.activeKey);
        return any.controller;
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

  private ensure(ref: EmbeddedBrowserTabRef): EmbeddedBrowserViewController {
    const key = viewKey(ref);
    const existing = this.views.get(key);
    if (existing) return existing.controller;
    const window = this.options.getWindow();
    if (!window || window.isDestroyed()) {
      throw new Error("main window is not available");
    }
    const create =
      this.options.createView ??
      ((owner, onState) => new EmbeddedBrowserViewController(owner, onState));
    const controller = create(window, (state) => {
      this.options.onState(ref.viewId, ref.threadId, state);
    });
    this.views.set(key, { ref, controller });
    return controller;
  }

  private hideAllExcept(key: string | null): void {
    for (const [id, record] of this.views) {
      if (id === key) continue;
      record.controller.hide();
    }
  }
}
