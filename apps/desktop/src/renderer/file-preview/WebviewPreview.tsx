import { useEffect, useRef, useState, type MutableRefObject } from "react";
import { AwakenIcon } from "@awaken/design-system";
import type { WebviewTag } from "electron";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

type WebviewPreviewHandle = ProjectFilePreviewUrlHandle & {
  kind: "office" | "html";
  partition: string;
};

export type WebviewNavigationHandle = {
  goBack: () => void;
  goForward: () => void;
  reload: () => void;
};

export type WebviewPreviewProps = {
  preview: WebviewPreviewHandle;
  onLoadingChange?: (loading: boolean) => void;
  onLoadError: (message: string) => void;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
  embedded?: boolean;
  navigationRef?: MutableRefObject<WebviewNavigationHandle | null>;
  onNavigationState?: (state: {
    canGoBack: boolean;
    canGoForward: boolean;
    address: string;
    loading: boolean;
  }) => void;
};

function visibleBrowserAddress(value: string, fallback: string): string {
  try {
    const url = new URL(value);
    if (url.protocol !== "awaken-preview:") return fallback;
    return decodeURIComponent(url.pathname) || `/${fallback}`;
  } catch {
    return `/${fallback}`;
  }
}

export default function WebviewPreview({
  preview,
  onLoadingChange,
  onLoadError,
  frameProps,
  embedded = false,
  navigationRef,
  onNavigationState,
}: WebviewPreviewProps) {
  const webviewRef = useRef<WebviewTag>(null);
  const domReadyRef = useRef(false);
  const onLoadingChangeRef = useRef(onLoadingChange);
  const onLoadErrorRef = useRef(onLoadError);
  const [canGoBack, setCanGoBack] = useState(false);
  const [canGoForward, setCanGoForward] = useState(false);
  const [address, setAddress] = useState(`/${preview.relativePath}`);

  const onNavigationStateRef = useRef(onNavigationState);
  onLoadingChangeRef.current = onLoadingChange;
  onLoadErrorRef.current = onLoadError;
  onNavigationStateRef.current = onNavigationState;

  useEffect(() => {
    const webview = webviewRef.current;
    if (!webview) return;
    const updateNavigation = () => {
      if (!domReadyRef.current || !webview.isConnected) return;
      try {
        const nextCanGoBack = webview.canGoBack();
        const nextCanGoForward = webview.canGoForward();
        const nextAddress = visibleBrowserAddress(webview.getURL(), preview.relativePath);
        setCanGoBack(nextCanGoBack);
        setCanGoForward(nextCanGoForward);
        setAddress(nextAddress);
        onNavigationStateRef.current?.({
          canGoBack: nextCanGoBack,
          canGoForward: nextCanGoForward,
          address: nextAddress,
          loading: webview.isLoading?.() ?? false,
        });
      } catch {
        // Electron rejects guest methods until dom-ready and after DOM detachment.
      }
    };
    const markReady = () => {
      domReadyRef.current = true;
      updateNavigation();
    };
    const startLoading = () => onLoadingChangeRef.current?.(true);
    const stopLoading = () => {
      onLoadingChangeRef.current?.(false);
      updateNavigation();
    };
    const failLoading = (event: Event) => {
      const details = event as Event & {
        errorCode?: number;
        errorDescription?: string;
        isMainFrame?: boolean;
      };
      if (details.errorCode === -3 || details.isMainFrame === false) return;
      onLoadingChangeRef.current?.(false);
      onLoadErrorRef.current(details.errorDescription || "文件内容加载失败。");
    };
    const rendererGone = () => {
      onLoadingChangeRef.current?.(false);
      onLoadErrorRef.current("预览页面意外停止运行，请重试。");
    };
    webview.addEventListener("dom-ready", markReady);
    webview.addEventListener("did-start-loading", startLoading);
    webview.addEventListener("did-stop-loading", stopLoading);
    webview.addEventListener("did-navigate", updateNavigation);
    webview.addEventListener("did-navigate-in-page", updateNavigation);
    webview.addEventListener("did-fail-load", failLoading);
    webview.addEventListener("render-process-gone", rendererGone);
    return () => {
      domReadyRef.current = false;
      webview.removeEventListener("dom-ready", markReady);
      webview.removeEventListener("did-start-loading", startLoading);
      webview.removeEventListener("did-stop-loading", stopLoading);
      webview.removeEventListener("did-navigate", updateNavigation);
      webview.removeEventListener("did-navigate-in-page", updateNavigation);
      webview.removeEventListener("did-fail-load", failLoading);
      webview.removeEventListener("render-process-gone", rendererGone);
    };
  }, [preview.previewId, preview.relativePath]);

  const runGuestAction = (action: (webview: WebviewTag) => void) => {
    const webview = webviewRef.current;
    if (!webview || !domReadyRef.current || !webview.isConnected) return;
    try {
      action(webview);
    } catch {
      onLoadErrorRef.current("预览页面暂时不可用，请重试。");
    }
  };

  if (navigationRef) {
    navigationRef.current = {
      goBack: () => runGuestAction((webview) => webview.goBack()),
      goForward: () => runGuestAction((webview) => webview.goForward()),
      reload: () => runGuestAction((webview) => webview.reload()),
    };
  }

  const guest = (
    <webview
      key={preview.previewId}
      ref={webviewRef}
      className="project-file-webview-preview__guest"
      src={preview.src}
      partition={preview.partition}
      webpreferences="contextIsolation=yes,nodeIntegration=no,sandbox=yes"
    />
  );

  return (
    <PreviewFrame
      {...frameProps}
      hideChrome={embedded}
      formatLabel={preview.kind === "html" ? "HTML" : "Office"}
      formatActions={
        embedded || preview.kind !== "html" ? undefined : (
          <>
            <button
              type="button"
              aria-label="后退"
              title="后退"
              disabled={!canGoBack}
              onClick={() => runGuestAction((webview) => webview.goBack())}
            >
              <AwakenIcon concept="navigate-back" size={16} />
            </button>
            <button
              type="button"
              aria-label="前进"
              title="前进"
              disabled={!canGoForward}
              onClick={() => runGuestAction((webview) => webview.goForward())}
            >
              <AwakenIcon concept="forward" size={16} />
            </button>
            <button
              type="button"
              aria-label="刷新页面"
              title="刷新页面"
              onClick={() => runGuestAction((webview) => webview.reload())}
            >
              <AwakenIcon concept="refresh" size={16} />
            </button>
            <output
              className="project-file-preview-heading__address"
              aria-label="当前地址"
            >
              {address}
            </output>
          </>
        )
      }
    >
      <div
        className={`project-file-webview-preview${
          embedded ? " is-embedded" : ""
        }`}
      >
        {guest}
      </div>
    </PreviewFrame>
  );
}
