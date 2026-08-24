import type { ComposerBarStructuredInputValue } from "@awaken/design-system";
import type { ComposerToken } from "../shared/runtime";
import type { ComposerTokenPlacement } from "./composerSlash";

export function composerTokenLeadingText(
  token: ComposerToken,
  leadingTexts: Record<string, string>,
): string {
  return token.leadingText ?? leadingTexts[token.id] ?? "";
}

export function reconcileComposerStructuredInput(
  tokens: ComposerToken[],
  placements: Record<string, ComposerTokenPlacement>,
  nextValue: ComposerBarStructuredInputValue,
): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
} {
  const leadingTextByTokenId = new Map(
    nextValue.tokens.map((token) => [token.id, token.leadingText]),
  );
  const currentTokenIds = new Set(tokens.map((token) => token.id));
  const remainingTokenIds = new Set(
    nextValue.tokens.map((token) => token.id).filter((tokenId) => currentTokenIds.has(tokenId)),
  );
  const nextTokens = tokens
    .filter((token) => remainingTokenIds.has(token.id))
    .map((token) => {
      const leadingText = leadingTextByTokenId.get(token.id) ?? "";
      const tokenWithoutLeadingText = { ...token };
      delete tokenWithoutLeadingText.leadingText;
      return leadingText ? { ...tokenWithoutLeadingText, leadingText } : tokenWithoutLeadingText;
    });

  return {
    tokens: nextTokens,
    placements: Object.fromEntries(
      Object.entries(placements).filter(([tokenId]) => remainingTokenIds.has(tokenId)),
    ),
    leadingTexts: Object.fromEntries(
      nextValue.tokens
        .filter((token) => remainingTokenIds.has(token.id) && token.leadingText)
        .map((token) => [token.id, token.leadingText]),
    ),
    value: nextValue.value,
  };
}

export function composerStructuredDraftLength(
  tokens: ComposerToken[],
  leadingTexts: Record<string, string>,
  value: string,
): number {
  return tokens.reduce(
    (length, token) => length + composerTokenLeadingText(token, leadingTexts).length + 1,
    value.length,
  );
}

export function insertComposerTokenAtOffset(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  offset: number;
  token: ComposerToken;
}): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  focusOffset: number;
} {
  const totalLength = composerStructuredDraftLength(args.tokens, args.leadingTexts, args.value);
  const offset = Math.max(0, Math.min(args.offset, totalLength));
  const nextTokens: ComposerToken[] = [];
  let consumed = 0;
  let inserted = false;

  for (const currentToken of args.tokens) {
    const leadingText = composerTokenLeadingText(currentToken, args.leadingTexts);
    if (!inserted && offset <= consumed + leadingText.length) {
      const splitOffset = offset - consumed;
      const before = leadingText.slice(0, splitOffset);
      const after = leadingText.slice(splitOffset);
      nextTokens.push(
        before ? { ...args.token, leadingText: before } : { ...args.token },
        after
          ? { ...currentToken, leadingText: after }
          : (() => {
              const token = { ...currentToken };
              delete token.leadingText;
              return token;
            })(),
      );
      inserted = true;
    } else {
      nextTokens.push(
        leadingText
          ? { ...currentToken, leadingText }
          : (() => {
              const token = { ...currentToken };
              delete token.leadingText;
              return token;
            })(),
      );
    }
    consumed += leadingText.length + 1;
  }

  let value = args.value;
  if (!inserted) {
    const tailOffset = Math.max(0, Math.min(offset - consumed, value.length));
    const before = value.slice(0, tailOffset);
    value = value.slice(tailOffset);
    nextTokens.push(before ? { ...args.token, leadingText: before } : { ...args.token });
  }

  return {
    tokens: nextTokens,
    placements: Object.fromEntries(
      nextTokens.map((token) => [
        token.id,
        token.id === args.token.id ? "after" : (args.placements[token.id] ?? "after"),
      ]),
    ),
    leadingTexts: Object.fromEntries(
      nextTokens
        .filter((token) => Boolean(token.leadingText))
        .map((token) => [token.id, token.leadingText ?? ""]),
    ),
    value,
    focusOffset: offset + 1,
  };
}

export function removeComposerTextRange(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  start: number;
  end: number;
}): {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
} {
  const start = Math.max(0, Math.min(args.start, args.end));
  const end = Math.max(start, args.end);
  let logicalOffset = 0;
  const tokens = args.tokens.map((token) => {
    const leadingText = composerTokenLeadingText(token, args.leadingTexts);
    const leadingEnd = logicalOffset + leadingText.length;
    if (start >= logicalOffset && end <= leadingEnd) {
      const nextLeadingText = `${leadingText.slice(0, start - logicalOffset)}${leadingText.slice(
        end - logicalOffset,
      )}`;
      logicalOffset = leadingEnd + 1;
      const nextToken = { ...token };
      if (nextLeadingText) {
        nextToken.leadingText = nextLeadingText;
      } else {
        delete nextToken.leadingText;
      }
      return nextToken;
    }
    logicalOffset = leadingEnd + 1;
    return leadingText ? { ...token, leadingText } : { ...token };
  });

  let value = args.value;
  const tailEnd = logicalOffset + value.length;
  if (start >= logicalOffset && end <= tailEnd) {
    value = `${value.slice(0, start - logicalOffset)}${value.slice(end - logicalOffset)}`;
  }

  return {
    tokens,
    placements: { ...args.placements },
    leadingTexts: Object.fromEntries(
      tokens
        .filter((token) => Boolean(token.leadingText))
        .map((token) => [token.id, token.leadingText ?? ""]),
    ),
    value,
  };
}

export function replaceComposerActionTriggerWithToken(args: {
  tokens: ComposerToken[];
  placements: Record<string, ComposerTokenPlacement>;
  leadingTexts: Record<string, string>;
  value: string;
  trigger: { start: number; end: number };
  token: ComposerToken;
}) {
  const withoutTrigger = removeComposerTextRange({
    tokens: args.tokens,
    placements: args.placements,
    leadingTexts: args.leadingTexts,
    value: args.value,
    start: args.trigger.start,
    end: args.trigger.end,
  });
  if (withoutTrigger.tokens.some((token) => token.id === args.token.id)) {
    return { ...withoutTrigger, focusOffset: args.trigger.start };
  }
  return insertComposerTokenAtOffset({
    ...withoutTrigger,
    offset: args.trigger.start,
    token: args.token,
  });
}
