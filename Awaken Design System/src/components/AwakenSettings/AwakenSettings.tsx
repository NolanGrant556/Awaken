import "./AwakenSettings.css";
import { AwakenIcon } from "../AwakenIcon";
import { type CSSProperties, type ReactNode, type Ref, useMemo } from "react";
import { AwakenSettingsNav, type AwakenSettingsNavItem } from "../AwakenSettingsNav";
import { AwakenSettingsSectionProvider } from "./AwakenSettingsSectionContext";

export type AwakenSettingsPresentation = "screen" | "embedded";
export type AwakenSettingsContentLayout = "standard" | "full-bleed";

export type AwakenSettingsProps = {
  title?: string;
  navItems: AwakenSettingsNavItem[];
  activeSectionId: string;
  sectionTitle: string;
  sectionDescription?: ReactNode;
  message?: ReactNode;
  toast?: ReactNode;
  children: ReactNode;
  presentation?: AwakenSettingsPresentation;
  contentLayout?: AwakenSettingsContentLayout;
  backLabel?: string;
  className?: string;
  style?: CSSProperties;
  rootRef?: Ref<HTMLElement>;
  onBack?: () => void;
  onSelectSection: (id: string) => void;
};

export function AwakenSettings({
  title = "设置",
  navItems,
  activeSectionId,
  sectionTitle,
  sectionDescription,
  message,
  toast,
  children,
  presentation = "screen",
  contentLayout = "standard",
  backLabel = "返回应用",
  className,
  style,
  rootRef,
  onBack,
  onSelectSection,
}: AwakenSettingsProps) {
  const activeSectionConcept = navItems.find(
    (item) => item.id === activeSectionId,
  )?.concept;
  const activeSection = useMemo(
    () => ({ title: sectionTitle, concept: activeSectionConcept }),
    [activeSectionConcept, sectionTitle],
  );

  return (
    <AwakenSettingsSectionProvider value={activeSection}>
      <section
        ref={rootRef}
        className={[
          "aw-settings-shell",
          `aw-settings-shell--${presentation}`,
          `aw-settings-shell--${contentLayout}`,
          className ?? "",
        ].filter(Boolean).join(" ")}
        style={style}
        role={presentation === "screen" ? "dialog" : undefined}
        aria-modal={presentation === "screen" ? true : undefined}
        aria-label={`${title}工作区`}
        tabIndex={-1}
      >
        {toast ? (
          <div
            className="aw-settings-toast"
            role="status"
            aria-live="polite"
            aria-atomic="true"
          >
            {toast}
          </div>
        ) : null}
        <aside className="aw-settings-sidebar">
          <div className="aw-settings-sidebar__head">
            {onBack ? (
              <button type="button" className="aw-settings-back" onClick={onBack}>
                <AwakenIcon concept="navigate-back" size={16} />
                <span>{backLabel}</span>
              </button>
            ) : null}
            <div className="aw-settings-sidebar__title">{title}</div>
          </div>
          <AwakenSettingsNav
            items={navItems}
            activeId={activeSectionId}
            onSelect={onSelectSection}
          />
        </aside>

        <main
          className={[
            "aw-settings-main",
            contentLayout === "full-bleed" ? "aw-settings-main--full-bleed" : "",
          ].filter(Boolean).join(" ")}
          aria-label={`${sectionTitle}设置`}
        >
          {contentLayout === "full-bleed" ? children : (
            <div className="aw-settings-scroll">
              <div className="aw-settings-page">
                <header className="aw-settings-page__header">
                  <h1 className="aw-settings-page__title">
                    {activeSectionConcept ? (
                      <AwakenIcon
                        concept={activeSectionConcept}
                        size={24}
                        className="aw-settings-page__title-icon"
                      />
                    ) : null}
                    <span>{sectionTitle}</span>
                  </h1>
                  {sectionDescription ? (
                    <p className="aw-settings-page__description">{sectionDescription}</p>
                  ) : null}
                </header>
                {message ? <div className="model-settings-message">{message}</div> : null}
                {children}
              </div>
            </div>
          )}
        </main>
      </section>
    </AwakenSettingsSectionProvider>
  );
}
