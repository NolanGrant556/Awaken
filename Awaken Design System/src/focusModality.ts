export type AwakenFocusModality = "keyboard" | "pointer";

const FOCUS_NAVIGATION_KEYS = new Set([
  "Tab",
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
  "PageUp",
  "PageDown",
  "Enter",
  " ",
  "Spacebar",
  "Escape",
]);

const NON_TEXT_INPUT_TYPES = new Set([
  "button",
  "checkbox",
  "color",
  "file",
  "hidden",
  "image",
  "radio",
  "range",
  "reset",
  "submit",
]);

export const AWAKEN_FOCUS_MODALITY_ATTRIBUTE = "data-aw-focus-modality";

function isTextEntryTarget(
  target: EventTarget | null,
  ownerDocument: Document,
): boolean {
  const ElementConstructor = ownerDocument.defaultView?.Element;
  if (!ElementConstructor || !(target instanceof ElementConstructor)) {
    return false;
  }

  const element = target as Element;
  if (
    element.closest(
      'textarea, [contenteditable]:not([contenteditable="false"])',
    )
  ) {
    return true;
  }

  const input = element.closest("input");
  return Boolean(
    input &&
      !NON_TEXT_INPUT_TYPES.has(input.getAttribute("type") ?? "text"),
  );
}

export function installAwakenFocusModality(root: HTMLElement): () => void {
  const ownerDocument = root.ownerDocument;

  const setModality = (modality: AwakenFocusModality) => {
    root.setAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE, modality);
  };
  const handlePointerDown = () => setModality("pointer");
  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    if (!FOCUS_NAVIGATION_KEYS.has(event.key)) return;
    if (
      event.key !== "Tab" &&
      event.key !== "Escape" &&
      isTextEntryTarget(event.target, ownerDocument)
    ) {
      return;
    }
    setModality("keyboard");
  };

  setModality("pointer");
  ownerDocument.addEventListener("pointerdown", handlePointerDown, true);
  ownerDocument.addEventListener("keydown", handleKeyDown, true);

  return () => {
    ownerDocument.removeEventListener("pointerdown", handlePointerDown, true);
    ownerDocument.removeEventListener("keydown", handleKeyDown, true);
    root.removeAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE);
  };
}
