import { useEffect, useRef } from "react";
import type { WebviewTag } from "electron";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

type WebviewPreviewHandle = ProjectFilePreviewUrlHandle & {
  kind: "office";
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

export default function WebviewPreview({
  preview,
  onLoadingChange,
  onLoadError,
  frameProps,
}: WebviewPreviewProps) {
  const webviewRef = useRef<WebviewTag>(null);
  const onLoadingChangeRef = useRef(onLoadingChange);
  const onLoadErrorRef = useRef(onLoadError);
  onLoadingChangeRef.current = onLoadingChange;
  onLoadErrorRef.current = onLoadError;

  useEffect(() => {
    const webview = webviewRef.current;
    if (!webview) return;
    const startLoading = () => onLoadingChangeRef.current?.(true);
    const stopLoading = () => onLoadingChangeRef.current?.(false);
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
    webview.addEventListener("did-start-loading", startLoading);
    webview.addEventListener("did-stop-loading", stopLoading);
    webview.addEventListener("did-fail-load", failLoading);
    webview.addEventListener("render-process-gone", rendererGone);
    return () => {
      webview.removeEventListener("did-start-loading", startLoading);
      webview.removeEventListener("did-stop-loading", stopLoading);
      webview.removeEventListener("did-fail-load", failLoading);
      webview.removeEventListener("render-process-gone", rendererGone);
    };
  }, [preview.previewId]);

  return (
    <PreviewFrame {...frameProps} formatLabel="Office">
      <div className="project-file-webview-preview">
        <webview
          key={preview.previewId}
          ref={webviewRef}
          className="project-file-webview-preview__guest"
          src={preview.src}
          partition={preview.partition}
          webpreferences="contextIsolation=yes,nodeIntegration=no,sandbox=yes"
        />
      </div>
    </PreviewFrame>
  );
}
