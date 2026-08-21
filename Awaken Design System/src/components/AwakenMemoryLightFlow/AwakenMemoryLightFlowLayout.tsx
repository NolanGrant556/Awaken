import { type ReactNode, useCallback, useEffect, useRef } from "react";
import { Brain } from "../../icons";
import { AwakenMemoryLightFlow } from "./AwakenMemoryLightFlow";
import type { AwakenMemoryLightFlowTheme } from "./AwakenMemoryLightFlow.model";
import "./AwakenMemoryLightFlowLayout.css";

export type AwakenMemoryLightFlowLayoutProps = {
  theme?: AwakenMemoryLightFlowTheme;
  title?: ReactNode;
  titleAs?: "h1" | "h2" | "h3";
  className?: string;
  children: ReactNode;
};

export function AwakenMemoryLightFlowLayout({
  theme = "light",
  title = "记忆",
  titleAs = "h2",
  className,
  children,
}: AwakenMemoryLightFlowLayoutProps) {
  const rootRef = useRef<HTMLElement>(null);
  const scrollFrameRef = useRef<number | null>(null);
  const Heading = titleAs;
  const rootClassName = [
    "aw-memory-light-flow-layout",
    `aw-memory-light-flow-layout--${theme}`,
    className,
  ].filter(Boolean).join(" ");

  const updateScrollThumb = useCallback(() => {
    const root = rootRef.current;
    if (!root) return;

    const viewportHeight = root.clientHeight;
    const scrollRange = root.scrollHeight - viewportHeight;
    const ambient = root.querySelector<HTMLElement>(".aw-memory-light-flow-layout__ambient");
    const indicatorTop = Math.min(ambient?.clientHeight ?? 0, viewportHeight);
    const inset = 4;
    const availableHeight = Math.max(0, viewportHeight - indicatorTop - inset * 2);
    const isScrollable = scrollRange > 1 && availableHeight > 0;

    root.dataset.scrollable = isScrollable ? "true" : "false";
    if (!isScrollable) return;

    const proportionalThumbHeight =
      availableHeight * (viewportHeight / root.scrollHeight);
    const thumbHeight = Math.min(
      availableHeight,
      Math.max(44, Math.min(72, proportionalThumbHeight)),
    );
    const thumbRange = Math.max(0, availableHeight - thumbHeight);
    const thumbOffset = indicatorTop + inset + (root.scrollTop / scrollRange) * thumbRange;

    root.style.setProperty("--aw-memory-scroll-thumb-height", `${thumbHeight}px`);
    root.style.setProperty(
      "--aw-memory-scroll-thumb-offset",
      `${root.scrollTop + thumbOffset}px`,
    );
  }, []);

  const scheduleScrollThumbUpdate = useCallback(() => {
    if (scrollFrameRef.current !== null) return;
    scrollFrameRef.current = window.requestAnimationFrame(() => {
      scrollFrameRef.current = null;
      updateScrollThumb();
    });
  }, [updateScrollThumb]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    updateScrollThumb();
    const observer = typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver(scheduleScrollThumbUpdate);
    observer?.observe(root);
    const content = root.querySelector<HTMLElement>(".aw-memory-light-flow-layout__content");
    if (content) observer?.observe(content);

    return () => {
      observer?.disconnect();
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current);
        scrollFrameRef.current = null;
      }
    };
  }, [scheduleScrollThumbUpdate, updateScrollThumb]);

  return (
    <section
      ref={rootRef}
      className={rootClassName}
      onScroll={scheduleScrollThumbUpdate}
    >
      <div className="aw-memory-light-flow-layout__ambient">
        <AwakenMemoryLightFlow theme={theme} />
        <Heading className="aw-memory-light-flow-layout__title">
          <Brain
            className="aw-memory-light-flow-layout__title-icon"
            size={28}
            strokeWidth={1.8}
            aria-hidden="true"
          />
          <span>{title}</span>
        </Heading>
      </div>
      <div className="aw-memory-light-flow-layout__content">
        {children}
      </div>
      <span className="aw-memory-light-flow-layout__scroll-thumb" aria-hidden="true" />
    </section>
  );
}
