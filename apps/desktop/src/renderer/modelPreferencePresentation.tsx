import { AwakenProviderLogo } from "@awaken/design-system";
import { providerBrandForId } from "./model-provider-brand";
import type {
  ReasoningEffortLevel,
  RuntimeModelProfileSummary,
} from "../shared/runtime";

export function modelProfileLabel(
  profile: Pick<
    RuntimeModelProfileSummary,
    "modelId" | "modelDisplayName" | "displayName"
  >,
): string {
  return profile.modelDisplayName || profile.modelId || profile.displayName;
}

export function modelPreferenceChangeForOption(
  profileId: string,
  selectedProfileId: string,
  reasoningEffort?: ReasoningEffortLevel,
):
  | { modelProfileId: string; reasoningEffort?: ReasoningEffortLevel }
  | undefined {
  if (profileId === selectedProfileId) return undefined;
  return { modelProfileId: profileId, reasoningEffort };
}

export function modelPreferenceRecoveryKey(
  threadId: string | undefined,
  persistedModelProfileId: string | undefined,
  selectedModelProfileId: string | undefined,
  persistedReasoningEffort: ReasoningEffortLevel | undefined,
  selectedReasoningEffort: ReasoningEffortLevel | undefined,
): string {
  if (!threadId || !selectedModelProfileId) return "";

  const persistedModelIsUnavailable =
    persistedModelProfileId !== undefined &&
    persistedModelProfileId !== selectedModelProfileId;
  const persistedEffortIsUnavailable =
    persistedReasoningEffort !== undefined &&
    persistedReasoningEffort !== selectedReasoningEffort;
  if (!persistedModelIsUnavailable && !persistedEffortIsUnavailable) return "";

  return `${threadId}:${persistedModelProfileId ?? ""}:${persistedReasoningEffort ?? ""}`;
}

export function modelProfileIcon(profile: RuntimeModelProfileSummary) {
  const brand = providerBrandForId(profile.providerId);
  return brand ? (
    <AwakenProviderLogo brand={brand} name={profile.displayName} />
  ) : (
    profile.displayName.trim().slice(0, 1) || "M"
  );
}
