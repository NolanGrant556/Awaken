import { readStyleSource } from "./style-test-source";
// @vitest-environment jsdom
import {
  AwakenDisclosure,
  AwakenMediaGenerationCard,
  AwakenThoughtTrace,
  AwakenToolTraceGroup,
  AwakenToolTraceLine,
} from "@awaken/design-system";
import { resolve } from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SubagentPanel } from "./SubagentPanel";
import { createInitialRuntimeState, reduceRuntimeState } from "./runtime/reducer";

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

  function finishGridDisclosureTransition(element: Element | null): void {
    if (!element) return;
    const event = new Event("transitionend", { bubbles: true });
    Object.defineProperty(event, "propertyName", {
      configurable: true,
      value: "grid-template-rows",
    });
    element.dispatchEvent(event);
  }

  it.each(["completed", "failed", "cancelled", "interrupted"] as const)(
    "removes the child thinking animation on %s while retaining readable reasoning",
    async (status) => {
      const identity = { runId: "run-1", subagentTaskId: "child-1" };
      let state = reduceRuntimeState(createInitialRuntimeState(), { type: "run/event", event: {
        ...identity, type: "REASONING_MESSAGE_CONTENT", messageId: "reasoning-1", delta: "已读取24块，480行",
      } });
      const renderPanel = () => <SubagentPanel task={state.subagentsById["child-1"]}
        nowMs={0} cancelling={false} onCancel={() => {}}
        renderActivity={(_id, items) => items.map(item => item.kind === "thinking"
          ? <AwakenThoughtTrace key={item.id} text={item.text} status={item.status} /> : null)} />;
      await act(async () => root.render(renderPanel()));
      expect(container.querySelector(".thought-trace-active")).not.toBeNull();
      expect(container.textContent).toContain("思考 中");

      state = reduceRuntimeState(state, { type: "run/event", event: {
        ...identity, type: "AWAKEN_SUBAGENT_CHANGED", subagentStatus: status,
        ...(status !== "completed" ? { subagentErrorCode: `subagent_${status}`, subagentErrorMessage: "子任务已终止" } : {}),
      } });
      await act(async () => root.render(renderPanel()));
      expect(container.querySelector(".thought-trace-active")).toBeNull();
      expect(container.querySelector(".aw-trace-shimmer")).toBeNull();
      expect(container.querySelector(".thought-trace-status")).toBeNull();
      expect(container.textContent).not.toContain("思考 中");
      expect(container.textContent).toContain("思考 片刻");
      if (status !== "completed") expect(container.querySelector('[role="alert"]')?.textContent).toContain(`subagent_${status}`);
      await act(async () => container.querySelector<HTMLButtonElement>(".thought-trace-heading")?.click());
      expect(container.querySelector(".thought-trace-body p")?.textContent).toBe("已读取24块，480行");
      // Reopening the panel must not replay the terminal thinking animation.
      await act(async () => root.render(null));
      await act(async () => root.render(renderPanel()));
      expect(container.querySelector(".thought-trace-active")).toBeNull();
      expect(container.querySelector(".aw-trace-shimmer")).toBeNull();
    },
  );

  it("keeps thinking collapsed by default and honors a manual reopen", async () => {
    await act(async () => {
      root.render(<AwakenThoughtTrace text="Inspecting" status="active" />);
    });
    const heading = container.querySelector<HTMLButtonElement>(
      ".thought-trace-heading",
    );
    expect(heading?.getAttribute("aria-expanded")).toBe("false");

    await act(async () => {
      root.render(
        <AwakenThoughtTrace text="Inspection complete" status="done" />,
      );
    });
    expect(heading?.getAttribute("aria-expanded")).toBe("false");
    expect(heading?.textContent).toContain("思考 片刻");

    await act(async () => heading?.click());
    expect(heading?.getAttribute("aria-expanded")).toBe("true");
  });

  it("does not mount completed Thought text until the user opens it", async () => {
    await act(async () => {
      root.render(
        <AwakenThoughtTrace text="Complete historical reasoning" status="done" />,
      );
    });
    const heading = container.querySelector<HTMLButtonElement>(
      ".thought-trace-heading",
    );
    expect(container.querySelector(".thought-trace-body p")).toBeNull();

    await act(async () => heading?.click());

    expect(container.querySelector(".thought-trace-body p")?.textContent).toBe(
      "Complete historical reasoning",
    );
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
    expect(container.querySelector(".tool-trace-output-card")).toBeNull();

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
    expect(container.querySelector(".tool-trace-output-card")).toBeNull();

    await act(async () => row?.click());
    expect(row?.getAttribute("aria-expanded")).toBe("true");
    expect(
      container.querySelector(".tool-trace-output-card")?.textContent,
    ).toContain("4 tests passed");
  });

  it("keeps a large collapsed trace DOM bounded while preserving full on-demand output", async () => {
    const fullOutput = `unique-start-${"x".repeat(20_000)}-unique-end`;
    await act(async () => {
      root.render(
        <>
          {Array.from({ length: 120 }, (_, index) => (
            <AwakenToolTraceLine
              key={index}
              title={`Historical tool ${index + 1}`}
              output={fullOutput}
              status="done"
              animateEntry={false}
            />
          ))}
        </>,
      );
    });

    expect(container.querySelectorAll(".tool-trace-item")).toHaveLength(120);
    expect(container.querySelectorAll(".tool-trace-output-card")).toHaveLength(0);
    expect(container.textContent).not.toContain("unique-end");

    const firstRow = container.querySelector<HTMLButtonElement>(
      "button.tool-trace-line",
    );
    await act(async () => firstRow?.click());

    expect(container.querySelectorAll(".tool-trace-output-card")).toHaveLength(1);
    expect(container.querySelector(".tool-trace-output-card")?.textContent).toBe(
      fullOutput,
    );
  });

  it("keeps detail mounted until the close transition finishes", async () => {
    vi.useFakeTimers();
    try {
      await act(async () => {
        root.render(
          <AwakenToolTraceLine
            title="Historical tool"
            output="complete output"
            status="done"
            defaultOpen
          />,
        );
      });
      const row = container.querySelector<HTMLButtonElement>(
        "button.tool-trace-line",
      );

      await act(async () => row?.click());
      expect(row?.getAttribute("aria-expanded")).toBe("false");
      expect(container.querySelector(".tool-trace-output-card")).not.toBeNull();

      await act(async () => vi.advanceTimersByTime(181));
      expect(container.querySelector(".tool-trace-output-card")).not.toBeNull();

      const opacityEvent = new Event("transitionend", { bubbles: true });
      Object.defineProperty(opacityEvent, "propertyName", {
        configurable: true,
        value: "opacity",
      });
      await act(async () =>
        container
          .querySelector(".tool-trace-output-disclosure")
          ?.dispatchEvent(opacityEvent),
      );
      expect(container.querySelector(".tool-trace-output-card")).not.toBeNull();

      await act(async () =>
        finishGridDisclosureTransition(
          container.querySelector(".tool-trace-output-disclosure"),
        ),
      );
      expect(container.querySelector(".tool-trace-output-card")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("removes closing detail immediately when reduced motion is requested", async () => {
    const originalMatchMedia = window.matchMedia;
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn(() => ({
        matches: true,
        media: "(prefers-reduced-motion: reduce)",
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
    try {
      await act(async () => {
        root.render(
          <AwakenToolTraceLine
            title="Reduced motion tool"
            output="complete output"
            status="done"
            defaultOpen
          />,
        );
      });
      const row = container.querySelector<HTMLButtonElement>(
        "button.tool-trace-line",
      );

      await act(async () => row?.click());

      expect(container.querySelector(".tool-trace-output-card")).toBeNull();
      expect(
        container.querySelector(".tool-trace-item")?.getAttribute(
          "data-disclosure-state",
        ),
      ).toBe("closed");
    } finally {
      Object.defineProperty(window, "matchMedia", {
        configurable: true,
        value: originalMatchMedia,
      });
    }
  });

  it("uses readable status text instead of a permanent icon column", async () => {
    for (const status of ["active", "done", "error"] as const) {
      await act(async () => root.render(<AwakenToolTraceLine title="查询订单" kind="mcp" status={status} />));
      expect(container.querySelector(".tool-trace-line-icon")).toBeNull();
      expect(container.textContent).toContain("查询订单");
      expect(Boolean(container.querySelector(".aw-trace-shimmer"))).toBe(status === "active");
      expect(container.textContent).not.toContain("失败");
    }
  });

  it("renders structured selectable detail sections without a copy chrome row", async () => {
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
    expect(container.querySelector(".tool-trace-output-actions")).toBeNull();
    expect(container.querySelector(".tool-trace-output-copy")).toBeNull();
    expect(
      container
        .querySelector(".tool-trace-detail-sections")
        ?.hasAttribute("data-aw-run-stage-scroll-boundary"),
    ).toBe(true);
  });


  it("supports controlled group state without forcing active groups open", async () => {
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
    ).toBeNull();
    await act(async () => heading?.click());
    expect(onOpenChange).toHaveBeenCalledWith(true);
  });

  it("does not mount collapsed group rows until the group is opened", async () => {
    await act(async () => {
      root.render(
        <AwakenToolTraceGroup
          actions={[
            { id: "read", title: "Read files", status: "done" },
            { id: "search", title: "Search files", status: "done" },
          ]}
        />,
      );
    });
    const heading = container.querySelector<HTMLButtonElement>(
      ".tool-trace-group-heading",
    );
    expect(container.querySelectorAll(".tool-trace-item")).toHaveLength(0);

    await act(async () => heading?.click());

    expect(container.querySelectorAll(".tool-trace-item")).toHaveLength(2);
  });

  it("keeps group rows mounted until the group close transition finishes", async () => {
    vi.useFakeTimers();
    try {
      await act(async () => {
        root.render(
          <AwakenToolTraceGroup
            actions={[
              { id: "read", title: "Read files", status: "done" },
              { id: "search", title: "Search files", status: "done" },
            ]}
            defaultOpen
          />,
        );
      });
      const heading = container.querySelector<HTMLButtonElement>(
        ".tool-trace-group-heading",
      );

      await act(async () => heading?.click());
      await act(async () => vi.advanceTimersByTime(181));
      expect(container.querySelectorAll(".tool-trace-item")).toHaveLength(2);

      await act(async () =>
        finishGridDisclosureTransition(
          container.querySelector(".tool-trace-group-disclosure"),
        ),
      );
      expect(container.querySelectorAll(".tool-trace-item")).toHaveLength(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it("lets Chromium skip only offscreen collapsed trace layout", () => {
    const workbenchStyles = readStyleSource(resolve(process.cwd(), "src/renderer/styles/workbench.css"));

    expect(workbenchStyles).toMatch(
      /\.thought-trace\[data-disclosure-state="closed"\][\s\S]*?content-visibility:\s*auto;/,
    );
    expect(workbenchStyles).not.toMatch(
      /data-disclosure-state="closing"[\s\S]*?content-visibility:\s*auto;/,
    );
    expect(workbenchStyles).toMatch(
      /contain-intrinsic-size:\s*auto 30px;/,
    );
    expect(workbenchStyles).not.toMatch(
      /\.execution-process-disclosure-body\s*\{[^}]*content-visibility:\s*hidden;/,
    );
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
    expect(group?.getAttribute("aria-expanded")).toBe("false");
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

    expect(container.querySelectorAll(".aw-icon--spinner")).toHaveLength(0);
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

    expect(container.querySelectorAll(".aw-icon--spinner")).toHaveLength(0);
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



});
