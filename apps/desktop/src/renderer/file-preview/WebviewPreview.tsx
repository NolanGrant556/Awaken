import { useEffect, useRef, useState } from "react";
import type { WebviewTag } from "electron";
import { ArrowLeft, ArrowRight, RefreshCw } from "lucide-react";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

type WebviewPreviewHandle = ProjectFilePreviewUrlHandle & {
  kind: "office" | "html";
  partition: string;
};

export type WebviewPreviewProps = {
  preview: WebviewPreviewHandle;
  onLoadingChange?: (loading: boolean) => void;
  onLoadError: (message: string) => void;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
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
}: WebviewPreviewProps) {
  const webviewRef = useRef<WebviewTag>(null);
  const domReadyRef = useRef(false);
  const onLoadingChangeRef = useRef(onLoadingChange);
  const onLoadErrorRef = useRef(onLoadError);
  const [canGoBack, setCanGoBack] = useState(false);
  const [canGoForward, setCanGoForward] = useState(false);
  const [address, setAddress] = useState(`/${preview.relativePath}`);

  onLoadingChangeRef.current = onLoadingChange;
  onLoadErrorRef.current = onLoadError;

  useEffect(() => {
    const webview = webviewRef.current;
    if (!webview) return;
    const updateNavigation = () => {
      if (!domReadyRef.current || !webview.isConnected) return;
      try {
        setCanGoBack(webview.canGoBack());
        setCanGoForward(webview.canGoForward());
        setAddress(
          visibleBrowserAddress(webview.getURL(), preview.relativePath),
        );
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

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel={preview.kind === "html" ? "HTML" : "Office"}
      formatActions={
        preview.kind === "html" ? (
          <>
            <button
              type="button"
              aria-label="后退"
              title="后退"
              disabled={!canGoBack}
              onClick={() => runGuestAction((webview) => webview.goBack())}
            >
              <ArrowLeft size={16} aria-hidden="true" />
            </button>
            <button
              type="button"
              aria-label="前进"
              title="前进"
              disabled={!canGoForward}
              onClick={() => runGuestAction((webview) => webview.goForward())}
            >
              <ArrowRight size={16} aria-hidden="true" />
            </button>
            <button
              type="button"
              aria-label="刷新页面"
              title="刷新页面"
              onClick={() => runGuestAction((webview) => webview.reload())}
            >
              <RefreshCw size={15} aria-hidden="true" />
            </button>
            <output
              className="project-file-preview-heading__address"
              aria-label="当前地址"
            >
              {address}
            </output>
          </>
        ) : undefined
      }
    >
      <webview
        key={preview.previewId}
        ref={webviewRef}
        className="project-file-webview-preview__guest"
        src={preview.src}
        partition={preview.partition}
        webpreferences="contextIsolation=yes,nodeIntegration=no,sandbox=yes"
      />
    </PreviewFrame>
  );
}
