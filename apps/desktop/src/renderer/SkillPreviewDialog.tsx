import { useEffect, useRef, useState, type RefObject } from "react";
import {
  AwakenSkillPreviewContent,
  AwakenSkillPreviewDeleteButton,
  AwakenSkillPreviewDialog as AwakenSkillPreviewDialogComponent,
} from "@awaken/design-system";
import type { SkillPreviewResponse } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";

export type SkillPreviewTarget = {
  catalogEntryId: string;
  projectRoot?: string;
  skillId?: string;
};

export type SkillPreviewPresentation = {
  name: string;
  sourceLabel: string;
  description: string;
  enabled: boolean;
  toggleDisabled?: boolean;
  deletable?: boolean;
  deleting?: boolean;
};

type SkillPreviewDialogProps = {
  target: SkillPreviewTarget | null;
  presentation: SkillPreviewPresentation | null;
  runtimeClient: Pick<RuntimeClient, "getSkillPreview">;
  onEnabledChange?: (enabled: boolean) => void;
  onDelete?: () => void;
  onOpenChange: (open: boolean) => void;
};

type SkillPreviewContentProps = {
  target: SkillPreviewTarget | null;
  presentation: SkillPreviewPresentation | null;
  runtimeClient: Pick<RuntimeClient, "getSkillPreview">;
  scrollRef: RefObject<HTMLDivElement | null>;
  onEnabledChange?: (enabled: boolean) => void;
};

function useSkillPreviewDocument(
  target: SkillPreviewTarget | null,
  runtimeClient: Pick<RuntimeClient, "getSkillPreview">,
) {
  const [preview, setPreview] = useState<SkillPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const requestVersionRef = useRef(0);

  useEffect(() => {
    const requestVersion = requestVersionRef.current + 1;
    requestVersionRef.current = requestVersion;

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
      })
      .catch((error: unknown) => {
        if (requestVersionRef.current !== requestVersion) {
          return;
        }
        setErrorMessage(
          error instanceof Error ? error.message : "无法读取这个 Skill 包",
        );
      })
      .finally(() => {
        if (requestVersionRef.current === requestVersion) {
          setLoading(false);
        }
      });
  }, [runtimeClient, target]);

  return { preview, loading, errorMessage };
}

export function SkillPreviewContent({
  target,
  presentation,
  runtimeClient,
  scrollRef,
  onEnabledChange,
}: SkillPreviewContentProps) {
  const { preview, loading, errorMessage } = useSkillPreviewDocument(target, runtimeClient);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: 0 });
    if (target) {
      scrollRef.current?.focus();
    }
  }, [scrollRef, target]);

  return (
    <AwakenSkillPreviewContent
      frontmatterRaw={preview?.frontmatterRaw}
      markdown={preview?.bodyMarkdown}
      resources={preview?.resources}
      resourcesTruncated={preview?.resourcesTruncated}
      loading={loading}
      errorMessage={errorMessage}
      skill={presentation ?? undefined}
      deleting={presentation?.deleting}
      documentRef={scrollRef}
      onEnabledChange={onEnabledChange}
    />
  );
}

export const SkillPreviewDeleteButton = AwakenSkillPreviewDeleteButton;

export function SkillPreviewDialog({
  target,
  presentation,
  runtimeClient,
  onEnabledChange,
  onDelete,
  onOpenChange,
}: SkillPreviewDialogProps) {
  const { preview, loading, errorMessage } = useSkillPreviewDocument(target, runtimeClient);

  return (
    <AwakenSkillPreviewDialogComponent
      open={target !== null}
      title={presentation ? `${presentation.name} 的 Skill 包` : "Skill 包预览"}
      contentKey={target ? `${target.catalogEntryId}:${target.projectRoot ?? ""}` : undefined}
      frontmatterRaw={preview?.frontmatterRaw}
      markdown={preview?.bodyMarkdown}
      resources={preview?.resources}
      resourcesTruncated={preview?.resourcesTruncated}
      loading={loading}
      errorMessage={errorMessage}
      skill={presentation ?? undefined}
      deletable={presentation?.deletable}
      deleting={presentation?.deleting}
      onEnabledChange={onEnabledChange}
      onDelete={onDelete}
      onOpenChange={onOpenChange}
    />
  );
}
