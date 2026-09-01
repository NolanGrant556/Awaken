import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  type ClipboardEvent as ReactClipboardEvent,
  type CSSProperties,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { getAwakenIcon, iconForConcept, type AwakenIconName } from "../../icons";
import { composerTokenIconConcept } from "../../composerTokenIcon";
import {
  awakenPluginBrandAsset,
  isAwakenPluginBrand,
} from "../AwakenPluginBrandIcon";
import {
  type ComposerInputCaretRect,
  pastedImageFilesFromClipboardData,
  shouldSubmitComposerInputKey,
} from "../AwakenComposerInput";

export type ComposerInlineInputElement = HTMLDivElement;

export type ComposerInlineToken = {
  id: string;
  type: string;
  label: string;
  title?: string;
  leadingText?: string;
  brandIcon?: string;
};

export type ComposerInlineTokenSegment = {
  id: string;
  leadingText: string;
};

export type ComposerInlineStructuredValue = {
  tokens: ComposerInlineTokenSegment[];
  value: string;
};

type ComposerInlineInputStyle = CSSProperties & {
  "--composer-inline-input-min-height": string;
  "--composer-inline-input-max-height": string;
};

export type ComposerInlineSelection = {
  anchor: number;
  focus: number;
  caretRect?: ComposerInputCaretRect;
};

type ComposerInlinePart = { type: "text"; value: string } | { type: "token"; id: string };

export type AwakenComposerInlineInputProps = {
  value: string;
  tokens: ComposerInlineToken[];
  onChange: (value: string) => void;
  onStructuredChange?: (value: ComposerInlineStructuredValue) => void;
  onSubmit?: () => void;
  submitOnEnter?: boolean;
  onKeyDown?: (event: ReactKeyboardEvent<ComposerInlineInputElement>) => void;
  onRemoveToken?: (tokenId: string) => void;
  ariaLabel?: string;
  placeholder?: string;
  minRows?: number;
  maxRows?: number;
  disabled?: boolean;
  focusRequest?: number;
  focusOffset?: number;
  onSelectionChange?: (selection: ComposerInlineSelection) => void;
  onPasteImages?: (files: File[]) => void;
};

const COMPOSER_TOKEN_ATTRIBUTE = "data-composer-token-id";
const COMPOSER_TOKEN_SELECTED_ATTRIBUTE = "data-composer-token-selected";
const COMPOSER_TOKEN_ICON_SIZE = 16;
const BLOCK_TAG_NAMES = new Set([
  "ADDRESS",
  "ARTICLE",
  "ASIDE",
  "BLOCKQUOTE",
  "DIV",
  "FOOTER",
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "HEADER",
  "LI",
  "MAIN",
  "NAV",
  "OL",
  "P",
  "PRE",
  "SECTION",
  "UL",
]);

function normalizeComposerText(value: string): string {
  return value.replace(/\r\n?/g, "\n").replace(/\u00a0/g, " ");
}

function composerInlineParts(root: Node): ComposerInlinePart[] {
  const parts: ComposerInlinePart[] = [];
  let hasOutput = false;
  let outputEndsWithNewline = false;

  function appendText(value: string): void {
    const normalized = normalizeComposerText(value);
    if (!normalized) {
      return;
    }
    const previous = parts[parts.length - 1];
    if (previous?.type === "text") {
      previous.value += normalized;
    } else {
      parts.push({ type: "text", value: normalized });
    }
    hasOutput = true;
    outputEndsWithNewline = normalized.endsWith("\n");
  }

  function appendToken(id: string): void {
    parts.push({ type: "token", id });
    hasOutput = true;
    outputEndsWithNewline = false;
  }

  function visitChildren(parent: Node): void {
    const children = Array.from(parent.childNodes);
    children.forEach((child, index) => {
      const element = child.nodeType === Node.ELEMENT_NODE ? (child as HTMLElement) : undefined;
      const isBlock = Boolean(element && BLOCK_TAG_NAMES.has(element.tagName));
      if (isBlock && hasOutput && !outputEndsWithNewline) {
        appendText("\n");
      }
      visit(child);
      if (isBlock && index < children.length - 1 && hasOutput && !outputEndsWithNewline) {
        appendText("\n");
      }
    });
  }

  function visit(node: Node): void {
    if (node.nodeType === Node.TEXT_NODE) {
      appendText(node.nodeValue ?? "");
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
      visitChildren(node);
      return;
    }

    const element = node as HTMLElement;
    const tokenId = element.getAttribute(COMPOSER_TOKEN_ATTRIBUTE);
    if (tokenId) {
      appendToken(tokenId);
      return;
    }
    if (element.tagName === "BR") {
      appendText("\n");
      return;
    }
    visitChildren(element);
  }

  visitChildren(root);
  return parts;
}

