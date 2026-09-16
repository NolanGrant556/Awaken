import { useRef, type PointerEvent as ReactPointerEvent } from "react";

const interactiveTargets = "button, a, input, textarea, select, label, video, [role='button'], [role='treeitem'], [role='separator'], [role='menu'], [role='dialog'], [contenteditable='true'], [contenteditable='plaintext-only'], .cm-editor";
const clickDistance = 6;

/** Observe completed clicks without consuming the original interaction. Content can opt in, including editors. */
export function useBackgroundPrimaryClick(onClick?: () => void, contentSelector?: string) {
  const gesture = useRef<{ pointerId: number; x: number; y: number; moved: boolean } | undefined>(undefined);

  function acceptsClick(event: ReactPointerEvent<HTMLElement>): boolean {
    if (!(event.target instanceof Element) || !event.currentTarget.contains(event.target)) return false;
    const content = contentSelector ? event.target.closest(contentSelector) : null;
    if (content && event.currentTarget.contains(content)) return true;
    return !event.defaultPrevented && !event.target.closest(interactiveTargets);
  }

  return {
    onPointerDownCapture(event: ReactPointerEvent<HTMLElement>) {
      gesture.current = undefined;
      if (event.button !== 0 || !onClick || !acceptsClick(event)) return;
      gesture.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, moved: false };
    },
    onPointerMoveCapture(event: ReactPointerEvent<HTMLElement>) {
      const current = gesture.current;
      if (!current || current.pointerId !== event.pointerId || current.moved) return;
      current.moved = (event.clientX - current.x) ** 2 + (event.clientY - current.y) ** 2 > clickDistance ** 2;
    },
    onPointerUpCapture(event: ReactPointerEvent<HTMLElement>) {
      const current = gesture.current;
      if (!current || current.pointerId !== event.pointerId) return;
      gesture.current = undefined;
      if (event.button === 0 && !current.moved && acceptsClick(event)) onClick?.();
    },
    onPointerCancelCapture(event: ReactPointerEvent<HTMLElement>) {
      if (gesture.current?.pointerId === event.pointerId) gesture.current = undefined;
    },
  };
}
