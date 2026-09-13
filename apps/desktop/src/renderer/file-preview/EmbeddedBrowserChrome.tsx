import { useEffect, useRef, useState, type FormEvent } from "react";
import type { WebviewTag } from "electron";
import { AwakenIcon } from "@awaken/design-system";
import {
  EMBEDDED_BROWSER_EMPTY_HINT,
  EMBEDDED_BROWSER_OVERLAY_SELECTOR,
  EMBEDDED_BROWSER_PARTITION,
  EMPTY_EMBEDDED_BROWSER_STATE,
  embeddedBrowserGuestWebPreferences,
  localHtmlPartition,
  normalizeBrowserAddressInput,
  type EmbeddedBrowserBridge,
  type EmbeddedBrowserState,
  type EmbeddedBrowserTabRef,
} from "../../shared/embedded-browser";

export type EmbeddedBrowserClient = Pick<
  EmbeddedBrowserBridge,
  | "navigate"
  | "back"
  | "forward"
  | "reload"
  | "stop"
  | "close"
  | "getState"
  | "setViewport"
  | "setActive"
  | "attachGuest"
  | "detachGuest"
  | "onStateChanged"
>;

export type EmbeddedHtmlNavigation = {
  canGoBack: boolean;
  canGoForward: boolean;
  address: string;
  loading: boolean;
  goBack: () => void;
  goForward: () => void;
  reload: () => void;
};

type GuestSession = {
  partition: string;
  src: string;
  allowFile: boolean;
};

type Props = {
  client: EmbeddedBrowserClient;
  viewRef: EmbeddedBrowserTabRef;
  enabled?: boolean;
  active: boolean;
  htmlOccupied?: boolean;
  onLeaveHtmlPreview?: () => void;
  htmlNavigation?: EmbeddedHtmlNavigation;
  children?: React.ReactNode;
};

export function overlayCoversEmbeddedBrowser(): boolean {
  for (const node of document.querySelectorAll(EMBEDDED_BROWSER_OVERLAY_SELECTOR)) {
    if (!(node instanceof HTMLElement)) continue;
    const style = getComputedStyle(node);
    if (style.display === "none" || style.visibility === "hidden") continue;
    const rect = node.getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0) return true;
  }
  return false;
}

function guestSessionFor(
  url: string,
  viewId: string,
): GuestSession | null {
  const parsed = normalizeBrowserAddressInput(url);
  if (!parsed.ok) return null;
  const allowFile = parsed.kind === "file";
  return {
    partition: allowFile
      ? localHtmlPartition(viewId)
      : EMBEDDED_BROWSER_PARTITION,
    src: parsed.url,
    allowFile,
  };
}

function sessionMatches(current: GuestSession | null, next: GuestSession): boolean {
  return current?.partition === next.partition && current.allowFile === next.allowFile;
}

