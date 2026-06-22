import { useEffect, useMemo, useState } from "react";
import { awakenGalleryEntries } from "./registry";

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
  if (awakenGalleryEntries.some((entry) => entry.id === id)) {
    return id;
  }
  return awakenGalleryEntries[0]?.id ?? "";
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

  function selectComponent(id: string): void {
    const route = routeForComponent(id);
    if (window.location.hash === route) {
      setActiveId(id);
      return;
    }
    window.location.hash = route;
  }

  return (
    <main className="ads-gallery">
      <aside className="ads-gallery-sidebar">
        <div className="ads-gallery-brand">
          <span>Awaken Design System</span>
          <strong>{awakenGalleryEntries.length} components</strong>
        </div>
        <nav aria-label="Components">
          {awakenGalleryEntries.map((entry) => (
            <button
              key={entry.id}
              type="button"
              className={entry.id === activeId ? "is-active" : ""}
              onClick={() => selectComponent(entry.id)}
            >
              {entry.name}
            </button>
          ))}
        </nav>
      </aside>
      <section className="ads-gallery-stage">
        {Demo ? <Demo /> : null}
      </section>
    </main>
  );
}
