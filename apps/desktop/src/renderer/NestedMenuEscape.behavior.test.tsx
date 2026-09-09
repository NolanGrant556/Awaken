// @vitest-environment jsdom
import { act, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  AwakenDialog,
  AwakenDropdown,
  AwakenModelProviderForm,
  AwakenSelect,
} from "@awaken/design-system";

let host: HTMLDivElement;
let root: Root;

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

function DraftDialog({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(true);
  return (
    <AwakenDialog open={open} title="未保存的设置" onOpenChange={setOpen}>
      <input aria-label="草稿" defaultValue="需要保留的修改" />
      {children}
    </AwakenDialog>
  );
}

async function click(element: HTMLElement) {
  await act(async () => {
    element.focus();
    element.click();
  });
}

async function escape(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  });
}

function expectDraft() {
  expect(document.querySelector<HTMLInputElement>('[aria-label="草稿"]')?.value)
    .toBe("需要保留的修改");
}

it.each(["trigger", "option"])(
  "Select Escape from %s closes only the menu, then a second Escape closes the dialog",
  async (origin) => {
    await act(async () => root.render(
      <DraftDialog>
        <AwakenSelect
          value="a"
          options={[{ value: "a", label: "A" }, { value: "b", label: "B" }]}
          ariaLabel="测试选择"
          onValueChange={vi.fn()}
        />
      </DraftDialog>,
    ));
    const trigger = document.querySelector<HTMLButtonElement>('[aria-label="测试选择"]')!;
    await click(trigger);
    const target = origin === "trigger"
      ? trigger
      : document.querySelector<HTMLElement>('[role="option"]')!;
    await act(async () => target.focus());
    await escape(target);
    expect(document.querySelector('[role="listbox"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expectDraft();
    await escape(trigger);
    expect(document.querySelector('[aria-label="草稿"]')).toBeNull();
  },
);

it.each(["input", "option", "toggle"])(
  "model catalog Escape from %s preserves provider settings",
  async (origin) => {
    await act(async () => root.render(
      <DraftDialog>
        <AwakenModelProviderForm
          providerId="test"
          modelMode="endpoint"
          models={["model-a", "model-b"]}
          modelId="model-a"
          baseURL="https://example.com"
          apiKey=""
          apiKeyPlaceholder="Key"
          apiDocsURL="https://example.com/docs"
          onModelChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />
      </DraftDialog>,
    ));
    const input = document.querySelector<HTMLInputElement>('[role="combobox"]')!;
    await act(async () => input.focus());
    expect(input.getAttribute("aria-expanded")).toBe("true");
    const target = origin === "option"
      ? document.querySelector<HTMLElement>('[role="option"]')!
      : origin === "toggle"
        ? document.querySelector<HTMLElement>(".model-provider-model-combobox__toggle")!
        : input;
    await act(async () => target.focus());
    await escape(target);
    expectDraft();
    expect(input.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(input);
    await escape(input);
    expect(document.querySelector('[aria-label="草稿"]')).toBeNull();
  },
);

it("a portalled dropdown Escape preserves its parent dialog", async () => {
  await act(async () => root.render(
    <DraftDialog>
      <AwakenDropdown trigger={<button aria-label="菜单">菜单</button>}>
        <button role="menuitem">菜单操作</button>
      </AwakenDropdown>
    </DraftDialog>,
  ));
  await click(document.querySelector<HTMLButtonElement>('[aria-label="菜单"]')!);
  const option = document.querySelector<HTMLElement>('[role="menuitem"]')!;
  await act(async () => option.focus());
  await escape(option);
  expect(document.querySelector('[role="menuitem"]')).toBeNull();
  expectDraft();
});
