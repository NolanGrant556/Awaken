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
    if (!runId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveExternalInteraction(
        runId,
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
    if (!runId || !activeExternalInteraction) {
      return;
    }
    if (resolvingExternalInteractionId === activeExternalInteraction.id) {
      return;
    }
    setNotice(null);
    setResolvingExternalInteractionId(activeExternalInteraction.id);
    try {
      await runtimeClient.resolveACPPermission(
        runId,
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
    if (!runId || !activeComputerReview) {
      return;
    }
    if (resolvingComputerReviewId === activeComputerReview.id) {
      return;
    }
    setNotice(null);
    setResolvingComputerReviewId(activeComputerReview.id);
    try {
      await runtimeClient.resolveComputerReview(
        runId,
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
    if (!runId || !activeComputerHandoff) {
      return;
    }
    if (controllingComputerHandoffId === activeComputerHandoff.id) {
      return;
    }
    setNotice(null);
    setControllingComputerHandoffId(activeComputerHandoff.id);
    try {
      await runtimeClient.computerSessionControl(runId, "return");
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