function structuredValueFromParts(
  parts: ComposerInlinePart[],
  allowedTokenIds?: ReadonlySet<string>,
): ComposerInlineStructuredValue {
  const tokens: ComposerInlineTokenSegment[] = [];
  const seenTokenIds = new Set<string>();
  let pendingText = "";

  for (const part of parts) {
    if (part.type === "text") {
      pendingText += part.value;
      continue;
    }
    if (seenTokenIds.has(part.id) || (allowedTokenIds && !allowedTokenIds.has(part.id))) {
      continue;
    }
    seenTokenIds.add(part.id);
    tokens.push({ id: part.id, leadingText: pendingText });
    pendingText = "";
  }

  return { tokens, value: pendingText };
}

export function composerInlineInputStructuredValue(
  element: HTMLElement,
  allowedTokenIds?: ReadonlySet<string>,
): ComposerInlineStructuredValue {
  return structuredValueFromParts(composerInlineParts(element), allowedTokenIds);
}

function structuredValueForProps(
  tokens: ComposerInlineToken[],
  value: string,
): ComposerInlineStructuredValue {
  return {
    tokens: tokens.map((token) => ({
      id: token.id,
      leadingText: token.leadingText ?? "",
    })),
    value,
  };
}

function structuredValuesEqual(
  left: ComposerInlineStructuredValue,
  right: ComposerInlineStructuredValue,
): boolean {
  return (
    left.value === right.value &&
    left.tokens.length === right.tokens.length &&
    left.tokens.every(
      (token, index) =>
        token.id === right.tokens[index]?.id &&
        token.leadingText === right.tokens[index]?.leadingText,
    )
  );
}

function tokenRenderSignature(tokens: ComposerInlineToken[]): string {
  return JSON.stringify(
    tokens.map(({ id, type, label, title, brandIcon }) => [
      id,
      type,
      label,
      title ?? "",
      brandIcon ?? "",
    ]),
  );
}

function appendAwakenLineIcon(
  document: Document,
  token: HTMLElement,
  name: AwakenIconName,
): void {
  const icon = getAwakenIcon(name);
  if (icon.mode !== "line") {
    throw new Error(`Composer token 只接受线性图标：${name}`);
  }
  const namespace = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(namespace, "svg");
  svg.setAttribute("class", `composer-bar-token-icon aw-icon aw-icon--${name}`);
  svg.setAttribute("width", String(COMPOSER_TOKEN_ICON_SIZE));
  svg.setAttribute("height", String(COMPOSER_TOKEN_ICON_SIZE));
  svg.setAttribute("viewBox", icon.viewBox);
  svg.setAttribute("fill", icon.filled ? "currentColor" : "none");
  svg.setAttribute("stroke", icon.filled ? "none" : "currentColor");
  svg.setAttribute("stroke-width", String(icon.strokeWidth));
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("focusable", "false");
  svg.setAttribute("aria-hidden", "true");
  for (const [index, pathData] of icon.paths.entries()) {
    const path = document.createElementNS(namespace, "path");
    path.setAttribute("d", pathData);
    if (icon.filledPathIndexes?.includes(index)) {
      path.setAttribute("fill", "currentColor");
    }
    svg.append(path);
  }
  token.append(svg);
}

function composerTokenElement(document: Document, token: ComposerInlineToken): HTMLElement {
  const element = document.createElement("span");
  element.className = `composer-bar-token composer-bar-token-${token.type}`;
  element.setAttribute(COMPOSER_TOKEN_ATTRIBUTE, token.id);
  element.setAttribute("contenteditable", "false");
  element.setAttribute("draggable", "false");
  element.title = token.title ?? token.label;
  if (isAwakenPluginBrand(token.brandIcon)) {
    const image = document.createElement("img");
    image.className = "composer-bar-token-icon composer-bar-token-brand-icon";
    image.src = awakenPluginBrandAsset(token.brandIcon);
    image.alt = "";
    image.draggable = false;
    image.setAttribute("aria-hidden", "true");
    element.append(image);
  } else {
    appendAwakenLineIcon(
      document,
      element,
      iconForConcept(composerTokenIconConcept(token.type)),
    );
  }

  const label = document.createElement("span");
  label.className = "composer-bar-token-label";
  label.textContent = token.label;
  element.append(label);
  return element;
}

