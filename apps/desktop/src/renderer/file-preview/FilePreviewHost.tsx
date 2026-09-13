import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { AwakenIcon } from "@awaken/design-system";
import type {
  FileLineComment,
  ProjectFilePreviewActionRequest,
  ProjectFilePreviewHandle,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import {
  createIdleEmbeddedBrowserClient,
  EMBEDDED_BROWSER_EMPTY_HINT,
  resolveEmbeddedBrowserTabRef,
  type EmbeddedBrowserBridge,
} from "../../shared/embedded-browser";
import ImagePreview from "./ImagePreview";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import PreviewErrorBoundary from "./PreviewErrorBoundary";
import { useProjectFilePreviewPrimaryClick } from "./useProjectFilePreviewPrimaryClick";
import WebviewPreview from "./WebviewPreview";
import EmbeddedBrowserChrome from "./EmbeddedBrowserChrome";

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
    | "saveProjectMarkdownFile"
  > &
    Partial<Pick<RuntimeClient, "onProjectFilePreviewInteraction">>;
  projectId?: string;
  folderPath?: string;
  selection?: ProjectFileSelection;
  lineRequest?: { id: string; startLine: number; endLine?: number };
  mode: ProjectFileWorkspaceMode;
  onModeChange: (mode: ProjectFileWorkspaceMode) => void;
  onPreviewLabelChange?: (label: string) => void;
  onPreviewPrimaryClick?: () => void;
  fileComments?: FileLineComment[];
  onCreateFileComment?: (comment: FileLineComment) => void;
  onAddFileTextSelection?: (selection: FileLineComment) => void;
  onDeleteFileComment?: (commentId: string) => void;
  fileCommentDisabled?: boolean;
  threadId?: string;
  viewId?: string;
  embeddedBrowser?: Pick<
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
    | "onGuestHttpNavigate"
  >;
  browserActive?: boolean;
  onLeaveHtmlPreview?: () => void;
};

type PreviewState =
  | { status: "idle" }
  | { status: "loading"; requestId: string; displayName: string }
  | {
      status: "ready";
      preview: ProjectFilePreviewHandle;
      request: ProjectFilePreviewActionRequest;
      saveState: { digest?: string; queue: Promise<void> };
    }
  | { status: "error"; requestId: string; message: string };

function previewRequestId(): string {
  return (
    globalThis.crypto?.randomUUID?.() ??
    `preview-${Date.now()}-${Math.random().toString(16).slice(2)}`
  );
}

