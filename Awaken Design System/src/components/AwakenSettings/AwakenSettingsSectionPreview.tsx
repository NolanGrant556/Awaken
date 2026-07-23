import type { ReactNode } from "react";
import { AwakenSettingsNavItems } from "../AwakenSettingsNav";
import { AwakenSettings } from "./AwakenSettings";
import "./AwakenSettingsSectionPreview.css";

const routeBySection: Record<string, string> = {
  models: "awaken-model-settings",
  search: "awaken-search-settings",
  memory: "awaken-memory-settings",
  appearance: "awaken-appearance-settings",
  pets: "awaken-pets-settings",
};

export function AwakenSettingsSectionPreview({
  activeSectionId,
  sectionTitle,
  fullBleed = false,
  children,
}: {
  activeSectionId: string;
  sectionTitle: string;
  fullBleed?: boolean;
  children: ReactNode;
}) {
  function navigate(sectionId: string): void {
    const route = routeBySection[sectionId];
    if (route) window.location.hash = `/components/${route}`;
  }

  return (
    <div className="aw-settings-section-preview">
      <AwakenSettings
        presentation="embedded"
        contentLayout={fullBleed ? "full-bleed" : "standard"}
        navItems={AwakenSettingsNavItems}
        activeSectionId={activeSectionId}
        sectionTitle={sectionTitle}
        onBack={() => { window.location.hash = "/components/awaken-settings"; }}
        onSelectSection={navigate}
      >
        {children}
      </AwakenSettings>
    </div>
  );
}