function rebuildComposerInlineInput(
  element: HTMLElement,
  tokens: ComposerInlineToken[],
  value: ComposerInlineStructuredValue,
): void {
  const tokenById = new Map(tokens.map((token) => [token.id, token]));
  const fragment = element.ownerDocument.createDocumentFragment();
  for (const segment of value.tokens) {
    const token = tokenById.get(segment.id);
    if (!token) {
      continue;
    }
    fragment.append(element.ownerDocument.createTextNode(segment.leadingText));
    fragment.append(composerTokenElement(element.ownerDocument, token));
  }
  fragment.append(element.ownerDocument.createTextNode(value.value));
  element.replaceChildren(fragment);
}

function structuredValueLength(value: ComposerInlineStructuredValue): number {
  return (
    value.value.length +
    value.tokens.reduce((length, token) => length + token.leadingText.length + 1, 0)
  );
}

function composerInlineSelectionOffset(
  element: HTMLElement,
  container: Node,
  offset: number,
): number {
  const range = element.ownerDocument.createRange();
  range.selectNodeContents(element);
  range.setEnd(container, offset);
  return composerInlineParts(range.cloneContents()).reduce(
    (length, part) => length + (part.type === "text" ? part.value.length : 1),
    0,
  );
}

export function composerInlineInputSelection(element: HTMLElement): ComposerInlineSelection | null {
  const selection = element.ownerDocument.defaultView?.getSelection();
  if (
    !selection?.rangeCount ||
    !selection.anchorNode ||
    !selection.focusNode ||
    !(selection.anchorNode === element || element.contains(selection.anchorNode)) ||
    !(selection.focusNode === element || element.contains(selection.focusNode))
  ) {
    return null;
  }
  const logicalSelection = {
    anchor: composerInlineSelectionOffset(element, selection.anchorNode, selection.anchorOffset),
    focus: composerInlineSelectionOffset(element, selection.focusNode, selection.focusOffset),
  };
  if (!selection.isCollapsed) {
    return logicalSelection;
  }
  const range = selection.getRangeAt(0).cloneRange();
  range.collapse(false);
  const elementRect = element.getBoundingClientRect();
  const rangeRects = typeof range.getClientRects === "function" ? range.getClientRects() : undefined;
  const rect =
    rangeRects?.[0] ??
    (typeof range.getBoundingClientRect === "function" ? range.getBoundingClientRect() : elementRect);
  const lineHeight =
    Number.parseFloat(
      element.ownerDocument.defaultView?.getComputedStyle(element).lineHeight ?? "",
    ) || 20;
  return {
    ...logicalSelection,
    caretRect: {
      left: rect.left || elementRect.left,
      top: rect.top || elementRect.top,
      bottom: rect.bottom || (rect.top || elementRect.top) + lineHeight,
    },
  };
}

export function syncComposerInlineTokenSelection(element: HTMLElement): void {
  const selection = composerInlineInputSelection(element);
  const selectedTokenIds = new Set<string>();
  if (selection && selection.anchor !== selection.focus) {
    const start = Math.min(selection.anchor, selection.focus);
    const end = Math.max(selection.anchor, selection.focus);
    const value = composerInlineInputStructuredValue(element);
    for (const range of tokenRanges(value)) {
      if (start < range.end && end > range.start) {
        selectedTokenIds.add(range.id);
      }
    }
  }
  for (const token of element.querySelectorAll<HTMLElement>(`[${COMPOSER_TOKEN_ATTRIBUTE}]`)) {
    token.toggleAttribute(
      COMPOSER_TOKEN_SELECTED_ATTRIBUTE,
      selectedTokenIds.has(token.getAttribute(COMPOSER_TOKEN_ATTRIBUTE) ?? ""),
    );
  }
}

