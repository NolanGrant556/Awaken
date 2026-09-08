import { useEffect, useRef, type ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";

export function PreviewToolsMenu({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const dismiss = (event: PointerEvent) => {
      if (
        ref.current?.open &&
        event.target instanceof Node &&
        !ref.current.contains(event.target)
      )
        ref.current.open = false;
    };
    document.addEventListener("pointerdown", dismiss, true);
    return () => document.removeEventListener("pointerdown", dismiss, true);
  }, []);
  return (
    <details
      ref={ref}
      className="project-file-preview-tools-menu"
      onKeyDown={(event) => {
        if (event.key === "Escape" && ref.current?.open) {
          ref.current.open = false;
          ref.current.querySelector("summary")?.focus();
          event.stopPropagation();
        }
      }}
    >
      <summary aria-label="更多 PDF 查看操作" title="缩放与旋转">
        <AwakenIcon concept="more-actions" size={16} />
      </summary>
      <div
        className="project-file-preview-tools-menu__content"
        onClick={(event) => {
          if (
            event.target instanceof Element &&
            event.target.closest("button") &&
            ref.current
          )
            ref.current.open = false;
        }}
      >
        {children}
      </div>
    </details>
  );
}
