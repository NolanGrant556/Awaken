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

// Keep acknowledgements across thread navigation without rewriting historical preferences.
export function consumeModelPreferenceRecoveryNotice(
  seen: Set<string>, key: string, catalogReady: boolean,
): boolean {
  if (!catalogReady || !key || seen.has(key)) return false;
  seen.add(key);
  return true;
}

export function modelProfileIcon(profile: RuntimeModelProfileSummary) {
  const brand = providerBrandForId(profile.providerId);
  return <AwakenProviderLogo brand={brand} name={profile.displayName} />;
}
