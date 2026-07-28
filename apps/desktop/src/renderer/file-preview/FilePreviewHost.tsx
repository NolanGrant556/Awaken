import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { ExternalLink, FolderSearch2, Globe2, RefreshCw } from "lucide-react";
import type {
  ProjectFilePreviewActionRequest,
  ProjectFilePreviewHandle,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import ImagePreview from "./ImagePreview";
import PreviewErrorBoundary from "./PreviewErrorBoundary";
import WebviewPreview from "./WebviewPreview";

const MarkdownPreview = lazy(() => import("./MarkdownPreview"));
const CodePreview = lazy(() => import("./CodePreview"));
const PdfPreview = lazy(() => import("./PdfPreview"));

export type ProjectFileSelection = {
  name: string;
  relativePath: string;
};

export type ProjectFileWorkspaceMode = "files" | "browser";

export type FilePreviewHostProps = {
  runtimeClient: Pick<
    RuntimeClient,
    | "startProjectFilePreview"
    | "closeProjectFilePreview"
    | "openProjectFileExternally"
    | "revealProjectFile"
  > &
    Partial<Pick<RuntimeClient, "onProjectFilePreviewInteraction">>;
  projectId?: string;
  folderPath?: string;
  selection?: ProjectFileSelection;
  mode: ProjectFileWorkspaceMode;
  onModeChange: (mode: ProjectFileWorkspaceMode) => void;
  onPreviewLabelChange?: (label: string) => void;
  onPreviewPrimaryClick?: () => void;
};

type PreviewState =
  | { status: "idle" }
  | { status: "loading"; requestId: string; displayName: string }
  | { status: "ready"; preview: ProjectFilePreviewHandle }
  | { status: "error"; requestId: string; message: string };

function previewRequestId(): string {
  return globalThis.crypto?.randomUUID?.() ??
    `preview-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export default function FilePreviewHost({
  runtimeClient,
  projectId,
  folderPath,
  selection,
  mode,
  onModeChange,
  onPreviewLabelChange,
  onPreviewPrimaryClick,
}: FilePreviewHostProps) {
  const [state, setState] = useState<PreviewState>({ status: "idle" });
  const [retryGeneration, setRetryGeneration] = useState(0);
  const [guestLoading, setGuestLoading] = useState(false);
  const activePreviewIdRef = useRef<string | undefined>(undefined);
  const generationRef = useRef(0);

  const closeActivePreview = useCallback(() => {
    const previewId = activePreviewIdRef.current;
    activePreviewIdRef.current = undefined;
    if (previewId) void runtimeClient.closeProjectFilePreview({ previewId });
  }, [runtimeClient]);

  useEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    closeActivePreview();
    setGuestLoading(false);
    if (!projectId || !folderPath || !selection) {
      setState({ status: "idle" });
      return;
    }

    const requestId = previewRequestId();
    setState({ status: "loading", requestId, displayName: selection.name });
    onPreviewLabelChange?.(selection.name);
    void runtimeClient
      .startProjectFilePreview({
        requestId,
        projectId,
        folderPath,
        relativePath: selection.relativePath,
      })
      .then((result) => {
        if (generationRef.current !== generation) {
          if (result.ok) {
            void runtimeClient.closeProjectFilePreview({
              previewId: result.preview.previewId,
            });
          }
          return;
        }
        if (!result.ok) {
          setState({ status: "error", requestId, message: result.message });
          return;
        }
        activePreviewIdRef.current = result.preview.previewId;
        onModeChange(result.preview.kind === "html" ? "browser" : "files");
        setState({ status: "ready", preview: result.preview });
      })
      .catch(() => {
        if (generationRef.current === generation) {
          setState({
            status: "error",
            requestId,
            message: "文件预览暂时不可用，请重试。",
          });
        }
      });

    return () => {
      void runtimeClient.closeProjectFilePreview({ requestId });
    };
  }, [
    closeActivePreview,
    folderPath,
    onModeChange,
    onPreviewLabelChange,
    projectId,
    retryGeneration,
    runtimeClient,
    selection,
  ]);

  useEffect(
    () => () => {
      generationRef.current += 1;
      closeActivePreview();
    },
    [closeActivePreview],
  );

  useEffect(() => {
    if (
      state.status !== "ready" ||
      state.preview.kind !== "office" ||
      !onPreviewPrimaryClick
    ) {
      return;
    }
    return runtimeClient.onProjectFilePreviewInteraction?.((interaction) => {
      if (
        interaction.previewId === state.preview.previewId &&
        interaction.type === "primary-click"
      ) {
        onPreviewPrimaryClick();
      }
    });
  }, [onPreviewPrimaryClick, runtimeClient, state]);

  const actionRequest: ProjectFilePreviewActionRequest | undefined =
    projectId && folderPath && selection
      ? {
          projectId,
          folderPath,
          relativePath: selection.relativePath,
        }
      : undefined;

  function runFallbackAction(
    action: (request: ProjectFilePreviewActionRequest) => Promise<{ ok: boolean; message?: string }>,
  ): void {
    if (!actionRequest) return;
    void action(actionRequest)
      .then((result) => {
        if (result.ok) return;
        setState({
          status: "error",
          requestId: previewRequestId(),
          message: result.message ?? "操作暂时无法完成。",
        });
      })
      .catch(() => {
        setState({
          status: "error",
          requestId: previewRequestId(),
          message: "操作暂时无法完成。",
        });
      });
  }

  if (mode === "browser" && state.status !== "ready") {
    return (
      <div className="project-files-preview-empty is-centered">
        <Globe2 size={42} strokeWidth={1.5} aria-hidden="true" />
        <strong>浏览器</strong>
        <span>预览 Html 文件</span>
      </div>
    );
  }

  if (state.status === "idle") return null;

  if (state.status === "loading") {
    return (
      <div className="project-file-preview-state" role="status">
        <span className="project-file-preview-spinner" aria-hidden="true" />
        <strong>{state.displayName}</strong>
        <span>正在准备只读预览…</span>
      </div>
    );
  }

  if (state.status === "error") {
    return (
      <div className="project-file-preview-state is-error" role="alert">
        <strong>无法预览此文件</strong>
        <span>{state.message}</span>
        <div className="project-file-preview-state__actions">
          <button type="button" onClick={() => setRetryGeneration((value) => value + 1)}>
            <RefreshCw size={15} aria-hidden="true" />
            重试
          </button>
          <button
            type="button"
            onClick={() => runFallbackAction(runtimeClient.openProjectFileExternally)}
          >
            <ExternalLink size={15} aria-hidden="true" />
            使用系统默认应用打开
          </button>
          <button
            type="button"
            onClick={() => runFallbackAction(runtimeClient.revealProjectFile)}
          >
            <FolderSearch2 size={15} aria-hidden="true" />
            在文件管理器中显示
          </button>
        </div>
      </div>
    );
  }

  const preview = state.preview;
  const previewVisible = preview.kind === "html" ? mode === "browser" : mode === "files";
  return (
    <div className="project-file-preview-mode-stack">
      <div
        className={`project-file-preview-mode-stack__preview${
          previewVisible ? "" : " is-hidden"
        }`}
        aria-hidden={previewVisible ? undefined : true}
      >
        <div className="project-file-preview-host">
          <header className="project-file-preview-heading">
            <div>
              <strong>{preview.displayName}</strong>
              <span>{preview.kind.toLocaleUpperCase()}</span>
            </div>
            <div className="project-file-preview-heading__actions">
              <button
                type="button"
                aria-label="重新载入"
                title="重新载入"
                onClick={() => setRetryGeneration((value) => value + 1)}
              >
                <RefreshCw size={15} aria-hidden="true" />
              </button>
              <button
                type="button"
                aria-label="使用系统默认应用打开"
                title="使用系统默认应用打开"
                onClick={() => runFallbackAction(runtimeClient.openProjectFileExternally)}
              >
                <ExternalLink size={15} aria-hidden="true" />
              </button>
              <button
                type="button"
                aria-label="在文件管理器中显示"
                title="在文件管理器中显示"
                onClick={() => runFallbackAction(runtimeClient.revealProjectFile)}
              >
                <FolderSearch2 size={15} aria-hidden="true" />
              </button>
            </div>
          </header>
          <div className="project-file-preview-content">
            <PreviewErrorBoundary
              resetKey={preview.previewId}
              onError={() =>
                setState({
                  status: "error",
                  requestId: preview.requestId,
                  message: "预览组件意外停止运行，请重试。",
                })
              }
            >
              <Suspense
                fallback={
                  <div className="project-file-preview-state" role="status">
                    <span className="project-file-preview-spinner" aria-hidden="true" />
                    <span>正在加载查看器…</span>
                  </div>
                }
              >
                {preview.transport === "text" && preview.kind === "markdown" ? (
                  <MarkdownPreview preview={preview} />
                ) : preview.transport === "text" && preview.kind === "code" ? (
                  <CodePreview preview={preview} />
                ) : preview.transport === "url" && preview.kind === "image" ? (
                  <ImagePreview
                    preview={preview}
                    onLoadError={() =>
                      setState({
                        status: "error",
                        requestId: preview.requestId,
                        message: "图片加载失败。",
                      })
                    }
                  />
                ) : preview.transport === "url" && preview.kind === "pdf" ? (
                  <PdfPreview
                    preview={preview as Parameters<typeof PdfPreview>[0]["preview"]}
                    onLoadingChange={setGuestLoading}
                    onLoadError={(message) =>
                      setState({
                        status: "error",
                        requestId: preview.requestId,
                        message,
                      })
                    }
                  />
                ) : preview.transport === "url" && preview.partition ? (
                  <WebviewPreview
                    preview={preview as Parameters<typeof WebviewPreview>[0]["preview"]}
                    onLoadingChange={setGuestLoading}
                    onLoadError={(message) =>
                      setState({
                        status: "error",
                        requestId: preview.requestId,
                        message,
                      })
                    }
                  />
                ) : null}
              </Suspense>
            </PreviewErrorBoundary>
            {guestLoading ? (
              <div className="project-file-preview-loading-overlay" role="status">
                <span className="project-file-preview-spinner" aria-hidden="true" />
                正在加载…
              </div>
            ) : null}
          </div>
        </div>
      </div>
      {!previewVisible ? (
        <div className="project-files-preview-empty is-centered">
          <Globe2 size={42} strokeWidth={1.5} aria-hidden="true" />
          {preview.kind === "html" ? (
            <>
              <strong>{preview.displayName}</strong>
              <span>HTML 文件统一在浏览器中预览</span>
              <button type="button" onClick={() => onModeChange("browser")}>
                打开浏览器
              </button>
            </>
          ) : (
            <>
              <strong>浏览器</strong>
              <span>预览 Html 文件</span>
            </>
          )}
        </div>
      ) : null}
    </div>
  );
}