function composerInlinePointForOffset(
  element: HTMLElement,
  requestedOffset: number,
): { node: Node; offset: number } {
  let remaining = Math.max(0, requestedOffset);
  const children = Array.from(element.childNodes);
  for (let index = 0; index < children.length; index += 1) {
    const child = children[index];
    if (child?.nodeType === Node.TEXT_NODE) {
      const length = child.nodeValue?.length ?? 0;
      if (remaining <= length) {
        return { node: child, offset: remaining };
      }
      remaining -= length;
      continue;
    }
    if (
      child?.nodeType === Node.ELEMENT_NODE &&
      (child as HTMLElement).hasAttribute(COMPOSER_TOKEN_ATTRIBUTE)
    ) {
      if (remaining === 0) {
        return { node: element, offset: index };
      }
      remaining -= 1;
    }
  }
  return { node: element, offset: children.length };
}

function restoreComposerInlineSelection(
  element: HTMLElement,
  selectionValue: ComposerInlineSelection,
): void {
  const selection = element.ownerDocument.defaultView?.getSelection();
  if (!selection) {
    return;
  }
  const value = composerInlineInputStructuredValue(element);
  const maxOffset = structuredValueLength(value);
  const anchor = composerInlinePointForOffset(element, Math.min(selectionValue.anchor, maxOffset));
  const focus = composerInlinePointForOffset(element, Math.min(selectionValue.focus, maxOffset));
  selection.removeAllRanges();
  selection.setBaseAndExtent(anchor.node, anchor.offset, focus.node, focus.offset);
}

function focusInlineInputAtOffset(
  element: HTMLElement,
  requestedOffset?: number,
): ComposerInlineSelection {
  element.focus({ preventScroll: true });
  const length = structuredValueLength(composerInlineInputStructuredValue(element));
  const offset = Math.max(0, Math.min(requestedOffset ?? length, length));
  const selection = { anchor: offset, focus: offset };
  restoreComposerInlineSelection(element, selection);
  return selection;
}

function tokenRanges(value: ComposerInlineStructuredValue): Array<{
  id: string;
  start: number;
  end: number;
}> {
  const ranges: Array<{ id: string; start: number; end: number }> = [];
  let offset = 0;
  for (const token of value.tokens) {
    offset += token.leadingText.length;
    ranges.push({ id: token.id, start: offset, end: offset + 1 });
    offset += 1;
  }
  return ranges;
}

function partsForStructuredValue(value: ComposerInlineStructuredValue): ComposerInlinePart[] {
  const parts: ComposerInlinePart[] = [];
  for (const token of value.tokens) {
    parts.push({ type: "text", value: token.leadingText });
    parts.push({ type: "token", id: token.id });
  }
  parts.push({ type: "text", value: value.value });
  return parts;
}

function deleteComposerInlineRange(
  value: ComposerInlineStructuredValue,
  requestedStart: number,
  requestedEnd: number,
): ComposerInlineStructuredValue {
  const totalLength = structuredValueLength(value);
  const start = Math.max(0, Math.min(requestedStart, requestedEnd, totalLength));
  const end = Math.max(0, Math.min(Math.max(requestedStart, requestedEnd), totalLength));
  const nextParts: ComposerInlinePart[] = [];
  let offset = 0;

  for (const part of partsForStructuredValue(value)) {
    const length = part.type === "text" ? part.value.length : 1;
    const partEnd = offset + length;
    if (part.type === "text") {
      const deleteStart = Math.max(0, start - offset);
      const deleteEnd = Math.min(length, end - offset);
      const nextText =
        deleteEnd > deleteStart
          ? `${part.value.slice(0, deleteStart)}${part.value.slice(deleteEnd)}`
          : part.value;
      if (nextText) {
        const previous = nextParts[nextParts.length - 1];
        if (previous?.type === "text") {
          previous.value += nextText;
        } else {
          nextParts.push({ type: "text", value: nextText });
        }
      }
    } else if (!(start < partEnd && end > offset)) {
      nextParts.push(part);
    }
    offset = partEnd;
  }

  return structuredValueFromParts(nextParts);
}

function appendPlainTextSegment(output: string, segment: string): string {
  if (!segment) {
    return output;
  }
  if (!output) {
    return segment;
  }
  if (/\s$/u.test(output) || /^\s/u.test(segment)) {
    return `${output}${segment}`;
  }
  return `${output} ${segment}`;
}

