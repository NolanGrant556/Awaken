import { builtinConnectorTokenBrand } from "./builtinConnectorComposer";
import type { ComposerToken, RuntimeRun } from "../shared/runtime";
import type { PromptNarrativeToken } from "./run-narrative";
import { type ThreadComposerDraft } from "./useThreadComposerDraft";
import { previewURLForRunInputAttachment } from "./composerAttachments";
import { composerTokenLeadingText } from "./composerStructuredDraft";

export function promptNarrativeTokensForComposer(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
): PromptNarrativeToken[] {
  return tokens.map((token) => {
    const leadingText = token.leadingText ?? leadingTexts[token.id];
    return {
      ...token,
      payload: token.payload ? { ...token.payload } : undefined,
      leadingText: leadingText || undefined,
    };
  });
}

export function composerTokenEndOffset(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
  tokenId: string,
): number | undefined {
  let offset = 0;
  for (const token of tokens) {
    offset += composerTokenLeadingText(token, leadingTexts).length + 1;
    if (token.id === tokenId) return offset;
  }
  return undefined;
}

export function promptTokenViews(
  tokens?: PromptNarrativeToken[],
  pluginBrandByName?: ReadonlyMap<string, string>,
) {
  return tokens?.map((token) => ({
    id: token.id,
    type: token.type,
    label: token.label,
    leadingText: token.leadingText || undefined,
    title: builtinConnectorTokenBrand(token) ? token.label :
      typeof token.payload?.skillFilePath === "string"
        ? token.payload.skillFilePath
        : token.label,
    brandIcon: builtinConnectorTokenBrand(token) ?? (
      token.type === "plugin" && typeof token.payload?.pluginName === "string"
        ? pluginBrandByName?.get(token.payload.pluginName)
        : undefined),
  }));
}

export function promptTextAfterTokenLeadingText(
  text: string,
  tokens?: ReadonlyArray<Pick<PromptNarrativeToken, "leadingText">>,
): string {
  let remainingText = text;
  for (const token of tokens ?? []) {
    const leadingText = token.leadingText;
    if (!leadingText) {
      continue;
    }

    // The submitted prompt trims only its outer boundary. Prefer the exact
    // durable segment so whitespace between the token and the suffix survives,
    // then tolerate only the outer trim variants for token-only/indented input.
    const candidates = [
      leadingText,
      leadingText.trimStart(),
      leadingText.trimEnd(),
      leadingText.trim(),
    ].filter(
      (candidate, index, values) =>
        candidate.length > 0 && values.indexOf(candidate) === index,
    );
    const matchedLeadingText = candidates.find((candidate) =>
      remainingText.startsWith(candidate),
    );
    if (!matchedLeadingText) {
      return text;
    }
    remainingText = remainingText.slice(matchedLeadingText.length);
  }
  return remainingText;
}

export const PROMPT_REWIND_SIDE_EFFECT_COPY =
  "这会移除该消息及后续对话，但不会撤销已经执行的文件修改或外部操作。";

export const PROMPT_REWIND_DRAFT_WARNING_COPY =
  "输入框中已有未发送内容，继续后将被这条消息替换。";

export function promptRewindConfirmDialog(input: {
  mode: "undo" | "edit";
  hasDraft: boolean;
}): { title: string; confirmLabel: string; paragraphs: string[] } {
  return {
    title: input.mode === "edit" ? "发送编辑后的消息？" : "撤销此消息？",
    confirmLabel: input.mode === "edit" ? "发送" : "撤销",
    paragraphs: input.hasDraft
      ? [PROMPT_REWIND_DRAFT_WARNING_COPY, PROMPT_REWIND_SIDE_EFFECT_COPY]
      : [PROMPT_REWIND_SIDE_EFFECT_COPY],
  };
}

export function threadSupportsMessageRewind(thread?: {
  archived?: boolean;
  source?: string;
}): boolean {
  return (
    !thread?.archived &&
    thread?.source !== "scheduled_task" &&
    thread?.source !== "acp_bridge"
  );
}

export function composerDraftFromRuntimeRun(
  run: RuntimeRun,
  value = promptTextAfterTokenLeadingText(run.input ?? "", run.composerTokens),
): ThreadComposerDraft {
  const tokens = (run.composerTokens ?? []).map((token) => ({
    ...token,
    payload: token.payload ? { ...token.payload } : undefined,
  }));
  const tokenLeadingTexts = Object.fromEntries(
    tokens.flatMap((token) =>
      token.leadingText ? [[token.id, token.leadingText] as const] : [],
    ),
  );
  return {
    value,
    attachments: (run.inputAttachments ?? []).map((attachment) => ({
      ...attachment,
      previewURL:
        attachment.kind === "image" && attachment.dataBase64
          ? previewURLForRunInputAttachment(attachment)
          : undefined,
    })),
    tokens,
    comments: (run.fileComments ?? []).map((comment) => ({ ...comment })),
    tokenPlacements: Object.fromEntries(
      tokens.map((token) => [token.id, "before" as const]),
    ),
    tokenLeadingTexts,
    selection: { anchor: value.length, focus: value.length },
  };
}

export function effectivePromptForComposerDraft(
  draft: ThreadComposerDraft,
): string {
  const prefix = draft.tokens
    .map(
      (token) => token.leadingText ?? draft.tokenLeadingTexts[token.id] ?? "",
    )
    .join("");
  return `${prefix}${draft.value}`.trim();
}

export function composerThreadLabel(
  prompt: string,
  attachments: Array<{ displayName?: string }>,
  tokens: ComposerToken[] = [],
): string {
  const trimmed = prompt.trim();
  if (trimmed) {
    return trimmed;
  }
  const attachmentName = attachments
    .map((attachment) => attachment.displayName?.trim())
    .find(Boolean);
  if (attachmentName) {
    return attachmentName;
  }
  const tokenLabel = tokens.map((token) => token.label.trim()).find(Boolean);
  return tokenLabel || "附件任务";
}
