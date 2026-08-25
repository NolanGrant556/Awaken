import { useCallback, useLayoutEffect, useState } from "react";

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
} {
  const [present, setPresent] = useState(open);

  useLayoutEffect(() => {
    if (open) {
      setPresent(true);
      return;
    }
    if (!present) return;
    const timer = window.setTimeout(() => setPresent(false), closeDelayMs);
    return () => window.clearTimeout(timer);
  }, [closeDelayMs, open, present]);

  const prepareOpen = useCallback(() => setPresent(true), []);
  return { present, prepareOpen };
}
