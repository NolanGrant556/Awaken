import { autoUpdate, flip, offset, shift, size, useFloating } from "@floating-ui/react";

/** Stay inside the Composer's clipping ancestors, including a scrolling dialog. */
export function AwakenComposerSlashPreview({
  anchor,
  description,
  path,
}: {
  anchor: HTMLElement;
  description?: string;
  path?: string;
}) {
  const { refs, floatingStyles, isPositioned } = useFloating({
    open: true,
    elements: { reference: anchor },
    placement: "right-start",
    whileElementsMounted: autoUpdate,
    middleware: [
      offset(6),
      flip({ padding: 8 }),
      shift({ padding: 8, crossAxis: true }),
      size({
        padding: 8,
        apply({ availableWidth, availableHeight, elements }) {
          elements.floating.style.maxWidth = `${Math.max(0, Math.min(220, availableWidth))}px`;
          elements.floating.style.maxHeight = `${Math.max(0, Math.min(150, availableHeight))}px`;
        },
      }),
    ],
  });

  return (
    <div
      ref={refs.setFloating}
      className="composer-bar-slash-preview"
      style={{ ...floatingStyles, visibility: isPositioned ? undefined : "hidden" }}
      aria-hidden="true"
    >
      {description ? <span className="composer-bar-slash-preview-description">{description}</span> : null}
      {path ? <span className="composer-bar-slash-preview-path">{path}</span> : null}
    </div>
  );
}
