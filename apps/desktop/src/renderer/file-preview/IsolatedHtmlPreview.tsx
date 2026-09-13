import { useEffect, useRef, useState } from "react";
import type { WebviewTag } from "electron";
import { parseNavigable } from "../../shared/embedded-browser";
import type { EmbeddedHtmlNavigation } from "./EmbeddedBrowserChrome";

type GuestEvent = Event & {
  url?: string;
  errorCode?: number;
  errorDescription?: string;
  isMainFrame?: boolean;
};

type Props = {
  src: string;
  partition: string;
  onHttpNavigate: (url: string) => void;
  onNavigationChange: (navigation: EmbeddedHtmlNavigation | undefined) => void;
};

const WEB_PREFERENCES =
  "contextIsolation=yes,nodeIntegration=no,sandbox=yes,webSecurity=yes,allowFileAccessFromFileUrls=yes";

function guestUrl(webview: WebviewTag | null, fallback: string): string {
  try {
    const current = webview?.getURL?.();
    if (current) return current;
  } catch {
    /* detached */
  }
  return fallback;
}

export default function IsolatedHtmlPreview({
  src,
  partition,
  onHttpNavigate,
  onNavigationChange,
}: Props) {
  const webviewRef = useRef<WebviewTag>(null);
  const srcRef = useRef(src);
  const onHttpNavigateRef = useRef(onHttpNavigate);
  const onNavigationChangeRef = useRef(onNavigationChange);
  const [address, setAddress] = useState(src);
  const [loading, setLoading] = useState(true);
  const [canGoBack, setCanGoBack] = useState(false);
  const [canGoForward, setCanGoForward] = useState(false);
  const [loadError, setLoadError] = useState<string | undefined>();

  srcRef.current = src;
  onHttpNavigateRef.current = onHttpNavigate;
  onNavigationChangeRef.current = onNavigationChange;

  const syncHistory = () => {
    const webview = webviewRef.current;
    setCanGoBack(Boolean(webview?.canGoBack?.()));
    setCanGoForward(Boolean(webview?.canGoForward?.()));
  };

  useEffect(() => {
    setAddress(src);
    setLoadError(undefined);
    setLoading(true);
    const webview = webviewRef.current;
    if (!webview?.loadURL) return;
    try {
      if (guestUrl(webview, "") !== src) void webview.loadURL(src);
    } catch {
      /* guest not attached yet; src attribute handles the first load */
    }
  }, [src]);

  useEffect(() => {
    const goBack = () => {
      try {
        webviewRef.current?.goBack?.();
      } catch {
        /* detached */
      }
    };
    const goForward = () => {
      try {
        webviewRef.current?.goForward?.();
      } catch {
        /* detached */
      }
    };
    const reload = () => {
      setLoadError(undefined);
      try {
        webviewRef.current?.reload?.();
      } catch {
        /* detached */
      }
    };
    onNavigationChangeRef.current({
      address,
      loading,
      canGoBack,
      canGoForward,
      goBack,
      goForward,
      reload,
    });
  }, [address, canGoBack, canGoForward, loading]);

  useEffect(
    () => () => {
      onNavigationChangeRef.current(undefined);
    },
    [],
  );

  useEffect(() => {
    const webview = webviewRef.current;
    if (!webview) return;

    const intercept = (url: string | undefined, event: Event) => {
      if (!url) return;
      const httpUrl = parseNavigable(url);
      if (httpUrl) {
        event.preventDefault();
        onHttpNavigateRef.current(httpUrl);
        return;
      }
      try {
        if (new URL(url).protocol !== "file:") event.preventDefault();
      } catch {
        event.preventDefault();
      }
    };

    const startLoading = () => {
      setLoading(true);
      setLoadError(undefined);
    };
    const stopLoading = () => {
      setLoading(false);
      setAddress(guestUrl(webview, srcRef.current));
      syncHistory();
    };
    const navigated = (event: Event) => {
      const next = (event as GuestEvent).url || guestUrl(webview, srcRef.current);
      setAddress(next);
      setLoading(false);
      syncHistory();
    };
    const willNavigate = (event: Event) => {
      intercept((event as GuestEvent).url, event);
    };
    const newWindow = (event: Event) => {
      intercept((event as GuestEvent).url, event);
    };
    const failLoading = (event: Event) => {
      const details = event as GuestEvent;
      if (details.errorCode === -3 || details.isMainFrame === false) return;
      setLoading(false);
      setLoadError(details.errorDescription || "无法打开此本地页面。");
    };

    webview.addEventListener("did-start-loading", startLoading);
    webview.addEventListener("did-stop-loading", stopLoading);
    webview.addEventListener("did-navigate", navigated);
    webview.addEventListener("did-navigate-in-page", navigated);
    webview.addEventListener("will-navigate", willNavigate);
    webview.addEventListener("new-window", newWindow);
    webview.addEventListener("did-fail-load", failLoading);
    return () => {
      webview.removeEventListener("did-start-loading", startLoading);
      webview.removeEventListener("did-stop-loading", stopLoading);
      webview.removeEventListener("did-navigate", navigated);
      webview.removeEventListener("did-navigate-in-page", navigated);
      webview.removeEventListener("will-navigate", willNavigate);
      webview.removeEventListener("new-window", newWindow);
      webview.removeEventListener("did-fail-load", failLoading);
    };
  }, []);

  return (
    <div className="project-file-webview-preview is-embedded">
      <webview
        ref={webviewRef}
        className="project-file-webview-preview__guest"
        src={src}
        partition={partition}
        webpreferences={WEB_PREFERENCES}
      />
      {loadError ? (
        <p className="embedded-browser__error" role="alert">
          {loadError}
        </p>
      ) : null}
    </div>
  );
}
