import { useMemo, useState } from "react";
import { awakenGalleryEntries } from "./registry";

export function AwakenGalleryApp() {
  const [activeId, setActiveId] = useState(awakenGalleryEntries[0]?.id ?? "");
  const activeEntry = useMemo(
    () => awakenGalleryEntries.find((entry) => entry.id === activeId) ?? awakenGalleryEntries[0],
    [activeId],
  );
  const Demo = activeEntry?.demo;

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
              onClick={() => setActiveId(entry.id)}
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
