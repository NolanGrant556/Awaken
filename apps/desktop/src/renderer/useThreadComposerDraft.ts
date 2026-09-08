import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import type { ComposerBarInputSelection } from "@awaken/design-system";
import type { ComposerToken, FileLineComment } from "../shared/runtime";
import type { ComposerDraftAttachment } from "./composerAttachments";
import type { ComposerTokenPlacement } from "./composerSlash";

export type ThreadComposerDraft = {
  value: string;
  attachments: ComposerDraftAttachment[];
  tokens: ComposerToken[];
  comments: FileLineComment[];
  tokenPlacements: Record<string, ComposerTokenPlacement>;
  tokenLeadingTexts: Record<string, string>;
  selection?: ComposerBarInputSelection;
};

const EMPTY_COMPOSER_DRAFT: ThreadComposerDraft = {
  value: "",
  attachments: [],
  tokens: [],
  comments: [],
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
  const [state, setState] = useState<{
    drafts: Record<string, ThreadComposerDraft>;
    pendingSubmissions: Record<string, symbol>;
  }>({ drafts: {}, pendingSubmissions: {} });
  const draft = threadComposerDraftForOwner(state.drafts, ownerKey);

  const updateField = useCallback(
    <Key extends keyof ThreadComposerDraft>(
      field: Key,
      action: SetStateAction<ThreadComposerDraft[Key]>,
    ): void => {
      setState((current) => {
        const drafts = updateThreadComposerDraftField(current.drafts, ownerKey, field, action);
        if (drafts === current.drafts) return current;
        const pendingSubmissions = { ...current.pendingSubmissions };
        // Caret movement after clearing the input is not a new user draft.
        if (field !== "selection") delete pendingSubmissions[ownerKey];
        return { drafts, pendingSubmissions };
      });
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
  const setComments = useCallback<Dispatch<SetStateAction<FileLineComment[]>>>(
    (action) => updateField("comments", action),
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
      setState((current) => {
        const pendingSubmissions = { ...current.pendingSubmissions };
        delete pendingSubmissions[targetOwnerKey];
        return { drafts: { ...current.drafts, [targetOwnerKey]: nextDraft }, pendingSubmissions };
      });
    },
    [ownerKey],
  );

  const clearDraft = useCallback(
    (targetOwnerKey = ownerKey): void => {
      replaceDraft(EMPTY_COMPOSER_DRAFT, targetOwnerKey);
    },
    [ownerKey, replaceDraft],
  );

  // The returned recovery action belongs to this submission and this owner,
  // even after navigation. A later edit/reset/submission invalidates it.
  const clearForSubmission = useCallback((targetOwnerKey = ownerKey) => {
    const submission = Symbol("draft-submission");
    setState((current) => ({
      drafts: { ...current.drafts, [targetOwnerKey]: EMPTY_COMPOSER_DRAFT },
      pendingSubmissions: { ...current.pendingSubmissions, [targetOwnerKey]: submission },
    }));
    return (restoredDraft: ThreadComposerDraft): void => {
      setState((current) => {
        if (current.pendingSubmissions[targetOwnerKey] !== submission) return current;
        const pendingSubmissions = { ...current.pendingSubmissions };
        delete pendingSubmissions[targetOwnerKey];
        return { drafts: { ...current.drafts, [targetOwnerKey]: restoredDraft }, pendingSubmissions };
      });
    };
  }, [ownerKey]);

  return {
    draft,
    setValue,
    setAttachments,
    setTokens,
    setComments,
    setTokenPlacements,
    setTokenLeadingTexts,
    setSelection,
    replaceDraft,
    clearDraft,
    clearForSubmission,
  };
}
