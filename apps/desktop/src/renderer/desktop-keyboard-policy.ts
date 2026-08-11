export function isUnmodifiedTabNavigation(event: KeyboardEvent): boolean {
  return (
    event.key === "Tab" &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.altKey
  );
}

export function installDesktopKeyboardPolicy(
  ownerDocument: Document,
): () => void {
  const handleKeyDown = (event: KeyboardEvent) => {
    if (!isUnmodifiedTabNavigation(event)) {
      return;
    }
    event.preventDefault();
    event.stopImmediatePropagation();
  };

  ownerDocument.addEventListener("keydown", handleKeyDown, true);
  return () => ownerDocument.removeEventListener("keydown", handleKeyDown, true);
}
