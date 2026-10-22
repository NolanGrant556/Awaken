import { type BrowserWindow, type Session, type WebContents, shell } from "electron";
import {
  type EmbeddedBrowserState,
  type EmbeddedBrowserViewRect,
  deriveBrowserState,
  EMPTY_EMBEDDED_BROWSER_STATE,
  originOf,
  parseNavigable,
  resolveEmbeddedBrowserNavigation,
  safeExternalUrl,
  viewportBounds,
} from "../../shared/embedded-browser.js";
import {
  updatePreviewPrimaryPointerGesture,
  type PreviewPrimaryPointerGesture,
} from "../preview-primary-pointer.js";

const backstopInstalledFor = new WeakSet<Session>();
const VIEWPORT_RESTORE_POLL_MS = 16;

export type EmbeddedBrowserViewControllerOptions = {
  allowFile?: boolean;
  partition?: string;
  guest: WebContents;
  onPrimaryClick?: () => void;
  onHandoffHttp?: (url: string) => void;
};

export class EmbeddedBrowserViewController {
  private primaryPointerGesture: PreviewPrimaryPointerGesture | undefined;
  private readonly onPrimaryClick?: () => void;
  private destroyed = false;
  private shownWithBounds = false;
  private admittedOrigin = "";
  private readonly eventDisposers: Array<() => void> = [];
  documentRevision = 0;
  private navigationGeneration = 0;
  private navigationFailure: Error | null = null;
  private navigationInFlight: { url: string; promise: Promise<void> } | null = null;
  private readonly onState: (state: EmbeddedBrowserState) => void;
  private readonly allowFile: boolean;
  private readonly guest: WebContents;
  private readonly onHandoffHttp?: (url: string) => void;

  constructor(
    _window: BrowserWindow,
    onState: (state: EmbeddedBrowserState) => void,
    options: EmbeddedBrowserViewControllerOptions,
  ) {
    this.onState = onState;
    this.onPrimaryClick = options.onPrimaryClick;
    this.allowFile = Boolean(options.allowFile);
    this.onHandoffHttp = options.onHandoffHttp;
    if (!options.guest || options.guest.isDestroyed()) {
      throw new Error("embedded browser guest is not available");
    }
    this.guest = options.guest;
    this.applySecurityBackstop();
    this.wireEvents();
  }

  get loadsLocalFiles(): boolean {
    return this.allowFile;
  }

  get shown(): boolean {
    return this.shownWithBounds;
  }

  get hasViewport(): boolean {
    return this.shownWithBounds;
  }

  ownsGuest(guest: WebContents): boolean {
    return this.guest === guest;
  }

  private get wc() {
    return this.guest;
  }

  private applySecurityBackstop(): void {
    const { session } = this.wc;
    if (backstopInstalledFor.has(session)) return;
    backstopInstalledFor.add(session);
    session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
    session.setPermissionCheckHandler(() => false);
    session.on("will-download", (event) => event.preventDefault());
  }

  get authorizedOrigin(): string {
    return this.admittedOrigin;
  }

  private wireEvents(): void {
    const wc = this.wc;
    const listen = (name: string, listener: (...args: any[]) => void) => {
      wc.on(name as "did-navigate", listener);
      this.eventDisposers.push(() => wc.removeListener(name as "did-navigate", listener));
    };
    listen("before-mouse-event", (_event, mouse) => {
      if (!this.shown) {
        this.primaryPointerGesture = undefined;
        return;
      }
      const next = updatePreviewPrimaryPointerGesture(this.primaryPointerGesture, mouse);
      this.primaryPointerGesture = next.gesture;
      if (next.primaryClick) this.onPrimaryClick?.();
    });
    listen("did-start-navigation", (_event, _url, _inPlace, isMainFrame) => { if (_event?.isMainFrame ?? isMainFrame) this.documentRevision++; });
    listen("did-start-loading", () => this.emitState());
    listen("did-stop-loading", () => this.emitState());
    listen("did-navigate", () => this.recordNavigation());
    listen("did-navigate-in-page", () => this.recordNavigation());
    listen("page-title-updated", () => this.emitState());
    listen("did-fail-load", () => this.emitState());
    listen("destroyed", () => {
      this.destroyed = true;
    });

    wc.setWindowOpenHandler(({ url }) => {
      if (this.handoffHttp(url)) return { action: "deny" };
      const navigable = resolveEmbeddedBrowserNavigation(url, this.allowFile);
      if (navigable) {
        this.navigationGeneration++;
        void this.loadInternal(navigable).catch(() => undefined);
      }
      else this.openExternal(url);
      return { action: "deny" };
    });

    const guardNavigation = (event: Electron.Event, url: string) => {
      if (this.handoffHttp(url)) {
        event.preventDefault();
        return;
      }
      if (resolveEmbeddedBrowserNavigation(url, this.allowFile)) return;
      event.preventDefault();
      this.openExternal(url);
    };
    listen("will-navigate", guardNavigation);
    listen("will-redirect", guardNavigation);
  }

