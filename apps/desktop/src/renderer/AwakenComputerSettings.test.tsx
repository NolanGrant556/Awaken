// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenComputerSettings } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

const settings = {
  enabled: true,
  allow_background_window_actions: false,
};

describe("AwakenComputerSettings runtime status", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  it("places runtime status last and combines the installation dependencies", () => {
    act(() => {
      root.render(
        <AwakenComputerSettings
          settings={settings}
          status={{
            available: true,
            healthy: true,
            version: "0.22.0",
            node_path: "/bundled/node",
            tcc_screen_recording: true,
            tcc_accessibility: true,
          }}
        />,
      );
    });

    expect(
      [...host.querySelectorAll(".computer-settings-card")].map((card) =>
        card.querySelector("h3")?.textContent,
      ),
    ).toEqual(["全局设置", "能力范围与限制", "运行状态"]);
    expect(host.textContent).toContain("Computer Use 安装状态");
    expect(host.textContent).toContain("已安装");
    expect(host.textContent).not.toContain("CUA Host 侧链");
    expect(host.textContent).not.toContain("Node runtime");
    expect(
      host.querySelector(".computer-settings-status-item")?.getAttribute("data-ok"),
    ).toBe("true");
  });

  it("shows a red installation error when either dependency is unavailable", () => {
    act(() => {
      root.render(
        <AwakenComputerSettings
          settings={settings}
          status={{
            available: true,
            healthy: true,
            tcc_screen_recording: true,
            tcc_accessibility: true,
          }}
        />,
      );
    });

    const installation = host.querySelector(
      ".computer-settings-status-item",
    );
    expect(installation?.getAttribute("data-ok")).toBe("false");
    expect(installation?.textContent).toContain("安装异常");
    expect(host.textContent).toContain("安装 Computer Use");
  });
});
