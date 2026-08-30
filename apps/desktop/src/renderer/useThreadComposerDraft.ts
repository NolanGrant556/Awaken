import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import type { ComposerBarInputSelection } from "@awaken/design-system";
import type { ComposerToken } from "../shared/runtime";
import type { ComposerDraftAttachment } from "./composerAttachments";
import type { ComposerTokenPlacement } from "./composerSlash";

export type ThreadComposerDraft = {
  value: string;
  attachments: ComposerDraftAttachment[];
  tokens: ComposerToken[];
  tokenPlacements: Record<string, ComposerTokenPlacement>;
  tokenLeadingTexts: Record<string, string>;
  selection?: ComposerBarInputSelection;
};

const EMPTY_COMPOSER_DRAFT: ThreadComposerDraft = {
  value: "",
  attachments: [],
  tokens: [],
  tokenPlacements: {},
  tokenLeadingTexts: {},
};

export function threadComposerDraftForOwner(
  drafts: Readonly<Record<string, ThreadComposerDraft>>,
  ownerKey: string,
): ThreadComposerDraft {
  return drafts[ownerKey] ?? EMPTY_COMPOSER_DRAFT;
}

export function updateThreadComposerDraftField<Key extends keyof ThreadComposerDraft>(
  drafts: Readonly<Record<string, ThreadComposerDraft>>,
  ownerKey: string,
  field: Key,
  action: SetStateAction<ThreadComposerDraft[Key]>,
): Record<string, ThreadComposerDraft> {
  const currentDraft = threadComposerDraftForOwner(drafts, ownerKey);
  const value = nextFieldValue(currentDraft[field], action);
  if (Object.is(value, currentDraft[field])) {
    return drafts as Record<string, ThreadComposerDraft>;
  }
  return {
    ...drafts,
    [ownerKey]: { ...currentDraft, [field]: value },
  };
}

export function composerDraftOwnerKey(threadId?: string): string {
  return threadId ? `thread:${threadId}` : "new-thread";
}

function nextFieldValue<T>(current: T, action: SetStateAction<T>): T {
  return typeof action === "function" ? (action as (value: T) => T)(current) : action;
}

export function composerSelectionsEqual(
  left: ComposerBarInputSelection | undefined,
  right: ComposerBarInputSelection | undefined,
): boolean {
  if (left === right) return true;
  if (!left || !right || left.anchor !== right.anchor || left.focus !== right.focus) return false;
  if (left.caretRect === right.caretRect) return true;
  if (!left.caretRect || !right.caretRect) return false;
  return (
    left.caretRect.left === right.caretRect.left &&
    left.caretRect.top === right.caretRect.top &&
    left.caretRect.bottom === right.caretRect.bottom
  );
}

export function useThreadComposerDraft(ownerKey: string) {
  const [drafts, setDrafts] = useState<Record<string, ThreadComposerDraft>>({});
  const draft = threadComposerDraftForOwner(drafts, ownerKey);

  const updateField = useCallback(
    <Key extends keyof ThreadComposerDraft>(
      field: Key,
      action: SetStateAction<ThreadComposerDraft[Key]>,
    ): void => {
      setDrafts((current) => updateThreadComposerDraftField(current, ownerKey, field, action));
    },
    [ownerKey],
  );

  const setValue = useCallback<Dispatch<SetStateAction<string>>>(
    (action) => updateField("value", action),
    [updateField],
  );
  const setAttachments = useCallback<Dispatch<SetStateAction<ComposerDraftAttachment[]>>>(
    (action) => updateField("attachments", action),
    [updateField],
  );
  const setTokens = useCallback<Dispatch<SetStateAction<ComposerToken[]>>>(
    (action) => updateField("tokens", action),
    [updateField],
  );
  const setTokenPlacements = useCallback<
    Dispatch<SetStateAction<Record<string, ComposerTokenPlacement>>>
  >((action) => updateField("tokenPlacements", action), [updateField]);
  const setTokenLeadingTexts = useCallback<Dispatch<SetStateAction<Record<string, string>>>>(
    (action) => updateField("tokenLeadingTexts", action),
    [updateField],
  );
  const setSelection = useCallback<Dispatch<SetStateAction<ComposerBarInputSelection | undefined>>>(
    (action) =>
      updateField("selection", (current) => {
        const next = nextFieldValue(current, action);
        return composerSelectionsEqual(current, next) ? current : next;
      }),
    [updateField],
  );

  const replaceDraft = useCallback(
    (nextDraft: ThreadComposerDraft, targetOwnerKey = ownerKey): void => {
      setDrafts((current) => ({ ...current, [targetOwnerKey]: nextDraft }));
    },
    [ownerKey],
  );

  const clearDraft = useCallback(
    (targetOwnerKey = ownerKey): void => {
      setDrafts((current) => ({ ...current, [targetOwnerKey]: EMPTY_COMPOSER_DRAFT }));
    },
    [ownerKey],
  );

  return {
    draft,
    setValue,
    setAttachments,
    setTokens,
    setTokenPlacements,
    setTokenLeadingTexts,
    setSelection,
    replaceDraft,
    clearDraft,
  };
}
