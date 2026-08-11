import "./AwakenModelSettings.css";
import { type ReactNode, useCallback, useEffect, useRef } from "react";
import {
  AwakenSettingsTabs,
  type AwakenSettingsTabItem,
} from "../AwakenSettingsTabs";
import { AwakenModelOrbitHero } from "./AwakenModelOrbitHero";

export type AwakenModelSettingsProps = {
  tabs: AwakenSettingsTabItem[];
  activeTabId: string;
  views: AwakenSettingsTabItem[];
  activeViewId: string;
  title: string;
  pageTitle?: string;
  theme?: "light" | "dark";
  description?: ReactNode;
  message?: ReactNode;
  children: ReactNode;
  onSelectTab?: (id: string) => void;
  onSelectView?: (id: string) => void;
};

export function AwakenModelSettings({
  tabs,
  activeTabId,
  views,
  activeViewId,
  title,
  pageTitle = "模型",
  theme = "light",
  description,
  message,
  children,
  onSelectTab,
  onSelectView,
}: AwakenModelSettingsProps) {
  const rootRef = useRef<HTMLElement>(null);
  const scrollFrameRef = useRef<number | null>(null);

  const updateScrollThumb = useCallback(() => {
    const root = rootRef.current;
    if (!root) return;

    const viewportHeight = root.clientHeight;
    const scrollRange = root.scrollHeight - viewportHeight;
    const orbit = root.querySelector<HTMLElement>(".aw-model-orbit");
    const indicatorTop = Math.min(orbit?.clientHeight ?? 0, viewportHeight);
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
    const thumbOffset =
      indicatorTop + inset + (root.scrollTop / scrollRange) * thumbRange;

    root.style.setProperty(
      "--aw-model-scroll-thumb-height",
      `${thumbHeight}px`,
    );
    root.style.setProperty(
      "--aw-model-scroll-thumb-offset",
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
    const content = root.querySelector<HTMLElement>(
      ".aw-model-settings-page__content",
    );
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
      className="aw-model-settings-page"
      aria-label="模型"
      onScroll={scheduleScrollThumbUpdate}
    >
      <AwakenModelOrbitHero theme={theme} title={pageTitle} />
      <div className="aw-model-settings-page__content">
        <div className="aw-model-settings">
          <aside className="aw-model-settings__rail" aria-label="模型接入类型">
            <AwakenSettingsTabs
              items={tabs}
              activeId={activeTabId}
              ariaLabel="模型接入类型"
              onSelect={onSelectTab}
            />
          </aside>
          <div className="aw-model-settings__workspace">
            <header className="aw-model-settings__header">
              <div className="aw-model-settings__heading">
                <h2>{title}</h2>
                {description ? <p>{description}</p> : null}
              </div>
              <AwakenSettingsTabs
                items={views}
                activeId={activeViewId}
                ariaLabel={`${title}设置内容`}
                onSelect={onSelectView}
              />
            </header>
            {message ? <div className="model-settings-message">{message}</div> : null}
            <div className="aw-model-settings__content">{children}</div>
          </div>
        </div>
      </div>
      <span className="aw-model-settings-page__scroll-thumb" aria-hidden="true" />
    </section>
  );
}
