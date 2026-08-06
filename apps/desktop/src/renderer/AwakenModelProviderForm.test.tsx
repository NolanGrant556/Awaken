// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenModelProviderForm } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("AwakenModelProviderForm custom model suggestions", () => {
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
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 768,
    });
    vi.restoreAllMocks();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  it("flips editable model suggestions and recomputes while open", () => {
    act(() => {
      root.render(
        <AwakenModelProviderForm
          providerId="custom-provider"
          modelMode="custom"
          models={["model-a", "model-b", "model-c", "model-d"]}
          modelId="model-a"
          baseURL="https://example.com/v1"
          apiKey=""
          apiKeyPlaceholder="API Key"
          apiDocsURL="https://example.com/docs"
          onModelChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />,
      );
    });

    const picker = host.querySelector<HTMLElement>(".model-custom-picker")!;
    const trigger = host.querySelector<HTMLButtonElement>(
      ".model-custom-picker-trigger",
    )!;
    vi.spyOn(picker, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 430,
      left: 100,
      top: 430,
      right: 500,
      bottom: 470,
      width: 400,
      height: 40,
      toJSON: () => ({}),
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 500,
    });

    act(() => trigger.click());
    expect(picker.dataset.menuPlacement).toBe("top");
    expect(host.querySelectorAll('[role="option"]')).toHaveLength(4);

    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 900,
    });
    act(() => window.dispatchEvent(new Event("resize")));
    expect(picker.dataset.menuPlacement).toBe("bottom");
  });

  it("keeps every API-key connection test disabled until the current form has a key", () => {
    const onTest = vi.fn();
    const renderForm = (apiKey: string) => (
      <AwakenModelProviderForm
        providerId="custom-provider"
        presentation="modal"
        modelMode="select"
        models={["model-a"]}
        modelId="model-a"
        baseURL="https://example.com/v1"
        apiKey={apiKey}
        apiKeyPlaceholder="API Key"
        apiDocsURL="https://example.com/docs"
        onModelChange={vi.fn()}
        onBaseURLChange={vi.fn()}
        onApiKeyChange={vi.fn()}
        onTest={onTest}
      />
    );

    act(() => root.render(renderForm("   ")));
    const button = host.querySelector<HTMLButtonElement>(
      ".model-provider-primary-test",
    )!;
    expect(button.disabled).toBe(true);
    act(() => button.click());
    expect(onTest).not.toHaveBeenCalled();

    act(() => root.render(renderForm("new-key")));
    expect(button.disabled).toBe(false);
    act(() => button.click());
    expect(onTest).toHaveBeenCalledTimes(1);
  });
});
