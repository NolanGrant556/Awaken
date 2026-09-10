/** @vitest-environment jsdom */
import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AwakenNewTaskButton, AwakenPrimaryNav } from "@awaken/design-system";
function renderFixture(element: ReactElement): HTMLElement {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(element);
  return host;
}
describe("workbench component contracts", () => {
  it("uses the same medium weight for new-task and navigation buttons", () => {
    for (const element of [createElement(AwakenNewTaskButton),
    createElement(AwakenPrimaryNav, { items: [{ id: "tasks", label: "任务" }] })]) {
      expect(renderFixture(element).querySelector("button")?.style.fontWeight).toBe("var(--aw-font-weight-medium)");
    }
  });
});