/** Visible clipboard text for a logical range: plaintext + token labels, no soft-wrap breaks. */
export function composerInlinePlainTextForRange(
  value: ComposerInlineStructuredValue,
  requestedStart: number,
  requestedEnd: number,
  tokenLabels: ReadonlyMap<string, string> = new Map(),
): string {
  const start = Math.max(0, Math.min(requestedStart, requestedEnd));
  const end = Math.max(requestedStart, requestedEnd);
  let output = "";
  let lastWasToken = false;
  let offset = 0;
  for (const part of partsForStructuredValue(value)) {
    const length = part.type === "text" ? part.value.length : 1;
    const partEnd = offset + length;
    if (start < partEnd && end > offset) {
      if (part.type === "text") {
        const sliceStart = Math.max(0, start - offset);
        const sliceEnd = Math.min(length, end - offset);
        if (sliceEnd > sliceStart) {
          const slice = part.value.slice(sliceStart, sliceEnd);
          output = lastWasToken ? appendPlainTextSegment(output, slice) : `${output}${slice}`;
          lastWasToken = false;
        }
      } else {
        const label = tokenLabels.get(part.id) ?? "";
        if (label) {
          output = appendPlainTextSegment(output, label);
          lastWasToken = true;
        }
      }
    }
    offset = partEnd;
  }
  return output;
}

