import { useState } from "react";
import type { RuntimeClient } from "./runtime/client";
import type { ExternalInteractionItem, ComputerReviewItem, ComputerHandoffItem } from "./runtime/reducer";

type Options = {
  runId?: string;
  activeExternalInteraction?: ExternalInteractionItem;
  activeComputerReview?: ComputerReviewItem;
  activeComputerHandoff?: ComputerHandoffItem;
  runtimeClient: Pick<RuntimeClient, "resolveExternalInteraction" | "resolveACPPermission" | "resolveComputerReview" | "computerSessionControl">;
  setNotice: (message: string | null) => void;
};
// Legacy root items may omit runId. A child must always keep its original owner.
function ownerRunId(item: { runId?: string; taskId?: string } | undefined, currentRunId?: string): string | undefined {
  return item?.runId ?? (item?.taskId ? undefined : currentRunId);
}
export function useRuntimeInteractions({ runId, activeExternalInteraction, activeComputerReview,
  activeComputerHandoff, runtimeClient, setNotice }: Options) {
  const [resolvingExternalInteractionId, setResolvingExternalInteractionId] =
    useState<string | null>(null);
  const [resolvingComputerReviewId, setResolvingComputerReviewId] = useState<
    string | null
  >(null);
  const [controllingComputerHandoffId, setControllingComputerHandoffId] =
    useState<string | null>(null);
  async function resolveActiveExternalInteraction(
    resolution: "completed" | "cancelled" | "expired" | "failed",
    content?: Record<string, unknown>,
  ): Promise<void> {
    const targetRunId = ownerRunId(activeExternalInteraction, runId);
    if (!targetRunId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveExternalInteraction(
        targetRunId,
        activeExternalInteraction.id,
        resolution,
        content,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to resolve external interaction",
      );
    } finally {
      setResolvingExternalInteractionId(null);
    }
  }

  async function resolveActiveACPPermission(
    optionId: string,
    cancelled = false,
  ): Promise<void> {
    const targetRunId = ownerRunId(activeExternalInteraction, runId);
    if (!targetRunId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveACPPermission(
        targetRunId,
        activeExternalInteraction.id,
        optionId,
        cancelled,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to resolve ACP permission",
      );
    } finally {
      setResolvingExternalInteractionId(null);
    }
  }

  async function resolveActiveComputerReview(
    decision: "approved_once" | "denied",
  ): Promise<void> {
    const targetRunId = ownerRunId(activeComputerReview, runId);
    if (!targetRunId || !activeComputerReview) {
      return;
    }
    if (resolvingComputerReviewId === activeComputerReview.id) {
      return;
    }
    setNotice(null);
    setResolvingComputerReviewId(activeComputerReview.id);
    try {
      await runtimeClient.resolveComputerReview(
        targetRunId,
        activeComputerReview.reviewId,
        decision,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to resolve computer review",
      );
    } finally {
      setResolvingComputerReviewId(null);
    }
  }

  async function returnComputerControl(): Promise<void> {
    const targetRunId = ownerRunId(activeComputerHandoff, runId);
    if (!targetRunId || !activeComputerHandoff) {
      return;
    }
    if (controllingComputerHandoffId === activeComputerHandoff.id) {
      return;
    }
    setNotice(null);
    setControllingComputerHandoffId(activeComputerHandoff.id);
    try {
      await runtimeClient.computerSessionControl(targetRunId, "return", activeComputerHandoff.sessionId);
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to return computer control",
      );
    } finally {
      setControllingComputerHandoffId(null);
    }
  }

  return { resolvingExternalInteractionId, resolvingComputerReviewId, controllingComputerHandoffId, resolveActiveExternalInteraction, resolveActiveACPPermission, resolveActiveComputerReview, returnComputerControl };
}
