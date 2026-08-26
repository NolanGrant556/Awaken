import {
  useCallback,
  useLayoutEffect,
  useState,
  type TransitionEventHandler,
} from "react";

const DISCLOSURE_CLOSE_FALLBACK_BUFFER_MS = 120;

function prefersReducedMotion(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Keeps disclosure content mounted only while it is visible or completing its
 * close animation. Collapsed historical traces therefore keep their full data
 * in React props without forcing hidden text through DOM layout and paint.
 */
export function useAwakenDisclosurePresence(
  open: boolean,
  closeDelayMs: number,
): {
  present: boolean;
  prepareOpen: () => void;
  onTransitionEnd: TransitionEventHandler<HTMLElement>;
} {
  const [present, setPresent] = useState(open);

  useLayoutEffect(() => {
    if (open) {
      setPresent(true);
      return;
    }
    if (!present) return;
    if (prefersReducedMotion()) {
      setPresent(false);
      return;
    }
    const timer = window.setTimeout(
      () => setPresent(false),
      closeDelayMs + DISCLOSURE_CLOSE_FALLBACK_BUFFER_MS,
    );
    return () => window.clearTimeout(timer);
  }, [closeDelayMs, open, present]);

  const prepareOpen = useCallback(() => setPresent(true), []);
  const onTransitionEnd = useCallback<TransitionEventHandler<HTMLElement>>(
    (event) => {
      if (
        open ||
        event.currentTarget !== event.target ||
        event.propertyName !== "grid-template-rows"
      ) {
        return;
      }
      setPresent(false);
    },
    [open],
  );
  return { present, prepareOpen, onTransitionEnd };
}
