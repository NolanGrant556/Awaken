import type { AguiEvent } from "./agui";

function finite(value?: number): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

/** Decode current fields, older aliases, then legacy message text in that order. */
export function compactEventDetails(event: AguiEvent, previousEstimate?: number) {
  const status = event.type === "AWAKEN_COMPACT_FAILED" ? "failed" as const
    : event.type === "AWAKEN_COMPACT_FINISHED" ? "finished" as const : "started" as const;
  const mode = event.contextCompactMode?.trim() || event.compactMode?.trim()
    || event.message?.match(/\bcompact (?:started|finished|failed):\s*([a-z_]+)(?=\s|$)/i)?.[1]?.toLowerCase();
  const boundaryId = event.contextCompactBoundaryId?.trim() || event.compactBoundaryId?.trim()
    || event.message?.match(/\bboundary=([^\s]+)/)?.[1];
  const tokenText = event.message?.match(/\bestimated_tokens=(\d+)\b/i)?.[1];
  const preCompactEstimatedTokens = finite(event.contextCompactPreEstimateTokens)
    ?? finite(event.compactPreEstimate)
    ?? (status === "started" ? finite(event.contextEstimatedTokens) : finite(previousEstimate));
  const postCompactEstimatedTokens = finite(event.contextCompactPostEstimateTokens)
    ?? finite(event.compactPostEstimate)
    ?? (status === "finished" ? finite(event.contextEstimatedTokens)
      ?? (tokenText ? finite(Number(tokenText)) : undefined) : undefined);
  const usedFallback = (!event.contextCompactMode?.trim() && Boolean(mode))
    || (!event.contextCompactBoundaryId?.trim() && Boolean(boundaryId))
    || (finite(event.contextCompactPreEstimateTokens) === undefined && preCompactEstimatedTokens !== undefined)
    || (finite(event.contextCompactPostEstimateTokens) === undefined && postCompactEstimatedTokens !== undefined);
  return {
    status, mode, boundaryId, preCompactEstimatedTokens, postCompactEstimatedTokens,
    fallbackState: usedFallback ? "legacy" as const : "structured" as const
  };
}
