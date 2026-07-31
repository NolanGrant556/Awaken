const textInputTypes = new Set([
  "email",
  "password",
  "search",
  "tel",
  "text",
  "url",
]);

const SELECT_ALL_ROOT_ATTRIBUTE = "data-aw-select-all-root";

function editableHostForElement(element: Element | null): HTMLElement | null {
  let current = element;
  while (current) {
    const contentEditable = current.getAttribute("contenteditable");
    // Atomic token chips use contenteditable=false inside the host; keep walking.
    if (
      contentEditable === "" ||
      contentEditable === "true" ||
      contentEditable === "plaintext-only"
    ) {
      return current as HTMLElement;
    }
    current = current.parentElement;
  }
  return null;
}

function selectAllRootForElement(element: Element | null): HTMLElement | null {
  let current = element;
  while (current) {
    if (
      current instanceof HTMLElement &&
      current.hasAttribute(SELECT_ALL_ROOT_ATTRIBUTE)
    ) {
      return current;
    }
    current = current.parentElement;
  }
  // Prompt bubbles focus the outer surface; the select-all root is a child.
  if (element instanceof HTMLElement) {
    return element.querySelector<HTMLElement>(`[${SELECT_ALL_ROOT_ATTRIBUTE}]`);
  }
  return null;
}

function elementFromSelection(document: Document): Element | null {
  const selection = document.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return null;
  }
  const node = selection.anchorNode;
  if (!node) {
    return null;
  }
  return node.nodeType === Node.ELEMENT_NODE
    ? (node as Element)
    : node.parentElement;
}

function clearDocumentSelection(document: Document): void {
  document.getSelection()?.removeAllRanges();
}

function selectNodeContents(document: Document, host: HTMLElement): boolean {
  const selection = document.getSelection();
  if (!selection) {
    return false;
  }
  const range = document.createRange();
  range.selectNodeContents(host);
  selection.removeAllRanges();
  selection.addRange(range);
  return true;
}

function resolveSelectAllHost(document: Document): HTMLElement | null {
  const activeElement = document.activeElement;
  const fromActiveEditable = editableHostForElement(activeElement);
  if (fromActiveEditable) {
    return fromActiveEditable;
  }

  if (
    activeElement instanceof HTMLTextAreaElement ||
    (activeElement instanceof HTMLInputElement &&
      textInputTypes.has(activeElement.type))
  ) {
    return activeElement;
  }

  const fromActiveReading = selectAllRootForElement(activeElement);
  if (fromActiveReading) {
    return fromActiveReading;
  }

  // After clicking atomic token chips or reading text, focus may sit on body
  // while the live selection still belongs to the intended host.
  const selectionElement = elementFromSelection(document);
  const fromSelectionEditable = editableHostForElement(selectionElement);
  if (fromSelectionEditable) {
    return fromSelectionEditable;
  }
  return selectAllRootForElement(selectionElement);
}

export function selectAllInFocusedEditor(document: Document): boolean {
  const activeElement = document.activeElement;
  if (activeElement instanceof HTMLTextAreaElement) {
    activeElement.select();
    return true;
  }
  if (
    activeElement instanceof HTMLInputElement &&
    textInputTypes.has(activeElement.type)
  ) {
    activeElement.select();
    return true;
  }

  const host = resolveSelectAllHost(document);
  if (!host) {
    clearDocumentSelection(document);
    return false;
  }

  if (host instanceof HTMLTextAreaElement || host instanceof HTMLInputElement) {
    host.focus();
    host.select();
    return true;
  }

  if (host.isContentEditable || host.getAttribute("contenteditable")) {
    host.focus({ preventScroll: true });
  }
  return selectNodeContents(document, host);
}
