const textInputTypes = new Set([
  "email",
  "password",
  "search",
  "tel",
  "text",
  "url",
]);

function editableHostForElement(element: Element | null): HTMLElement | null {
  let current = element;
  while (current) {
    const contentEditable = current.getAttribute("contenteditable");
    if (contentEditable === "false") {
      return null;
    }
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

function clearDocumentSelection(document: Document): void {
  document.getSelection()?.removeAllRanges();
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

  const editableHost = editableHostForElement(activeElement);
  if (!editableHost) {
    clearDocumentSelection(document);
    return false;
  }

  const selection = document.getSelection();
  if (!selection) {
    return false;
  }
  const range = document.createRange();
  range.selectNodeContents(editableHost);
  selection.removeAllRanges();
  selection.addRange(range);
  return true;
}
