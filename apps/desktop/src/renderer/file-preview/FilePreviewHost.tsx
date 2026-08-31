import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
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
import ImagePreview from "./ImagePreview";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import PreviewErrorBoundary from "./PreviewErrorBoundary";
import { useProjectFilePreviewPrimaryClick } from "./useProjectFilePreviewPrimaryClick";
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
    | "saveProjectMarkdownFile"
  > &
    Partial<Pick<RuntimeClient, "onProjectFilePreviewInteraction">>;
  projectId?: string;
  folderPath?: string;
  selection?: ProjectFileSelection;
  mode: ProjectFileWorkspaceMode;
  onModeChange: (mode: ProjectFileWorkspaceMode) => void;
  onPreviewLabelChange?: (label: string) => void;
  onPreviewPrimaryClick?: () => void;
  fileComments?: FileLineComment[];
  onCreateFileComment?: (comment: FileLineComment) => void;
  onDeleteFileComment?: (commentId: string) => void;
  fileCommentDisabled?: boolean;
};

type PreviewState =
  | { status: "idle" }
  | { status: "loading"; requestId: string; displayName: string }
  | { status: "ready"; preview: ProjectFilePreviewHandle }
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
  mode,
  onModeChange,
  onPreviewLabelChange,
  onPreviewPrimaryClick,
  fileComments = [],
  onCreateFileComment,
  onDeleteFileComment,
  fileCommentDisabled = false,
}: FilePreviewHostProps) {
  const [state, setState] = useState<PreviewState>({ status: "idle" });
  const [retryGeneration, setRetryGeneration] = useState(0);
  const [guestLoading, setGuestLoading] = useState(false);
  const activePreviewIdRef = useRef<string | undefined>(undefined);
	const markdownDigestRef = useRef<string | undefined>(undefined);
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
		markdownDigestRef.current = result.preview.transport === "text"
			? result.preview.contentDigest
			: undefined;
        onModeChangeRef.current(result.preview.kind === "html" ? "browser" : "files");
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

  const officePreviewId =
    state.status === "ready" && state.preview.kind === "office"
      ? state.preview.previewId
      : undefined;
  useProjectFilePreviewPrimaryClick({
    runtimeClient,
    previewId: officePreviewId,
    onPreviewPrimaryClick,
  });

  const actionRequest: ProjectFilePreviewActionRequest | undefined =
    projectId && folderPath && selection
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

  if (mode === "browser" && state.status !== "ready") {
    return (
      <div className="project-files-preview-empty is-centered">
        <AwakenIcon concept="browser" size={24} />
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
        <span>正在准备预览…</span>
      </div>
    );
  }

  if (state.status === "error") {
    return (
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
      </div>
    );
  }

  const preview = state.preview;
  const previewVisible =
    preview.kind === "html" ? mode === "browser" : mode === "files";
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
  return (
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
                preview={preview}
                frameProps={frameProps}
                onSave={(content) =>
                  actionRequest
					? runtimeClient.saveProjectMarkdownFile({
							...actionRequest,
							content,
						expectedDigest: actionRequest.relativePath === ".awaken/AGENTS.md"
							? markdownDigestRef.current
							: undefined,
						  }).then((result) => {
							if (result.ok && result.digest) markdownDigestRef.current = result.digest;
							if (!result.ok && result.code === "FILE_CHANGED" && result.currentDigest) {
								markdownDigestRef.current = result.currentDigest;
							}
							return result;
						  })
                    : Promise.resolve({
                        ok: false,
                        code: "INVALID_REQUEST",
                        message: "无法保存此文件。",
                      })
                }
              />
            ) : preview.transport === "text" && preview.kind === "code" ? (
              <CodePreview
                preview={preview}
                frameProps={frameProps}
                fileComments={fileComments.filter(
                  (comment) =>
                    comment.projectId === projectId &&
                    comment.folderPath === folderPath &&
                    comment.relativePath === selection?.relativePath,
                )}
                onCreateComment={(selectionRange, commentText) => {
                  if (
                    !projectId ||
                    !folderPath ||
                    !selection ||
                    !preview.contentDigest ||
                    !onCreateFileComment
                  ) return;
                  onCreateFileComment({
                    id: globalThis.crypto?.randomUUID?.() ??
                      `file-comment-${Date.now()}-${Math.random().toString(16).slice(2)}`,
                    projectId,
                    folderPath,
                    relativePath: selection.relativePath,
                    displayName: preview.displayName,
                    contentDigest: preview.contentDigest,
                    startLine: selectionRange.startLine,
                    endLine: selectionRange.endLine,
                    selectedText: selectionRange.selectedText,
                    comment: commentText.trim(),
                    createdAt: new Date().toISOString(),
                    validationState: "ready",
                  });
                }}
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