export default function FilePreviewHost({
  runtimeClient,
  projectId,
  folderPath,
  selection,
  lineRequest,
  mode,
  onModeChange,
  onPreviewLabelChange,
  onPreviewPrimaryClick,
  fileComments = [],
  onCreateFileComment,
  onAddFileTextSelection,
  onDeleteFileComment,
  fileCommentDisabled = false,
  threadId,
  viewId,
  embeddedBrowser,
  browserActive = true,
  onLeaveHtmlPreview,
}: FilePreviewHostProps) {
  const idleBrowserClient = useMemo(() => createIdleEmbeddedBrowserClient(), []);
  const browserClient = embeddedBrowser ?? idleBrowserClient;
  const [state, setState] = useState<PreviewState>({ status: "idle" });
  const [retryGeneration, setRetryGeneration] = useState(0);
  const [guestLoading, setGuestLoading] = useState(false);
  const activePreviewIdRef = useRef<string | undefined>(undefined);
  const generationRef = useRef(0);
  const onModeChangeRef = useRef(onModeChange);
  const onPreviewLabelChangeRef = useRef(onPreviewLabelChange);

  onModeChangeRef.current = onModeChange;
  onPreviewLabelChangeRef.current = onPreviewLabelChange;

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
    onPreviewLabelChangeRef.current?.(selection.name);
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
        onModeChangeRef.current("files");
        setState({
          status: "ready", preview: result.preview,
          request: { projectId, folderPath, relativePath: selection.relativePath },
          saveState: {
            digest: result.preview.transport === "text" ? result.preview.contentDigest : undefined,
            queue: Promise.resolve(),
          },
        });
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
    projectId,
    retryGeneration,
    runtimeClient,
    selection?.name,
    selection?.relativePath,
  ]);

  useEffect(
    () => () => {
      generationRef.current += 1;
      closeActivePreview();
    },
    [closeActivePreview],
  );

  const officePreviewId =
    state.status === "ready" && state.preview.kind === "office"
      ? state.preview.previewId
      : undefined;
  useProjectFilePreviewPrimaryClick({
    runtimeClient,
    previewId: officePreviewId,
    onPreviewPrimaryClick,
  });

  const browserRef = useMemo(
    () => resolveEmbeddedBrowserTabRef({ threadId, viewId }),
    [threadId, viewId],
  );

  const actionRequest: ProjectFilePreviewActionRequest | undefined =
    state.status === "ready" ? state.request : projectId && folderPath && selection
      ? {
          projectId,
          folderPath,
          relativePath: selection.relativePath,
        }
      : undefined;

  function runFallbackAction(
    action: (
      request: ProjectFilePreviewActionRequest,
    ) => Promise<{ ok: boolean; message?: string }>,
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

  const saveTextContent = (content: string) => {
    if (state.status !== "ready") {
      return Promise.resolve({
        ok: false as const,
        code: "INVALID_REQUEST" as const,
        message: "无法保存此文件。",
      });
    }
    // The request and digest belong to the loaded preview, including its final
    // flush after selection has already moved to another file.
    const { request, saveState } = state;
    return new Promise<Awaited<ReturnType<RuntimeClient["saveProjectMarkdownFile"]>>>((resolve, reject) => {
      saveState.queue = saveState.queue
        .catch(() => undefined)
        .then(async () => {
          try {
            const result = await runtimeClient.saveProjectMarkdownFile({
              ...request,
              content,
              expectedDigest: saveState.digest,
            });
            if (result.ok && result.digest) saveState.digest = result.digest;
            resolve(result);
          } catch (error) {
            reject(error);
          }
        });
    });
  };

  const createFileContext = (
    selectionRange: import("./CodePreview").CodePreviewLineSelection,
    kind: "comment" | "selection",
    commentText = "",
  ) => {
    const contentDigest = state.status === "ready" ? state.saveState.digest : undefined;
    if (!projectId || !folderPath || !selection || !contentDigest) return;
    const item: FileLineComment = {
      id: globalThis.crypto?.randomUUID?.() ??
        `file-context-${Date.now()}-${Math.random().toString(16).slice(2)}`,
      kind,
      projectId,
      folderPath,
      relativePath: selection.relativePath,
      displayName: selection.name,
      contentDigest,
      startLine: selectionRange.startLine,
      endLine: selectionRange.endLine,
      selectedText: selectionRange.selectedText,
      comment: commentText.trim(),
      createdAt: new Date().toISOString(),
      validationState: "ready",
    };
    if (kind === "selection") onAddFileTextSelection?.(item);
    else onCreateFileComment?.(item);
  };

  const wrapBrowser = (children?: React.ReactNode) => (
    <EmbeddedBrowserChrome
      key={JSON.stringify([browserRef.threadId, browserRef.viewId])}
      client={browserClient}
      viewRef={browserRef}
      enabled={mode === "browser"}
      active={browserActive}
      onLeaveHtmlPreview={onLeaveHtmlPreview}
    >
      {children}
    </EmbeddedBrowserChrome>
  );

  if (mode === "browser" && state.status !== "ready") {
    if (state.status === "loading") {
      return wrapBrowser(
        <div className="project-file-preview-state" role="status">
          <span className="project-file-preview-spinner" aria-hidden="true" />
          <strong>{state.displayName}</strong>
          <span>正在准备预览…</span>
        </div>,
      );
    }
    if (state.status === "error") {
      return wrapBrowser(
        <div className="project-file-preview-state is-error" role="alert">
          <strong>无法预览此文件</strong>
          <span>{state.message}</span>
        </div>,
      );
    }
    return wrapBrowser();
  }

  if (state.status === "idle") return wrapBrowser();

  if (state.status === "loading") {
    return wrapBrowser(
      <div className="project-file-preview-state" role="status">
        <span className="project-file-preview-spinner" aria-hidden="true" />
        <strong>{state.displayName}</strong>
        <span>正在准备预览…</span>
      </div>,
    );
  }

  if (state.status === "error") {
    return wrapBrowser(
      <div className="project-file-preview-state is-error" role="alert">
        <strong>无法预览此文件</strong>
        <span>{state.message}</span>
        <div className="project-file-preview-state__actions">
          <button
            type="button"
            onClick={() => setRetryGeneration((value) => value + 1)}
          >
            <AwakenIcon concept="refresh" size={16} />
            重试
          </button>
          <button
            type="button"
            onClick={() =>
              runFallbackAction(runtimeClient.openProjectFileExternally)
            }
          >
            <AwakenIcon concept="external-open" size={16} />
            使用系统默认应用打开
          </button>
          <button
            type="button"
            onClick={() => runFallbackAction(runtimeClient.revealProjectFile)}
          >
            <AwakenIcon concept="locate-folder" size={16} />
            在文件管理器中显示
          </button>
        </div>
      </div>,
    );
  }

  const preview = state.preview;
  const previewVisible = mode === "files";
  const frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  > = {
    displayName: preview.displayName,
    loading: guestLoading,
    onReload: () => setRetryGeneration((value) => value + 1),
    onOpenExternal: () =>
      runFallbackAction(runtimeClient.openProjectFileExternally),
    onReveal: () => runFallbackAction(runtimeClient.revealProjectFile),
  };
  const stack = (
    <div className="project-file-preview-mode-stack">
      <div
        className={`project-file-preview-mode-stack__preview${
          previewVisible ? "" : " is-hidden"
        }`}
        aria-hidden={previewVisible ? undefined : true}
      >
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
              <PreviewFrame
                {...frameProps}
                formatLabel={preview.kind.toLocaleUpperCase()}
              >
                <div className="project-file-preview-state" role="status">
                  <span
                    className="project-file-preview-spinner"
                    aria-hidden="true"
                  />
                  <span>正在加载查看器…</span>
                </div>
              </PreviewFrame>
            }
          >
            {preview.transport === "text" && preview.kind === "markdown" ? (
              <MarkdownPreview
                lineRequest={lineRequest}
                preview={preview}
                frameProps={frameProps}
                onSave={saveTextContent}
                fileComments={fileComments.filter(
                  (item) => item.projectId === projectId && item.folderPath === folderPath &&
                    item.relativePath === selection?.relativePath,
                )}
                onCreateComment={(range, text) => createFileContext(range, "comment", text)}
                onAddTextSelection={(range) => createFileContext(range, "selection")}
                onDeleteComment={onDeleteFileComment}
                commentDisabled={fileCommentDisabled}
              />
            ) : preview.transport === "text" && preview.kind === "code" ? (
              <CodePreview
                lineRequest={lineRequest}
                preview={preview}
                frameProps={frameProps}
                editable
                onSave={saveTextContent}
                fileComments={fileComments.filter(
                  (comment) =>
                    comment.kind !== "selection" &&
                    comment.projectId === projectId &&
                    comment.folderPath === folderPath &&
                    comment.relativePath === selection?.relativePath,
                )}
                onCreateComment={(selectionRange, commentText) =>
                  createFileContext(selectionRange, "comment", commentText)}
                onDeleteComment={onDeleteFileComment}
                commentDisabled={fileCommentDisabled}
              />
            ) : preview.transport === "url" && preview.kind === "image" ? (
              <ImagePreview
                preview={preview}
                frameProps={frameProps}
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
                frameProps={frameProps}
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
                preview={
                  preview as Parameters<typeof WebviewPreview>[0]["preview"]
                }
                frameProps={frameProps}
                onLoadingChange={setGuestLoading}
                onLoadError={(message) =>
                  setState({
                    status: "error",
                    requestId: preview.requestId,
                    message,
                  })
                }
              />
            ) : (
              <PreviewFrame
                {...frameProps}
                formatLabel={preview.kind.toLocaleUpperCase()}
              >
                <div className="project-file-preview-state" role="status">
                  <span>此文件没有可用的预览器。</span>
                </div>
              </PreviewFrame>
            )}
          </Suspense>
        </PreviewErrorBoundary>
      </div>
      {!previewVisible ? (
        <div className="project-files-preview-empty is-centered">
          <AwakenIcon concept="browser" size={24} />
          <strong>浏览器</strong>
          <span>{EMBEDDED_BROWSER_EMPTY_HINT}</span>
        </div>
      ) : null}
    </div>
  );
  return wrapBrowser(stack);
}