  private handoffHttp(url: string): boolean {
    if (!this.allowFile || !this.onHandoffHttp) return false;
    const httpUrl = parseNavigable(url);
    if (!httpUrl) return false;
    this.onHandoffHttp(httpUrl);
    return true;
  }

  private openExternal(url: string): void {
    const safe = safeExternalUrl(url);
    if (safe) void shell.openExternal(safe).catch(() => undefined);
  }

  private async loadInternal(url: string): Promise<void> {
    try {
      await this.wc.loadURL(url);
    } catch {
      throw new Error("browser_navigation_interrupted");
    }
  }

  private recordNavigation(): void {
    const origin = originOf(this.wc.getURL());
    if (origin !== this.admittedOrigin) this.admittedOrigin = "";
    this.emitState();
  }

  private emitState(): void {
    if (this.destroyed) return;
    this.onState(this.state());
  }

  state(): EmbeddedBrowserState {
    if (this.destroyed || this.wc.isDestroyed()) return EMPTY_EMBEDDED_BROWSER_STATE;
    const wc = this.wc;
    return deriveBrowserState({
      url: wc.getURL(),
      title: wc.getTitle(),
      canGoBack: wc.navigationHistory.canGoBack(),
      canGoForward: wc.navigationHistory.canGoForward(),
      loading: wc.isLoading(),
    });
  }

  currentURL(): string {
    if (this.destroyed || this.wc.isDestroyed()) return "";
    return this.wc.getURL();
  }

  admitOrigin(url: string): void {
    this.admittedOrigin = parseNavigable(url) ? originOf(url) : "";
  }

  originAdmitted(url: string): boolean {
    const origin = originOf(url);
    return !this.allowFile && Boolean(parseNavigable(url)) && origin !== "" && origin === this.admittedOrigin;
  }

  async navigate(url: string): Promise<EmbeddedBrowserState> {
    const navigable = resolveEmbeddedBrowserNavigation(url, this.allowFile);
    if (!navigable || (this.allowFile && parseNavigable(navigable))) {
      throw new Error("unsupported_scheme");
    }
    const generation = ++this.navigationGeneration;
    this.navigationFailure = null;
    this.admitOrigin(navigable);
    if (this.navigationInFlight?.url === navigable) {
      await this.navigationInFlight.promise;
    } else if (this.currentURL() !== navigable || this.wc.isLoading()) {
      const navigation = { url: navigable, promise: this.loadInternal(navigable) };
      this.navigationInFlight = navigation;
      try {
        await navigation.promise;
      } catch (error) {
        this.navigationFailure = error instanceof Error ? error : new Error("browser_navigation_interrupted");
        throw this.navigationFailure;
      } finally {
        if (this.navigationInFlight === navigation) this.navigationInFlight = null;
      }
    }
    if (generation !== this.navigationGeneration) throw new Error("browser_navigation_interrupted");
    return this.state();
  }

  async waitForNavigation(): Promise<void> {
    await this.navigationInFlight?.promise;
    if (this.navigationFailure) throw this.navigationFailure;
  }

  back(): EmbeddedBrowserState {
    if (this.wc.navigationHistory.canGoBack()) this.wc.navigationHistory.goBack();
    return this.state();
  }

  forward(): EmbeddedBrowserState {
    if (this.wc.navigationHistory.canGoForward()) this.wc.navigationHistory.goForward();
    return this.state();
  }

  reload(): EmbeddedBrowserState {
    if (!this.destroyed && !this.wc.isDestroyed()) this.wc.reload();
    return this.state();
  }

  stop(): EmbeddedBrowserState {
    if (!this.destroyed && !this.wc.isDestroyed()) this.wc.stop();
    return this.state();
  }

  setBounds(rect: EmbeddedBrowserViewRect | null): void {
    this.shownWithBounds = Boolean(viewportBounds(rect));
    if (!this.shownWithBounds) this.primaryPointerGesture = undefined;
  }

  hide(): void {
    this.setBounds(null);
  }

  async waitForLiveViewport(timeoutMs: number, signal?: AbortSignal): Promise<boolean> {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      if (signal?.aborted) return false;
      if (this.shownWithBounds) return true;
      await new Promise((resolve) => setTimeout(resolve, VIEWPORT_RESTORE_POLL_MS));
    }
    return this.shownWithBounds;
  }

  async executeJavaScript<T>(code: string): Promise<T> {
    if (this.destroyed || this.wc.isDestroyed()) {
      throw new Error("browser view is destroyed");
    }
    if (this.allowFile || !parseNavigable(this.currentURL())) {
      throw new Error("unsupported_scheme");
    }
    return this.wc.executeJavaScript(code, true) as Promise<T>;
  }

  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    this.shownWithBounds = false;
    for (const dispose of this.eventDisposers) dispose();
    this.eventDisposers.length = 0;
  }
}
