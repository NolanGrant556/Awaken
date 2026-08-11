// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  AwakenDisclosure,
  AwakenMediaGenerationCard,
  AwakenThoughtTrace,
  AwakenToolTraceGroup,
  AwakenToolTraceLine,
} from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("execution trace disclosures", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  it("keeps thinking open while active, collapses it when done, and honors a manual reopen", async () => {
    await act(async () => {
      root.render(<AwakenThoughtTrace text="Inspecting" status="active" />);
    });
    const heading = container.querySelector<HTMLButtonElement>(
      ".thought-trace-heading",
    );
    expect(heading?.getAttribute("aria-expanded")).toBe("true");

    await act(async () => {
      root.render(
        <AwakenThoughtTrace text="Inspection complete" status="done" />,
      );
    });
    expect(heading?.getAttribute("aria-expanded")).toBe("false");
    expect(heading?.textContent).toContain("Thought");

    await act(async () => heading?.click());
    expect(heading?.getAttribute("aria-expanded")).toBe("true");
  });

  it("does not hide a completed Thought label or reserve fading body height", () => {
    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );
    const disclosureBlock = componentStyles.match(
      /\.thought-trace-disclosure\s*\{([\s\S]*?)\n\}/,
    )?.[1];
    const bodyBlock = componentStyles.match(
      /\.thought-trace-body\s*\{([\s\S]*?)\n\}/,
    )?.[1];

    expect(disclosureBlock).toBeDefined();
    expect(disclosureBlock).not.toMatch(/animation\s*:/);
    expect(bodyBlock).toMatch(/grid-template-rows\s*:/);
    expect(bodyBlock).not.toMatch(/opacity\s*:/);
  });

  it("keeps a terminal row compact until the user opens its nested card", async () => {
    await act(async () => {
      root.render(
        <AwakenToolTraceLine
          title="Run tests"
          detail="npm test"
          output="running suite"
          outputLabel="终端输出"
          kind="terminal"
          status="active"
        />,
      );
    });
    const row = container.querySelector<HTMLButtonElement>(
      "button.tool-trace-line",
    );
    expect(row?.getAttribute("aria-expanded")).toBe("false");
    expect(
      container.querySelector(".tool-trace-output-card")?.textContent,
    ).toContain("running suite");

    await act(async () => {
      root.render(
        <AwakenToolTraceLine
          title="Run tests"
          detail="npm test"
          output="4 tests passed"
          outputLabel="终端输出"
          kind="terminal"
          status="done"
        />,
      );
    });
    expect(row?.getAttribute("aria-expanded")).toBe("false");

    await act(async () => row?.click());
    expect(row?.getAttribute("aria-expanded")).toBe("true");
  });

  it("renders structured selectable detail sections and invokes the copy callback", async () => {
    const onCopyDetail = vi.fn();
    await act(async () => {
      root.render(
        <AwakenToolTraceLine
          title="运行测试"
          status="done"
          defaultOpen
          detailCard={{
            title: "Shell 详情",
            copyText: "$ npm test\n4 tests passed",
            sections: [
              { label: "命令", content: "npm test", format: "code" },
              {
                label: "输出",
                content: "4 tests passed",
                format: "text",
                tone: "success",
              },
            ],
          }}
          onCopyDetail={onCopyDetail}
        />,
      );
    });

    expect(
      container.querySelectorAll(".tool-trace-detail-section"),
    ).toHaveLength(2);
    expect(
      container.querySelector('[data-format="code"]')?.textContent,
    ).toContain("npm test");
    expect(
      container.querySelector(".tool-trace-output-card")?.textContent,
    ).not.toContain("Shell 详情");
    expect(container.querySelector(".tool-trace-output-heading")).toBeNull();
    expect(
      container
        .querySelector(".tool-trace-detail-sections")
        ?.hasAttribute("data-aw-run-stage-scroll-boundary"),
    ).toBe(true);
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>(".tool-trace-output-copy")
        ?.click(),
    );
    expect(onCopyDetail).toHaveBeenCalledWith("$ npm test\n4 tests passed");
  });

  it("bounds long detail cards and scrolls their sections inside the card", () => {
    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );

    expect(componentStyles).toMatch(
      /\.tool-trace-output-card\s*\{[\s\S]*?max-height:\s*320px;[\s\S]*?overflow:\s*hidden;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-output-card\s*\{[\s\S]*?background:\s*var\(--aw-surface-code\);/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-detail-section\[data-tone="command"\]\s*pre\s*\{[\s\S]*?color:\s*var\(--aw-text-blue\);/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-detail-sections\s*\{[\s\S]*?max-height:\s*320px;[\s\S]*?overflow-y:\s*auto;[\s\S]*?overscroll-behavior:\s*contain;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-output-scroll::-webkit-scrollbar\s*\{/,
    );
  });

  it("supports controlled group state while preserving active automatic behavior when uncontrolled", async () => {
    const onOpenChange = vi.fn();
    await act(async () => {
      root.render(
        <AwakenToolTraceGroup
          actions={[{ id: "read", title: "正在读取", status: "active" }]}
          label="正在检查 1 项"
          detail="执行中"
          icon={<span data-testid="search-group-icon">搜索</span>}
          open={false}
          onOpenChange={onOpenChange}
        />,
      );
    });
    const heading = container.querySelector<HTMLButtonElement>(
      ".tool-trace-group-heading",
    );
    expect(heading?.getAttribute("aria-expanded")).toBe("false");
    expect(
      heading?.querySelector('[data-testid="search-group-icon"]'),
    ).not.toBeNull();
    await act(async () => heading?.click());
    expect(onOpenChange).toHaveBeenCalledWith(true);
  });

  it("collapses a finished tool group and ready media while preserving their summary rows", async () => {
    await act(async () => {
      root.render(
        <>
          <AwakenToolTraceGroup
            actions={[{ id: "read", title: "Read files", status: "active" }]}
          />
          <AwakenMediaGenerationCard
            kind="image"
            status="generating"
            requestedCount={2}
          />
        </>,
      );
    });
    const group = container.querySelector<HTMLButtonElement>(
      ".tool-trace-group-heading",
    );
    const media = container.querySelector<HTMLButtonElement>(
      ".aw-media-generation-summary",
    );
    expect(group?.getAttribute("aria-expanded")).toBe("true");
    expect(group?.querySelector(".tool-trace-group-detail")).toBeNull();
    expect(media?.getAttribute("aria-expanded")).toBe("true");

    await act(async () => {
      root.render(
        <>
          <AwakenToolTraceGroup
            actions={[{ id: "read", title: "Read files", status: "done" }]}
          />
          <AwakenMediaGenerationCard
            kind="image"
            status="ready"
            src="data:image/gif;base64,R0lGODlhAQABAAAAACw="
          />
        </>,
      );
    });
    expect(group?.getAttribute("aria-expanded")).toBe("false");
    expect(media?.getAttribute("aria-expanded")).toBe("false");
    expect(group?.textContent).toContain("1 个工具调用");
    expect(group?.textContent).not.toContain("已完成");
    expect(media?.textContent).toContain("已生成 1 张图片");
  });

  it("shows historical tool rows immediately while keeping live-entry animation opt-in by default", async () => {
    await act(async () => {
      root.render(
        <>
          <AwakenToolTraceLine title="Live tool" status="active" />
          <AwakenToolTraceLine
            title="Historical tool"
            status="done"
            animateEntry={false}
          />
          <AwakenToolTraceGroup
            actions={[
              { id: "read", title: "Read files", status: "done" },
              { id: "search", title: "Search files", status: "done" },
            ]}
            defaultOpen
            animateItems={false}
          />
        </>,
      );
    });

    const items = Array.from(
      container.querySelectorAll<HTMLElement>(".tool-trace-item"),
    );
    expect(items.map((item) => item.dataset.animateEntry)).toEqual([
      "true",
      "false",
      "false",
      "false",
    ]);

    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-item\[data-animate-entry="false"\]\s*\{[\s\S]*?animation:\s*none;/,
    );
  });

  it("shows historical thoughts immediately while preserving live thought entry animation", async () => {
    await act(async () => {
      root.render(
        <>
          <AwakenThoughtTrace text="Live reasoning" status="done" />
          <AwakenThoughtTrace
            text="Historical reasoning"
            status="done"
            animateEntry={false}
          />
        </>,
      );
    });

    const thoughts = Array.from(
      container.querySelectorAll<HTMLElement>(".thought-trace"),
    );
    expect(thoughts.map((item) => item.dataset.animateEntry)).toEqual([
      "true",
      "false",
    ]);

    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );
    expect(componentStyles).toMatch(
      /\.thought-trace\[data-animate-entry="false"\] \.thought-trace-disclosure,[\s\S]*?animation:\s*none;/,
    );
  });

  it("keeps disclosure labels first and chevrons at the compact trigger's right edge", async () => {
    await act(async () => {
      root.render(
        <>
          <AwakenDisclosure title="执行过程">过程内容</AwakenDisclosure>
          <AwakenThoughtTrace text="Reasoning" status="done" />
          <AwakenToolTraceGroup
            actions={[{ id: "read", title: "Read files", status: "done" }]}
          />
          <AwakenMediaGenerationCard
            kind="image"
            status="ready"
            src="data:image/gif;base64,R0lGODlhAQABAAAAACw="
          />
        </>,
      );
    });

    const disclosure = container.querySelector<HTMLButtonElement>(
      ".aw-disclosure-trigger",
    );
    const thought = container.querySelector<HTMLButtonElement>(
      ".thought-trace-heading",
    );
    const group = container.querySelector<HTMLButtonElement>(
      ".tool-trace-group-heading",
    );
    const media = container.querySelector<HTMLButtonElement>(
      ".aw-media-generation-summary",
    );

    expect(
      disclosure?.firstElementChild?.classList.contains("aw-disclosure-title"),
    ).toBe(true);
    expect(
      disclosure?.lastElementChild?.classList.contains("aw-disclosure-chevron"),
    ).toBe(true);
    expect(
      thought?.lastElementChild?.classList.contains(
        "thought-trace-disclosure-icon",
      ),
    ).toBe(true);
    expect(
      group?.lastElementChild?.classList.contains("tool-trace-group-chevron"),
    ).toBe(true);
    expect(
      media?.lastElementChild?.classList.contains(
        "aw-media-generation-chevron",
      ),
    ).toBe(true);
  });

  it("keeps compact triggers content-sized and expanded bodies at the original left edge", () => {
    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );
    const mediaStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenMediaGenerationCard/AwakenMediaGenerationCard.css",
      ),
      "utf8",
    );
    const disclosureStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenDisclosure/AwakenDisclosure.css",
      ),
      "utf8",
    );

    expect(componentStyles).toMatch(
      /\.tool-trace-line\s*\{[\s\S]*?width:\s*fit-content;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-group-lines\s*\{[\s\S]*?padding-left:\s*0;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-output-rail\s*\{[\s\S]*?padding:\s*2px 0 7px;/,
    );
    expect(componentStyles).not.toContain(".tool-trace-output-rail::before");
    expect(componentStyles).toMatch(
      /\.thought-trace p\s*\{[\s\S]*?padding:\s*5px 0;/,
    );
    expect(mediaStyles).toMatch(
      /\.aw-media-generation-summary\s*\{[\s\S]*?width:\s*fit-content;/,
    );
    expect(mediaStyles).toMatch(
      /\.aw-media-generation-grid\s*\{[\s\S]*?padding:\s*var\(--aw-space-2\) 0 1px;/,
    );
    expect(disclosureStyles).toMatch(
      /\.aw-disclosure-trigger\s*\{[\s\S]*?width:\s*fit-content;/,
    );
  });

  it("uses text-only hover emphasis without card backgrounds across disclosure triggers", () => {
    const componentStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/tokens/components.css",
      ),
      "utf8",
    );
    const mediaStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenMediaGenerationCard/AwakenMediaGenerationCard.css",
      ),
      "utf8",
    );
    const disclosureStyles = readFileSync(
      resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenDisclosure/AwakenDisclosure.css",
      ),
      "utf8",
    );

    for (const styles of [componentStyles, mediaStyles, disclosureStyles]) {
      expect(styles).not.toContain("background: var(--aw-surface-hover)");
    }
    expect(componentStyles).toMatch(
      /\.thought-trace-heading:hover,[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
    expect(componentStyles).toMatch(
      /:root\[data-aw-focus-modality="keyboard"\] \.thought-trace-heading:focus-visible\s*\{[\s\S]*?outline:\s*none;/,
    );
    expect(componentStyles).toMatch(
      /\.thought-trace-heading:focus-visible[\s\S]*?:is\(\.thought-trace-disclosure-label, \.aw-thinking-orb-status__label\)[\s\S]*?text-decoration-line:\s*underline;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-group-heading:hover,[\s\S]*?background:\s*transparent;[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
    expect(componentStyles).toMatch(
      /button\.tool-trace-line:hover,[\s\S]*?background:\s*transparent;[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-line-chevron\s*\{[\s\S]*?opacity:\s*0;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-line:hover \.tool-trace-line-chevron\[data-visible="available"\],[\s\S]*?\.tool-trace-line:focus-visible[\s\S]*?opacity:\s*1;/,
    );
    expect(componentStyles).toMatch(
      /\.tool-trace-group-active:not\(\.tool-trace-group-open\) \.tool-trace-group-label\s*\{[\s\S]*?animation:\s*tool-trace-shimmer 1\.6s linear infinite;/,
    );
    expect(componentStyles).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.tool-trace-group-active:not\(\.tool-trace-group-open\) \.tool-trace-group-label,[\s\S]*?animation:\s*none;/,
    );
    expect(mediaStyles).toMatch(
      /\.aw-media-generation-summary:hover,[\s\S]*?background:\s*transparent;[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
    expect(disclosureStyles).toMatch(
      /\.aw-disclosure-trigger:hover,[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
  });
});
