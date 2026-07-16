import { useEffect, useRef, useState } from "react";
import {
  AwakenDialog as Dialog,
  AwakenMarkdownRenderer,
} from "@awaken/design-system";
import type { SkillPreviewResponse } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";

export type SkillPreviewTarget = {
  catalogEntryId: string;
  projectRoot?: string;
};

type SkillPreviewDialogProps = {
  target: SkillPreviewTarget | null;
  runtimeClient: Pick<RuntimeClient, "getSkillPreview">;
  onOpenChange: (open: boolean) => void;
};

export function SkillPreviewDialog({
  target,
  runtimeClient,
  onOpenChange,
}: SkillPreviewDialogProps) {
  const [preview, setPreview] = useState<SkillPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const requestVersionRef = useRef(0);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const requestVersion = requestVersionRef.current + 1;
    requestVersionRef.current = requestVersion;
    scrollRef.current?.scrollTo({ top: 0 });

    if (!target) {
      setPreview(null);
      setErrorMessage(null);
      setLoading(false);
      return;
    }

    setPreview(null);
    setErrorMessage(null);
    setLoading(true);

    void runtimeClient
      .getSkillPreview({
        catalogEntryId: target.catalogEntryId,
        projectRoot: target.projectRoot,
      })
      .then((nextPreview) => {
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        setPreview(nextPreview);
        scrollRef.current?.scrollTo({ top: 0 });
      })
      .catch((error: unknown) => {
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        setErrorMessage(
          error instanceof Error ? error.message : "无法读取这个 Skill 的 SKILL.md",
        );
      })
      .finally(() => {
        if (requestVersionRef.current === requestVersion) {
          setLoading(false);
        }
      });
  }, [runtimeClient, target]);

  return (
    <Dialog
      open={target !== null}
      title="SKILL.md 预览"
      titleVisuallyHidden
      size="large"
      className="skill-preview-dialog"
      bodyClassName="skill-preview-dialog-scroll"
      bodyRef={scrollRef}
      bodyTabIndex={0}
      initialFocusRef={scrollRef}
      onOpenChange={onOpenChange}
    >
      <div className="skill-preview-content">
        {loading ? (
          <div className="skill-preview-status" role="status" aria-live="polite">
            正在读取 SKILL.md…
          </div>
        ) : null}
        {errorMessage ? (
          <div className="skill-preview-error" role="alert">
            <strong>无法预览 SKILL.md</strong>
            <span>{errorMessage}</span>
          </div>
        ) : null}
        {preview ? (
          <>
            {preview.frontmatterRaw ? (
              <pre className="skill-preview-frontmatter" aria-label="YAML frontmatter">
                <code className="language-yaml">{preview.frontmatterRaw}</code>
              </pre>
            ) : null}
            <AwakenMarkdownRenderer
              className="assistant-message assistant-message-markdown skill-preview-markdown"
              markdown={preview.bodyMarkdown}
              fallback=""
              renderMermaid={false}
              renderImages={false}
              allowedLinkProtocols={["http", "https"]}
            />
          </>
        ) : null}
      </div>
    </Dialog>
  );
}
