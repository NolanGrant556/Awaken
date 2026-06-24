import { useCallback, useEffect, useRef, type RefCallback, type UIEventHandler } from "react";

type UseAutoScrollTailOptions = {
  dependencies?: readonly unknown[];
  resetKeys?: readonly unknown[];
  bottomOffset?: number;
  enabled?: boolean;
};

export function useAwakenAutoScrollTail<T extends HTMLElement = HTMLDivElement>({
  dependencies = [],
  resetKeys = [],
  bottomOffset = 80,
  enabled = true,
}: UseAutoScrollTailOptions = {}) {
  const elementRef = useRef<T | null>(null);
  const enabledRef = useRef(enabled);
  const shouldFollowTailRef = useRef(true);
  enabledRef.current = enabled;

  const scrollToTail = useCallback(() => {
    const element = elementRef.current;
    if (!element || !enabledRef.current) {
      return;
    }
    window.requestAnimationFrame(() => {
      const latestElement = elementRef.current;
      if (!latestElement || !enabledRef.current) {
        return;
      }
      latestElement.scrollTop = latestElement.scrollHeight;
    });
  }, []);

  const scrollRef = useCallback<RefCallback<T>>(
    (element) => {
      elementRef.current = element;
      if (element && shouldFollowTailRef.current) {
        scrollToTail();
      }
    },
    [scrollToTail],
  );

  const onScroll = useCallback<UIEventHandler<T>>(
    (event) => {
      const element = event.currentTarget;
      const distanceFromBottom =
        element.scrollHeight - element.scrollTop - element.clientHeight;
      shouldFollowTailRef.current = distanceFromBottom < bottomOffset;
    },
    [bottomOffset],
  );

  useEffect(() => {
    if (shouldFollowTailRef.current) {
      scrollToTail();
    }
  }, [scrollToTail, ...dependencies]);

  useEffect(() => {
    shouldFollowTailRef.current = true;
    scrollToTail();
  }, [scrollToTail, ...resetKeys]);

  return {
    scrollRef,
    onScroll,
    scrollToTail,
    getElement: () => elementRef.current,
    isFollowingTail: () => shouldFollowTailRef.current,
  };
}
