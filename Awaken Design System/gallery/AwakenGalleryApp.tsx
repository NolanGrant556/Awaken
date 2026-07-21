import { useEffect, useMemo, useState } from "react";
import {
  awakenGalleryEntries,
  awakenGallerySections,
  type AwakenGalleryCategory,
} from "./registry";

const componentRoutePrefix = "#/components/";

function idFromHash(): string {
  if (typeof window === "undefined") {
    return "";
  }
  const hash = window.location.hash;
  if (!hash.startsWith(componentRoutePrefix)) {
    return "";
  }
  return decodeURIComponent(hash.slice(componentRoutePrefix.length));
}

function routeForComponent(id: string): string {
  return `${componentRoutePrefix}${encodeURIComponent(id)}`;
}

function resolveComponentId(id: string): string {
  const entry = awakenGalleryEntries.find(
    (candidate) => candidate.id === id || candidate.aliases?.includes(id),
  );
  return entry?.id ?? awakenGalleryEntries[0]?.id ?? "";
}

function ensureComponentRoute(id: string): void {
  if (!id || typeof window === "undefined") {
    return;
  }
  const route = routeForComponent(id);
  if (window.location.hash !== route) {
    window.history.replaceState(null, "", route);
  }
}

export function AwakenGalleryApp() {
  const [activeId, setActiveId] = useState(() => resolveComponentId(idFromHash()));
  const [expandedSectionIds, setExpandedSectionIds] = useState<Set<AwakenGalleryCategory>>(
    () => {
      const initialCategory = awakenGalleryEntries.find(
        (entry) => entry.id === resolveComponentId(idFromHash()),
      )?.category;
      return new Set(initialCategory ? [initialCategory] : []);
    },
  );
  const activeEntry = useMemo(
    () => awakenGalleryEntries.find((entry) => entry.id === activeId) ?? awakenGalleryEntries[0],
    [activeId],
  );
  const Demo = activeEntry?.demo;

  useEffect(() => {
    function syncFromHash(): void {
      const resolvedId = resolveComponentId(idFromHash());
      setActiveId(resolvedId);
      ensureComponentRoute(resolvedId);
    }

    syncFromHash();
    window.addEventListener("hashchange", syncFromHash);
    return () => window.removeEventListener("hashchange", syncFromHash);
  }, []);

  useEffect(() => {
    const activeCategory = activeEntry?.category;
    if (!activeCategory) {
      return;
    }
    setExpandedSectionIds((current) => {
      if (current.has(activeCategory)) {
        return current;
      }
      const next = new Set(current);
      next.add(activeCategory);
      return next;
    });
  }, [activeEntry?.category]);

  function selectComponent(id: string): void {
    const route = routeForComponent(id);
    if (window.location.hash === route) {
      setActiveId(id);
      return;
    }
    window.location.hash = route;
  }

  function toggleSection(id: AwakenGalleryCategory): void {
    setExpandedSectionIds((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  return (
    <main className="ads-gallery">
      <aside className="ads-gallery-sidebar">
        <div className="ads-gallery-brand">
          <span className="ads-gallery-brand__title">Awaken 设计系统</span>
          <div className="ads-gallery-brand__catalog">
            <span>组件目录</span>
            <strong>{awakenGalleryEntries.length} 个组件</strong>
          </div>
        </div>
        <nav aria-label="组件目录">
          {awakenGallerySections.map((section) => (
            <div className="ads-gallery-nav-section" key={section.id}>
              <h2>
                <button
                  type="button"
                  className="ads-gallery-nav-section__toggle"
                  aria-expanded={expandedSectionIds.has(section.id)}
                  aria-controls={`ads-gallery-section-${section.id}`}
                  onClick={() => toggleSection(section.id)}
                >
                  <span>{section.label}</span>
                  <span className="ads-gallery-nav-section__count">{section.entries.length}</span>
                  <svg
                    className="ads-gallery-nav-section__chevron"
                    viewBox="0 0 16 16"
                    aria-hidden="true"
                  >
                    <path d="m4 6 4 4 4-4" />
                  </svg>
                </button>
              </h2>
              <div
                id={`ads-gallery-section-${section.id}`}
                className="ads-gallery-nav-section__entries"
                hidden={!expandedSectionIds.has(section.id)}
              >
                {section.entries.map((entry) => (
                  <button
                    key={entry.id}
                    type="button"
                    className={`ads-gallery-entry-button${entry.id === activeId ? " is-active" : ""}`}
                    aria-label={`${entry.displayName} ${entry.name}`}
                    onClick={() => selectComponent(entry.id)}
                  >
                    <span className="ads-gallery-entry-name">{entry.displayName}</span>
                    <span className="ads-gallery-entry-code">{entry.name}</span>
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>
      </aside>
      <section
        className="ads-gallery-stage"
        data-gallery-category={activeEntry?.category}
      >
        {Demo ? <Demo /> : null}
      </section>
    </main>
  );
}
