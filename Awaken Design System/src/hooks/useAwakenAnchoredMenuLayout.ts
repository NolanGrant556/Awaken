import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react";

export type AwakenAnchoredMenuPlacement = "bottom" | "top";

export type AwakenAnchoredMenuLayout = {
  placement: AwakenAnchoredMenuPlacement;
  maxHeight: number;
};

type UseAwakenAnchoredMenuLayoutOptions = {
  open: boolean;
  estimatedHeight: number;
  maxHeight: number;
  preferredPlacement?: AwakenAnchoredMenuPlacement;
  gap?: number;
  viewportPadding?: number;
};

type UseAwakenAnchoredMenuLayoutResult<
  TAnchor extends HTMLElement,
  TMenu extends HTMLElement,
> = AwakenAnchoredMenuLayout & {
  anchorRef: RefObject<TAnchor | null>;
  menuRef: RefObject<TMenu | null>;
  updateLayout: (menuHeight?: number) => void;
};

export function resolveAwakenAnchoredMenuLayout(
  anchorRect: Pick<DOMRect, "top" | "bottom">,
  menuHeight: number,
  viewportHeight: number,
  options: {
    maxHeight: number;
    preferredPlacement: AwakenAnchoredMenuPlacement;
    gap: number;
    viewportPadding: number;
  },
): AwakenAnchoredMenuLayout {
  const availableBottom = Math.max(
    0,
    viewportHeight - anchorRect.bottom - options.viewportPadding,
  );
  const availableTop = Math.max(0, anchorRect.top - options.viewportPadding);
  const availableByPlacement = {
    bottom: availableBottom,
    top: availableTop,
  } satisfies Record<AwakenAnchoredMenuPlacement, number>;
  const oppositePlacement =
    options.preferredPlacement === "bottom" ? "top" : "bottom";
  const preferredAvailable = availableByPlacement[options.preferredPlacement];
  const oppositeAvailable = availableByPlacement[oppositePlacement];
  const requiredSpace = Math.min(options.maxHeight, menuHeight) + options.gap;
  const placement =
    preferredAvailable < requiredSpace && oppositeAvailable > preferredAvailable
      ? oppositePlacement
      : options.preferredPlacement;

  return {
    placement,
    maxHeight: Math.max(
      0,
      Math.min(options.maxHeight, availableByPlacement[placement] - options.gap),
    ),
  };
}

/**
 * Keeps a same-layer anchored menu inside the current viewport. The preferred
 * side is retained while it fits; otherwise the menu flips to the roomier side
 * and becomes internally scrollable through the returned maxHeight.
 */
export function useAwakenAnchoredMenuLayout<
  TAnchor extends HTMLElement,
  TMenu extends HTMLElement,
>({
  open,
  estimatedHeight,
  maxHeight,
  preferredPlacement = "bottom",
  gap = 6,
  viewportPadding = 8,
}: UseAwakenAnchoredMenuLayoutOptions): UseAwakenAnchoredMenuLayoutResult<
  TAnchor,
  TMenu
> {
  const anchorRef = useRef<TAnchor>(null);
  const menuRef = useRef<TMenu>(null);
  const [layout, setLayout] = useState<AwakenAnchoredMenuLayout>({
    placement: preferredPlacement,
    maxHeight,
  });

  const updateLayout = useCallback(
    (menuHeight = estimatedHeight) => {
      const anchorRect = anchorRef.current?.getBoundingClientRect();
      if (!anchorRect) return;
      const nextLayout = resolveAwakenAnchoredMenuLayout(
        anchorRect,
        menuHeight,
        window.innerHeight,
        {
          maxHeight,
          preferredPlacement,
          gap,
          viewportPadding,
        },
      );
      setLayout((current) =>
        current.placement === nextLayout.placement &&
        current.maxHeight === nextLayout.maxHeight
          ? current
          : nextLayout,
      );
    },
    [estimatedHeight, gap, maxHeight, preferredPlacement, viewportPadding],
  );

  useLayoutEffect(() => {
    if (!open) return;

    const updateFromLayout = (): void => {
      const measuredHeight = menuRef.current?.scrollHeight ?? 0;
      updateLayout(measuredHeight > 0 ? measuredHeight : estimatedHeight);
    };

    updateFromLayout();
    window.addEventListener("resize", updateFromLayout);
    window.addEventListener("scroll", updateFromLayout, true);
    const resizeObserver =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(updateFromLayout);
    if (menuRef.current) resizeObserver?.observe(menuRef.current);

    return () => {
      window.removeEventListener("resize", updateFromLayout);
      window.removeEventListener("scroll", updateFromLayout, true);
      resizeObserver?.disconnect();
    };
  }, [estimatedHeight, open, updateLayout]);

  return {
    anchorRef,
    menuRef,
    placement: layout.placement,
    maxHeight: layout.maxHeight,
    updateLayout,
  };
}
