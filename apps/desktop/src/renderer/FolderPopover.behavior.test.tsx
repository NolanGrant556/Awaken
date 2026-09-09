// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AwakenProjectSkillsDialog } from "@awaken/design-system";
import { AwakenProjectHoverCard } from "../../../../Awaken Design System/src/components/AwakenProjectHoverCard/AwakenProjectHoverCard";

let host: HTMLDivElement;
let root: Root;
const folders = Array.from({ length: 40 }, (_, index) => ({
  path: `/isolated/folder-${index + 1}`,
  label: `Folder ${index + 1}`,
  isPrimary: index === 0,
}));

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

it("bounds a long Skills folder list while keeping the last empty folder selectable", async () => {
  const close = vi.fn();
  await act(async () => root.render(
    <AwakenProjectSkillsDialog
      open
      project={{ id: "project", name: "Folder test", folders, skills: [] }}
      onOpenChange={close}
      onSkillPreview={vi.fn()}
      onSkillToggle={vi.fn()}
    />,
  ));
  const trigger = document.querySelector<HTMLButtonElement>('[aria-label="筛选 40 个项目文件夹"]')!;
  await act(async () => trigger.click());
  const floating = document.querySelector<HTMLElement>(".aw-popover__floating")!;
  // jsdom checks sizing configuration; actual pixel bounds require Desktop evidence.
  expect(floating.style.position).toBe("fixed");
  expect(floating.style.getPropertyValue("--aw-popover-available-height"))
    .toMatch(/^\d+(?:\.\d+)?px$/);
  const options = document.querySelectorAll<HTMLButtonElement>(".aw-project-skills-dialog__folder-item");
  expect(options.length).toBe(41);
  await act(async () => options[40].click());
  expect(trigger.textContent).toContain("Folder 40");
  expect(document.querySelector(".aw-project-skills-dialog__folder-popover")).toBeNull();
  expect(document.querySelector(".aw-project-skills-dialog")).toBeTruthy();
  expect(close).not.toHaveBeenCalled();
});

it("bounds a long project hover card without losing reveal actions", async () => {
  const reveal = vi.fn();
  await act(async () => root.render(
    <AwakenProjectHoverCard card={{ title: "Folder test", folders, onRevealFolder: reveal }}>
      {(anchor) => <button {...anchor}>Project</button>}
    </AwakenProjectHoverCard>,
  ));
  await act(async () => {
    host.querySelector("button")!.dispatchEvent(new MouseEvent("mouseenter", {
      clientX: 100,
      clientY: 100,
    }));
    await new Promise((resolve) => setTimeout(resolve, 450));
  });
  const card = document.querySelector<HTMLElement>(".aw-project-hover-card")!;
  expect(card).toBeTruthy();
  expect(card.style.position).toBe("fixed");
  expect(card.style.maxHeight).toMatch(/^\d+(?:\.\d+)?px$/);
  const buttons = card.querySelectorAll<HTMLButtonElement>(".aw-project-hover-card__folder");
  expect(buttons.length).toBe(40);
  await act(async () => buttons[39].click());
  expect(reveal).toHaveBeenCalledExactlyOnceWith("/isolated/folder-40");
  await act(async () => card.dispatchEvent(new KeyboardEvent("keydown", {
    key: "Escape",
    bubbles: true,
  })));
  expect(document.querySelector(".aw-project-hover-card")).toBeNull();
});