export function AwakenComposerInlineInput({
  value,
  tokens,
  onChange,
  onStructuredChange,
  onSubmit,
  submitOnEnter = true,
  onKeyDown,
  onRemoveToken,
  ariaLabel = "输入任务",
  placeholder = "",
  minRows = 2,
  maxRows = 6,
  disabled,
  focusRequest = 0,
  focusOffset,
  onSelectionChange,
  onPasteImages,
}: AwakenComposerInlineInputProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const isComposingRef = useRef(false);
  const compositionGenerationRef = useRef(0);
  const renderedTokenSignatureRef = useRef("");
  const onSelectionChangeRef = useRef(onSelectionChange);
  onSelectionChangeRef.current = onSelectionChange;
  const tokenIds = new Set(tokens.map((token) => token.id));
  const desiredValue = structuredValueForProps(tokens, value);
  const style: ComposerInlineInputStyle = {
    "--composer-inline-input-min-height": `${minRows * 20}px`,
    "--composer-inline-input-max-height": `${maxRows * 20}px`,
  };

  const emitSelection = useCallback(() => {
    const editor = editorRef.current;
    const selection = editor ? composerInlineInputSelection(editor) : null;
    if (selection) {
      onSelectionChangeRef.current?.(selection);
    }
  }, []);

  useLayoutEffect(() => {
    const editor = editorRef.current;
    if (!editor || isComposingRef.current) {
      return;
    }
    const currentValue = composerInlineInputStructuredValue(editor, tokenIds);
    const nextTokenSignature = tokenRenderSignature(tokens);
    if (
      structuredValuesEqual(currentValue, desiredValue) &&
      renderedTokenSignatureRef.current === nextTokenSignature
    ) {
      return;
    }

    const hadFocus = editor.ownerDocument.activeElement === editor;
    const selection = hadFocus ? composerInlineInputSelection(editor) : null;
    rebuildComposerInlineInput(editor, tokens, desiredValue);
    renderedTokenSignatureRef.current = nextTokenSignature;
    if (hadFocus && selection) {
      editor.focus({ preventScroll: true });
      restoreComposerInlineSelection(editor, selection);
    }
  });

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || disabled || focusRequest <= 0) {
      return;
    }
    const selection = focusInlineInputAtOffset(editor, focusOffset);
    onSelectionChangeRef.current?.(selection);
    // A new request is the only command to reposition the logical caret.
    // Callback identity and draft rerenders must leave the live selection in
    // browser ownership.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [disabled, focusRequest]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || disabled) {
      return;
    }
    const handleSelectionChange = (): void => {
      syncComposerInlineTokenSelection(editor);
      emitSelection();
    };
    editor.ownerDocument.addEventListener("selectionchange", handleSelectionChange);
    handleSelectionChange();
    return () => {
      editor.ownerDocument.removeEventListener("selectionchange", handleSelectionChange);
    };
  }, [disabled, emitSelection]);

  const emitStructuredValue = useCallback(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    const nextValue = composerInlineInputStructuredValue(
      editor,
      new Set(tokens.map((token) => token.id)),
    );
    onChange(nextValue.value);
    onStructuredChange?.(nextValue);
  }, [onChange, onStructuredChange, tokens]);

  const applyDeletion = useCallback(
    (start: number, end: number): boolean => {
      const editor = editorRef.current;
      if (!editor) {
        return false;
      }
      const currentValue = composerInlineInputStructuredValue(
        editor,
        new Set(tokens.map((token) => token.id)),
      );
      const removedTokenIds = tokenRanges(currentValue)
        .filter((range) => start < range.end && end > range.start)
        .map((range) => range.id);
      if (removedTokenIds.length === 0) {
        return false;
      }

      if (!onStructuredChange) {
        if (removedTokenIds.length === 1 && start + 1 === end && onRemoveToken) {
          onRemoveToken(removedTokenIds[0] ?? "");
          return true;
        }
        return false;
      }

      const nextValue = deleteComposerInlineRange(currentValue, start, end);
      const remainingTokenIds = new Set(nextValue.tokens.map((token) => token.id));
      const nextTokens = tokens
        .filter((token) => remainingTokenIds.has(token.id))
        .map((token) => ({
          ...token,
          leadingText:
            nextValue.tokens.find((segment) => segment.id === token.id)?.leadingText ?? "",
        }));
      rebuildComposerInlineInput(editor, nextTokens, nextValue);
      renderedTokenSignatureRef.current = tokenRenderSignature(nextTokens);
      editor.focus({ preventScroll: true });
      restoreComposerInlineSelection(editor, { anchor: start, focus: start });
      onChange(nextValue.value);
      onStructuredChange(nextValue);
      return true;
    },
    [onChange, onRemoveToken, onStructuredChange, tokens],
  );

  const handleInput = useCallback(() => {
    if (isComposingRef.current) {
      return;
    }
    compositionGenerationRef.current += 1;
    emitStructuredValue();
    emitSelection();
  }, [emitSelection, emitStructuredValue]);

  const handleKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      onKeyDown?.(event);
      if (event.defaultPrevented) {
        return;
      }

      const editor = editorRef.current;
      const selection = editor ? composerInlineInputSelection(editor) : null;
      if (editor && selection && (event.key === "Backspace" || event.key === "Delete")) {
        const currentValue = composerInlineInputStructuredValue(
          editor,
          new Set(tokens.map((token) => token.id)),
        );
        const start = Math.min(selection.anchor, selection.focus);
        const end = Math.max(selection.anchor, selection.focus);
        let deleteStart = start;
        let deleteEnd = end;

        if (start === end) {
          const adjacentToken = tokenRanges(currentValue).find((range) =>
            event.key === "Backspace" ? range.end === start : range.start === start,
          );
          if (adjacentToken) {
            deleteStart = adjacentToken.start;
            deleteEnd = adjacentToken.end;
          }
        }

        if (
          deleteEnd > deleteStart &&
          tokenRanges(currentValue).some(
            (range) => deleteStart < range.end && deleteEnd > range.start,
          ) &&
          applyDeletion(deleteStart, deleteEnd)
        ) {
          event.preventDefault();
          return;
        }
      }

      if (
        !submitOnEnter ||
        !shouldSubmitComposerInputKey({
          key: event.key,
          shiftKey: event.shiftKey,
          isComposing: isComposingRef.current || event.nativeEvent.isComposing,
        })
      ) {
        return;
      }
      event.preventDefault();
      onSubmit?.();
    },
    [applyDeletion, onKeyDown, onSubmit, submitOnEnter, tokens],
  );

  const handleBeforeInput = useCallback(
    (event: FormEvent<HTMLDivElement>) => {
      const nativeEvent = event.nativeEvent as InputEvent;
      if (
        typeof nativeEvent.inputType !== "string" ||
        !nativeEvent.inputType.startsWith("delete")
      ) {
        return;
      }
      const editor = editorRef.current;
      const selection = editor ? composerInlineInputSelection(editor) : null;
      if (!editor || !selection) {
        return;
      }
      const currentValue = composerInlineInputStructuredValue(
        editor,
        new Set(tokens.map((token) => token.id)),
      );
      let start = Math.min(selection.anchor, selection.focus);
      let end = Math.max(selection.anchor, selection.focus);
      if (start === end) {
        const adjacentToken = tokenRanges(currentValue).find((range) =>
          nativeEvent.inputType.endsWith("Backward") ? range.end === start : range.start === start,
        );
        if (adjacentToken) {
          start = adjacentToken.start;
          end = adjacentToken.end;
        }
      }
      if (end > start && applyDeletion(start, end)) {
        event.preventDefault();
      }
    },
    [applyDeletion, tokens],
  );

  const handlePaste = useCallback(
    (event: ReactClipboardEvent<HTMLDivElement>) => {
      if (!onPasteImages) {
        return;
      }
      const files = pastedImageFilesFromClipboardData(event.clipboardData);
      if (files.length === 0) {
        return;
      }
      event.preventDefault();
      onPasteImages(files);
    },
    [onPasteImages],
  );

  const tokenLabels = useMemo(
    () => new Map(tokens.map((token) => [token.id, token.label])),
    [tokens],
  );

  const handleCopy = useCallback(
    (event: ReactClipboardEvent<HTMLDivElement>) => {
      const editor = editorRef.current;
      const selection = editor ? composerInlineInputSelection(editor) : null;
      if (!editor || !selection || selection.anchor === selection.focus) {
        return;
      }
      const currentValue = composerInlineInputStructuredValue(
        editor,
        new Set(tokens.map((token) => token.id)),
      );
      event.clipboardData.setData(
        "text/plain",
        composerInlinePlainTextForRange(
          currentValue,
          selection.anchor,
          selection.focus,
          tokenLabels,
        ),
      );
      event.preventDefault();
    },
    [tokenLabels, tokens],
  );

  const handleCut = useCallback(
    (event: ReactClipboardEvent<HTMLDivElement>) => {
      const editor = editorRef.current;
      const selection = editor ? composerInlineInputSelection(editor) : null;
      if (!editor || !selection || selection.anchor === selection.focus || !onStructuredChange) {
        return;
      }
      const currentValue = composerInlineInputStructuredValue(
        editor,
        new Set(tokens.map((token) => token.id)),
      );
      event.clipboardData.setData(
        "text/plain",
        composerInlinePlainTextForRange(
          currentValue,
          selection.anchor,
          selection.focus,
          tokenLabels,
        ),
      );
      event.preventDefault();
      const start = Math.min(selection.anchor, selection.focus);
      const end = Math.max(selection.anchor, selection.focus);
      const nextValue = deleteComposerInlineRange(currentValue, start, end);
      const remainingTokenIds = new Set(nextValue.tokens.map((token) => token.id));
      const nextTokens = tokens
        .filter((token) => remainingTokenIds.has(token.id))
        .map((token) => ({
          ...token,
          leadingText:
            nextValue.tokens.find((segment) => segment.id === token.id)?.leadingText ?? "",
        }));
      rebuildComposerInlineInput(editor, nextTokens, nextValue);
      renderedTokenSignatureRef.current = tokenRenderSignature(nextTokens);
      editor.focus({ preventScroll: true });
      restoreComposerInlineSelection(editor, { anchor: start, focus: start });
      onChange(nextValue.value);
      onStructuredChange(nextValue);
    },
    [onChange, onStructuredChange, tokenLabels, tokens],
  );

  return (
    <div
      ref={editorRef}
      className={`composer-bar-inline-editor${
        disabled ? " composer-bar-inline-editor-disabled" : ""
      }`}
      style={style}
      contentEditable={disabled ? false : "plaintext-only"}
      suppressContentEditableWarning
      role="textbox"
      aria-label={ariaLabel}
      aria-multiline="true"
      aria-disabled={disabled || undefined}
      data-placeholder={placeholder}
      tabIndex={disabled ? -1 : 0}
      onBeforeInput={handleBeforeInput}
      onInput={handleInput}
      onScroll={emitSelection}
      onKeyDown={handleKeyDown}
      onPaste={handlePaste}
      onCopy={handleCopy}
      onCut={handleCut}
      onDragStart={(event) => event.preventDefault()}
      onDrop={(event) => event.preventDefault()}
      onCompositionStart={() => {
        isComposingRef.current = true;
        compositionGenerationRef.current += 1;
      }}
      onCompositionEnd={() => {
        isComposingRef.current = false;
        const generation = compositionGenerationRef.current + 1;
        compositionGenerationRef.current = generation;
        queueMicrotask(() => {
          if (compositionGenerationRef.current === generation) {
            emitStructuredValue();
          }
        });
      }}
    />
  );
}
