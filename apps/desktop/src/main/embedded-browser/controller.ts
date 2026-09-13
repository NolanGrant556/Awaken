import { type BrowserWindow, type Session, shell, WebContentsView } from "electron";
import {
  type EmbeddedBrowserState,
  type EmbeddedBrowserViewRect,
  deriveBrowserState,
  EMPTY_EMBEDDED_BROWSER_STATE,
  originOf,
  parseNavigable,
  safeExternalUrl,
  viewportBounds,
} from "../../shared/embedded-browser.js";
import { browserViewWebPreferences } from "./options.js";

const backstopInstalledFor = new WeakSet<Session>();
const VIEWPORT_RESTORE_POLL_MS = 16;

export class EmbeddedBrowserViewController {
  private readonly view: WebContentsView;
  private destroyed = false;
  private shownWithBounds = false;
  private admittedOrigin = "";
  private readonly onState: (state: EmbeddedBrowserState) => void;

  constructor(
    private readonly window: BrowserWindow,
    onState: (state: EmbeddedBrowserState) => void,
  ) {
    this.onState = onState;
    this.view = new WebContentsView({ webPreferences: browserViewWebPreferences() });
    this.window.contentView.addChildView(this.view);
    this.view.setVisible(false);
    this.applySecurityBackstop();
    this.wireEvents();
  }

  get shown(): boolean {
    return this.shownWithBounds;
  }

  get hasViewport(): boolean {
    return this.shownWithBounds;
  }

  private get wc() {
    return this.view.webContents;
  }

  private applySecurityBackstop(): void {
    const { session } = this.wc;
    if (backstopInstalledFor.has(session)) return;
    backstopInstalledFor.add(session);
    session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
    session.setPermissionCheckHandler(() => false);
    session.on("will-download", (event) => event.preventDefault());
  }

  private wireEvents(): void {
    const wc = this.wc;
    wc.on("did-start-loading", () => this.emitState());
    wc.on("did-stop-loading", () => this.emitState());
    wc.on("did-navigate", () => this.recordNavigation());
    wc.on("did-navigate-in-page", () => this.recordNavigation());
    wc.on("page-title-updated", () => this.emitState());
    wc.on("did-fail-load", () => this.emitState());

    wc.setWindowOpenHandler(({ url }) => {
      const navigable = parseNavigable(url);
      if (navigable) void this.loadInternal(navigable);
      else this.openExternal(url);
      return { action: "deny" };
    });

    wc.on("will-navigate", (event, url) => {
      if (parseNavigable(url)) return;
      event.preventDefault();
      this.openExternal(url);
    });
  }

  private openExternal(url: string): void {
    const safe = safeExternalUrl(url);
    if (safe) void shell.openExternal(safe).catch(() => undefined);
  }

  private async loadInternal(url: string): Promise<void> {
    try {
      await this.wc.loadURL(url);
    } catch {
      /* surfaced via did-fail-load */
    }
  }

  private recordNavigation(): void {
    const origin = originOf(this.wc.getURL());
    if (origin) this.admittedOrigin = origin;
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
    const origin = originOf(url);
    if (origin) this.admittedOrigin = origin;
  }

  originAdmitted(url: string): boolean {
    const origin = originOf(url);
    return origin !== "" && origin === this.admittedOrigin;
  }

  async navigate(url: string): Promise<EmbeddedBrowserState> {
    const navigable = parseNavigable(url);
    if (!navigable) return this.state();
    this.admitOrigin(navigable);
    await this.loadInternal(navigable);
    return this.state();
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
    const bounds = viewportBounds(rect);
    if (!bounds) {
      this.shownWithBounds = false;
      this.view.setVisible(false);
      this.view.setBounds({ x: 0, y: 0, width: 0, height: 0 });
      return;
    }
    this.view.setBounds(bounds);
    this.view.setVisible(true);
    this.shownWithBounds = true;
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
    return this.wc.executeJavaScript(code, true) as Promise<T>;
  }

  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    this.hide();
    try {
      this.window.contentView.removeChildView(this.view);
    } catch {
      /* already detached */
    }
    if (!this.wc.isDestroyed()) this.wc.close({ waitForBeforeUnload: false });
  }
}
