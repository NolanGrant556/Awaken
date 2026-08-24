import React from "react";
import { renderToString } from "react-dom/server";
import {
  AwakenMemoryLightFlowLayout,
  AwakenSettings,
  AwakenSettingsNavItems,
  iconForConcept,
} from "@awaken/design-system";
import { describe, expect, it, vi } from "vitest";

describe("AwakenSettings section heading icons", () => {
  it("uses each active navigation concept in the matching standard page title", () => {
    for (const item of AwakenSettingsNavItems) {
      expect(item.concept, item.id).toBeDefined();
      if (!item.concept) continue;

      const iconName = iconForConcept(item.concept);
      const html = renderToString(
        <AwakenSettings
          presentation="embedded"
          navItems={AwakenSettingsNavItems}
          activeSectionId={item.id}
          sectionTitle={item.label}
          onSelectSection={vi.fn()}
        >
          <div>设置内容</div>
        </AwakenSettings>,
      );

      expect(html.match(new RegExp(`aw-icon--${iconName}`, "g"))?.length, item.id).toBe(2);
      expect(html).toContain(
        `<h1 class="aw-settings-page__title"><svg width="24" height="24"`,
      );
      expect(html).toContain(`<span>${item.label}</span></h1>`);
    }
  });

  it("passes the active navigation concept into the full-bleed memory title", () => {
    const html = renderToString(
      <AwakenSettings
        presentation="embedded"
        contentLayout="full-bleed"
        navItems={AwakenSettingsNavItems}
        activeSectionId="memory"
        sectionTitle="记忆"
        onSelectSection={vi.fn()}
      >
        <AwakenMemoryLightFlowLayout titleAs="h1">
          <div>记忆内容</div>
        </AwakenMemoryLightFlowLayout>
      </AwakenSettings>,
    );

    expect(html.match(/aw-icon--brain/g)?.length).toBe(2);
    expect(html).toContain("aw-memory-light-flow-layout__title-icon");
    expect(html).toContain("<span>记忆</span></h1>");
  });
});
