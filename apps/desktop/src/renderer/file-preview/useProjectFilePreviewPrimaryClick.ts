import { useEffect } from "react";
import type { RuntimeClient } from "../runtime/client";

type PreviewInteractionRuntimeClient = Partial<
  Pick<RuntimeClient, "onProjectFilePreviewInteraction">
>;

export function useProjectFilePreviewPrimaryClick({
  runtimeClient,
  previewId,
  onPreviewPrimaryClick,
}: {
  runtimeClient: PreviewInteractionRuntimeClient;
  previewId?: string;
  onPreviewPrimaryClick?: () => void;
}): void {
  useEffect(() => {
    if (!previewId || !onPreviewPrimaryClick) return;
    return runtimeClient.onProjectFilePreviewInteraction?.((interaction) => {
      if (
        interaction.previewId === previewId &&
        interaction.type === "primary-click"
      ) {
        onPreviewPrimaryClick();
      }
    });
  }, [onPreviewPrimaryClick, previewId, runtimeClient]);
}
