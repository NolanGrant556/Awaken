/** @vitest-environment jsdom */
import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AwakenNewTaskButton, AwakenPrimaryNav, AwakenSubagentExecutionCard } from "@awaken/design-system";
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
  it("renders subagent identity and metrics in its header without a follow-up editor", () => {
    const host = renderFixture(createElement(AwakenSubagentExecutionCard, {
      title: "检查测试", agentLabel: "Reviewer", status: "completed",
      metrics: ["3 次工具"], activity: "核对完成", onClose: () => { },
    }));
    const header = host.querySelector("header");
    expect(header?.textContent).toContain("Reviewer：检查测试");
    expect(header?.textContent).toContain("3 次工具 · 已完成");
    expect(host.textContent).toContain("核对完成");
    expect(host.querySelector("input, textarea, [contenteditable]")).toBeNull();
  });
});
