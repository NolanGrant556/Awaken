import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { AwakenIcon } from "@awaken/design-system";
import type { ArtifactOpenRequest, ProjectFilePreviewHandle } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import ImagePreview from "./ImagePreview";
import PreviewErrorBoundary from "./PreviewErrorBoundary";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import { useProjectFilePreviewPrimaryClick } from "./useProjectFilePreviewPrimaryClick";
import WebviewPreview from "./WebviewPreview";

const MarkdownPreview = lazy(() => import("./MarkdownPreview"));
const CodePreview = lazy(() => import("./CodePreview"));
const PdfPreview = lazy(() => import("./PdfPreview"));

export type ArtifactPreviewSelection = ArtifactOpenRequest & {
  title: string;
};

export type ArtifactPreviewHostProps = {
  runtimeClient: Pick<
    RuntimeClient,
    | "startArtifactPreview"
    | "closeProjectFilePreview"
    | "openArtifact"
    | "revealArtifact"
  > &
    Partial<Pick<RuntimeClient, "onProjectFilePreviewInteraction">>;
  selection: ArtifactPreviewSelection;
  onPreviewPrimaryClick?: () => void;
};

type ArtifactPreviewState =
  | { status: "loading"; requestId: string }
  | { status: "ready"; preview: ProjectFilePreviewHandle }
  | { status: "error"; requestId: string; message: string };

function previewRequestId(): string {
  return (
    globalThis.crypto?.randomUUID?.() ??
    `artifact-preview-${Date.now()}-${Math.random().toString(16).slice(2)}`
  );
}

export default function ArtifactPreviewHost({
  runtimeClient,
  selection,
  onPreviewPrimaryClick,
}: ArtifactPreviewHostProps) {
  const [retryGeneration, setRetryGeneration] = useState(0);
  const [guestLoading, setGuestLoading] = useState(false);
  const [state, setState] = useState<ArtifactPreviewState>(() => ({
    status: "loading",
    requestId: previewRequestId(),
  }));
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
    const requestId = previewRequestId();
    setState({ status: "loading", requestId });
    void runtimeClient
      .startArtifactPreview({ ...selection, requestId })
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
        setState({ status: "ready", preview: result.preview });
      })
      .catch(() => {
        if (generationRef.current === generation) {
          setState({
            status: "error",
            requestId,
            message: "交付物预览暂时不可用，请重试。",
          });
        }
      });
    return () => {
      void runtimeClient.closeProjectFilePreview({ requestId });
    };
  }, [closeActivePreview, retryGeneration, runtimeClient, selection]);

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

  function runArtifactAction(
    action: (request: ArtifactOpenRequest) => Promise<{ ok: boolean; message?: string }>,
  ): void {
    void action(selection)
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

  if (state.status === "loading") {
    return (
      <div className="project-file-preview-state" role="status">
        <span className="project-file-preview-spinner" aria-hidden="true" />
        <strong>{selection.title}</strong>
        <span>正在准备交付物预览…</span>
      </div>
    );
  }

  if (state.status === "error") {
    return (
      <div className="project-file-preview-state is-error" role="alert">
        <strong>无法预览此交付物</strong>
        <span>{state.message}</span>
        <div className="project-file-preview-state__actions">
          <button type="button" onClick={() => setRetryGeneration((value) => value + 1)}>
            <AwakenIcon concept="refresh" size={16} />
            重试
          </button>
          <button type="button" onClick={() => runArtifactAction(runtimeClient.openArtifact)}>
            <AwakenIcon concept="external-open" size={16} />
            使用系统默认应用打开
          </button>
          <button type="button" onClick={() => runArtifactAction(runtimeClient.revealArtifact)}>
            <AwakenIcon concept="locate-folder" size={16} />
            在文件管理器中显示
          </button>
        </div>
      </div>
    );
  }

  const preview = state.preview;
  const frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  > = {
    displayName: selection.title || preview.displayName,
    loading: guestLoading,
    onReload: () => setRetryGeneration((value) => value + 1),
    onOpenExternal: () => runArtifactAction(runtimeClient.openArtifact),
    onReveal: () => runArtifactAction(runtimeClient.revealArtifact),
  };

  return (
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
          <PreviewFrame {...frameProps} formatLabel={preview.kind.toLocaleUpperCase()}>
            <div className="project-file-preview-state" role="status">
              <span className="project-file-preview-spinner" aria-hidden="true" />
              <span>正在加载查看器…</span>
            </div>
          </PreviewFrame>
        }
      >
        {preview.transport === "text" && preview.kind === "markdown" ? (
          <MarkdownPreview preview={preview} frameProps={frameProps} />
        ) : preview.transport === "text" && preview.kind === "code" ? (
          <CodePreview preview={preview} frameProps={frameProps} />
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
              setState({ status: "error", requestId: preview.requestId, message })
            }
          />
        ) : preview.transport === "url" && preview.partition ? (
          <WebviewPreview
            preview={preview as Parameters<typeof WebviewPreview>[0]["preview"]}
            frameProps={frameProps}
            onLoadingChange={setGuestLoading}
            onLoadError={(message) =>
              setState({ status: "error", requestId: preview.requestId, message })
            }
          />
        ) : (
          <PreviewFrame {...frameProps} formatLabel={preview.kind.toLocaleUpperCase()}>
            <div className="project-file-preview-state" role="status">
              <span>此交付物没有可用的预览器。</span>
            </div>
          </PreviewFrame>
        )}
      </Suspense>
    </PreviewErrorBoundary>
  );
}