export default function EmbeddedBrowserChrome({
  client,
  viewRef,
  enabled = true,
  active,
  htmlOccupied = false,
  onLeaveHtmlPreview,
  htmlNavigation,
  children,
}: Props) {
  const stripRef = useRef<HTMLDivElement>(null);
  const webviewRef = useRef<WebviewTag>(null);
  const htmlOccupiedRef = useRef(htmlOccupied);
  const [state, setState] = useState<EmbeddedBrowserState>(EMPTY_EMBEDDED_BROWSER_STATE);
  const [address, setAddress] = useState("");
  const [guestSession, setGuestSession] = useState<GuestSession | null>(null);
  const [guestSnapshot, setGuestSnapshot] = useState<string | null>(null);
  const guestSnapshotRef = useRef<string | null>(null);
  const snapshotInFlightRef = useRef(false);
  const editingRef = useRef(false);
  const [addressError, setAddressError] = useState<string | undefined>();
  htmlOccupiedRef.current = htmlOccupied;

  useEffect(() => {
    let disposed = false;
    editingRef.current = false;
    setAddressError(undefined);
    void client.getState(viewRef).then((next) => {
      if (disposed) return;
      setState(next);
      if (!editingRef.current && !htmlOccupiedRef.current) setAddress(next.url);
    });
    const unsubscribe = client.onStateChanged((payload) => {
      if (payload.viewId !== viewRef.viewId || payload.threadId !== viewRef.threadId) return;
      setState(payload.state);
      if (!editingRef.current && !htmlOccupiedRef.current) {
        setAddress(payload.state.url);
      }
    });
    return () => {
      disposed = true;
      unsubscribe();
    };
  }, [client, viewRef.threadId, viewRef.viewId]);

  useEffect(() => {
    if (editingRef.current || !htmlNavigation) return;
    if (htmlOccupied || !state.hasPage) setAddress(htmlNavigation.address);
  }, [htmlNavigation, htmlOccupied, state.hasPage]);

  useEffect(() => {
    if (!enabled || !active) {
      void client.setActive(null);
      return;
    }
    void client.setActive(viewRef);
    return () => {
      void client.setActive(null);
    };
  }, [active, client, enabled, viewRef.threadId, viewRef.viewId]);

  useEffect(() => {
    if (!enabled || !active || htmlOccupied) {
      setGuestSession(null);
      return;
    }
    const next = state.url ? guestSessionFor(state.url, viewRef.viewId) : null;
    if (!next) return;
    setGuestSession((current) => (sessionMatches(current, next) ? current : next));
  }, [active, enabled, htmlOccupied, state.url, viewRef.viewId]);

  const showPage = enabled && active && Boolean(guestSession) && !htmlOccupied;
  guestSnapshotRef.current = guestSnapshot;
  useEffect(() => {
    if (!showPage) {
      void client.setViewport(viewRef, null);
      setGuestSnapshot(null);
      snapshotInFlightRef.current = false;
      return;
    }
    let frame = 0;
    const report = () => {
      const covered = overlayCoversEmbeddedBrowser();
      if (covered) {
        void client.setViewport(viewRef, null);
        const webview = webviewRef.current as
          | (WebviewTag & {
              capturePage?: () => Promise<{ toDataURL?: () => string }>;
            })
          | null;
        if (
          webview &&
          !guestSnapshotRef.current &&
          !snapshotInFlightRef.current &&
          typeof webview.capturePage === "function"
        ) {
          snapshotInFlightRef.current = true;
          void webview
            .capturePage()
            .then((image) => {
              snapshotInFlightRef.current = false;
              const dataUrl = image?.toDataURL?.();
              if (dataUrl && overlayCoversEmbeddedBrowser()) {
                setGuestSnapshot(dataUrl);
              }
            })
            .catch(() => {
              snapshotInFlightRef.current = false;
            });
        }
      } else {
        if (guestSnapshotRef.current) setGuestSnapshot(null);
        const rect = stripRef.current?.getBoundingClientRect();
        void client.setViewport(
          viewRef,
          rect
            ? { x: rect.x, y: rect.y, width: rect.width, height: rect.height }
            : null,
        );
      }
      frame = requestAnimationFrame(report);
    };
    frame = requestAnimationFrame(report);
    return () => {
      cancelAnimationFrame(frame);
      void client.setViewport(viewRef, null);
    };
  }, [client, showPage, viewRef.threadId, viewRef.viewId]);

  useEffect(() => {
    const webview = webviewRef.current;
    const session = guestSession;
    if (!enabled || !active || htmlOccupied || !session || !webview) {
      return;
    }
    const attach = () => {
      try {
        void client.attachGuest(viewRef, webview.getWebContentsId(), session.allowFile);
      } catch {
        /* guest not attached yet */
      }
    };
    webview.addEventListener("did-attach", attach);
    webview.addEventListener("dom-ready", attach);
    attach();
    return () => {
      webview.removeEventListener("did-attach", attach);
      webview.removeEventListener("dom-ready", attach);
      void client.detachGuest(viewRef);
    };
  }, [
    active,
    client,
    enabled,
    guestSession,
    htmlOccupied,
    viewRef.threadId,
    viewRef.viewId,
  ]);

  const showingHtml =
    enabled && (htmlOccupied || (Boolean(htmlNavigation) && !state.hasPage));
  const toolbarCanGoBack = showingHtml
    ? Boolean(htmlNavigation?.canGoBack)
    : state.canGoBack;
  const toolbarCanGoForward = showingHtml
    ? Boolean(htmlNavigation?.canGoForward)
    : state.canGoForward;
  const toolbarLoading = showingHtml
    ? Boolean(htmlNavigation?.loading)
    : state.loading;

  const submitAddress = (event: FormEvent) => {
    event.preventDefault();
    const parsed = normalizeBrowserAddressInput(address);
    if (!parsed.ok) {
      setAddressError(
        parsed.reason === "unsupported_scheme"
          ? "只支持 http、https 或本地 HTML"
          : parsed.reason === "empty"
            ? `请${EMBEDDED_BROWSER_EMPTY_HINT}`
            : "地址无效",
      );
      return;
    }
    setAddressError(undefined);
    editingRef.current = false;
    setAddress(parsed.url);
    if (parsed.kind === "http") onLeaveHtmlPreview?.();
    const next = guestSessionFor(parsed.url, viewRef.viewId);
    if (next) {
      setGuestSession((current) => (sessionMatches(current, next) ? current : next));
    }
    void client.navigate(viewRef, parsed.url);
  };

  const emptyState = (
    <div className="project-files-preview-empty is-centered">
      <AwakenIcon concept="browser" size={24} />
      <strong>浏览器</strong>
      <span>{EMBEDDED_BROWSER_EMPTY_HINT}</span>
    </div>
  );

  return (
    <div
      className={enabled ? "embedded-browser" : "file-preview-host-shell"}
    >
      {enabled ? (
        <form className="embedded-browser__toolbar" onSubmit={submitAddress}>
          <button
            type="button"
            aria-label="后退"
            title="后退"
            disabled={!toolbarCanGoBack}
            onClick={() =>
              showingHtml ? htmlNavigation?.goBack() : void client.back(viewRef)
            }
          >
            <AwakenIcon concept="navigate-back" size={16} />
          </button>
          <button
            type="button"
            aria-label="前进"
            title="前进"
            disabled={!toolbarCanGoForward}
            onClick={() =>
              showingHtml
                ? htmlNavigation?.goForward()
                : void client.forward(viewRef)
            }
          >
            <AwakenIcon concept="forward" size={16} />
          </button>
          <button
            type="button"
            aria-label={toolbarLoading ? "停止加载" : "刷新页面"}
            title={toolbarLoading ? "停止加载" : "刷新页面"}
            onClick={() => {
              if (showingHtml) {
                htmlNavigation?.reload();
                return;
              }
              void (state.loading ? client.stop(viewRef) : client.reload(viewRef));
            }}
          >
            <AwakenIcon concept={toolbarLoading ? "close" : "refresh"} size={16} />
          </button>
          <input
            className="embedded-browser__address"
            aria-label="当前地址"
            placeholder={EMBEDDED_BROWSER_EMPTY_HINT}
            value={address}
            onChange={(event) => {
              editingRef.current = true;
              setAddressError(undefined);
              setAddress(event.target.value);
            }}
            onFocus={() => {
              editingRef.current = true;
            }}
            onBlur={() => {
              editingRef.current = false;
              if (!address.trim()) {
                setAddress(showingHtml ? (htmlNavigation?.address ?? "") : state.url);
              }
            }}
          />
        </form>
      ) : null}
      {enabled && addressError ? (
        <p className="embedded-browser__error" role="alert">
          {addressError}
        </p>
      ) : null}
      {showPage && guestSession ? (
        <div
          key="native-page"
          ref={stripRef}
          className="embedded-browser__page"
        >
          <webview
            key={guestSession.partition}
            ref={webviewRef}
            className={
              guestSnapshot
                ? "embedded-browser__guest is-covered"
                : "embedded-browser__guest"
            }
            src={guestSession.src}
            partition={guestSession.partition}
            webpreferences={embeddedBrowserGuestWebPreferences(
              guestSession.allowFile,
            )}
          />
          {guestSnapshot ? (
            <img
              className="embedded-browser__guest-snapshot"
              src={guestSnapshot}
              alt=""
            />
          ) : null}
        </div>
      ) : null}
      <div
        key="preview-body"
        className={enabled && !showPage ? "embedded-browser__page" : undefined}
        hidden={showPage || undefined}
        aria-hidden={showPage || undefined}
      >
        {children ?? (enabled ? emptyState : null)}
      </div>
    </div>
  );
}
