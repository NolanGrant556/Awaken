// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenModelProviderForm } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("AwakenModelProviderForm custom model input", () => {
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

  it("renders custom models as a plain text field without suggestions", () => {
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

    const modelInput = host.querySelector<HTMLInputElement>(
      'input[placeholder="输入模型 ID"]',
    );
    expect(modelInput?.value).toBe("model-a");
    expect(host.querySelector(".model-custom-picker-trigger")).toBeNull();
    expect(host.querySelector('[role="listbox"]')).toBeNull();
    expect(host.querySelector('[role="option"]')).toBeNull();
  });

  it("shows synchronized candidates while keeping official providers editable", () => {
    const onModelChange = vi.fn();
    act(() => {
      root.render(
        <AwakenModelProviderForm
          providerId="qwen"
          modelMode="editable"
          models={["qwen3.8-max", "qwen3.7-plus"]}
          modelId="qwen3.8-max"
          baseURL="https://dashscope.aliyuncs.com/compatible-mode/v1"
          apiKey="key"
          apiKeyPlaceholder="API Key"
          apiDocsURL="https://help.aliyun.com"
          onModelChange={onModelChange}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />,
      );
    });

    const input = host.querySelector<HTMLInputElement>('input[role="combobox"]');
    expect(input?.value).toBe("qwen3.8-max");
    expect(host.querySelector("datalist")).toBeNull();
    expect(input?.getAttribute("aria-expanded")).toBe("false");
    act(() => input?.focus());
    expect([...host.querySelectorAll('[role="option"]')].map((option) => option.textContent))
      .toEqual(["qwen3.8-max", "qwen3.7-plus"]);
    expect(input?.getAttribute("aria-expanded")).toBe("true");
    act(() => host.querySelectorAll<HTMLButtonElement>('[role="option"]')[1]?.click());
    expect(onModelChange).toHaveBeenCalledWith("qwen3.7-plus");
    act(() => {
      if (input) {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
        setter?.call(input, "qwen-custom-id");
        input.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    expect(onModelChange).toHaveBeenCalledWith("qwen-custom-id");
  });

  it("offers Ark Model IDs while allowing a manual Endpoint ID", () => {
    const onModelChange = vi.fn();
    act(() => {
      root.render(
        <AwakenModelProviderForm
          providerId="doubao"
          modelMode="endpoint"
          models={[
            "doubao-seed-2-1-pro-260628",
            "doubao-seed-2-1-turbo-260628",
          ]}
          modelDisplayNames={{
            "doubao-seed-2-1-pro-260628": "Doubao-Seed-2.1-Pro",
            "doubao-seed-2-1-turbo-260628": "Doubao-Seed-2.1-Turbo",
          }}
          modelId="doubao-seed-2-1-pro-260628"
          baseURL="https://ark.cn-beijing.volces.com/api/v3"
          apiKey="key"
          apiKeyPlaceholder="API Key"
          apiDocsURL="https://console.volcengine.com"
          onModelChange={onModelChange}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />,
      );
    });

    const input = host.querySelector<HTMLInputElement>('input[role="combobox"]');
    expect(input?.placeholder).toBe("选择或输入 Model ID / Endpoint ID");
    expect(input?.value).toBe("Doubao-Seed-2.1-Pro");
    expect(host.textContent).toContain("模型 / 推理接入点");
    act(() => input?.focus());
    expect([...host.querySelectorAll('[role="option"]')].map((option) => option.textContent))
      .toEqual([
        "Doubao-Seed-2.1-Pro",
        "Doubao-Seed-2.1-Turbo",
      ]);
    act(() => host.querySelectorAll<HTMLButtonElement>('[role="option"]')[1]?.click());
    expect(onModelChange).toHaveBeenCalledWith("doubao-seed-2-1-turbo-260628");
    act(() => {
      if (input) {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
        setter?.call(input, "ep-custom-endpoint");
        input.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    expect(onModelChange).toHaveBeenCalledWith("ep-custom-endpoint");
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

  it("requires a model and Base URL even when no authentication is selected", () => {
    const onTest = vi.fn();
    const renderForm = (modelId: string, baseURL: string) => (
      <AwakenModelProviderForm
        providerId="custom"
        presentation="modal"
        modelMode="custom"
        models={[]}
        modelId={modelId}
        authScheme="none"
        baseURL={baseURL}
        apiKey=""
        apiKeyPlaceholder="API Key"
        apiDocsURL=""
        onModelChange={vi.fn()}
        onBaseURLChange={vi.fn()}
        onApiKeyChange={vi.fn()}
        onTest={onTest}
      />
    );

    act(() => root.render(renderForm("", "")));
    const button = host.querySelector<HTMLButtonElement>(".model-provider-primary-test")!;
    expect(button.disabled).toBe(true);

    act(() => root.render(renderForm("local-model", "")));
    expect(button.disabled).toBe(true);

    act(() => root.render(renderForm("local-model", "http://127.0.0.1:8080/v1")));
    expect(button.disabled).toBe(false);
    act(() => button.click());
    expect(onTest).toHaveBeenCalledTimes(1);
  });

  it("renders single protocol and model choices as readonly fields", () => {
    act(() => {
      root.render(
        <AwakenModelProviderForm
          providerId="openai"
          presentation="modal"
          modelMode="select"
          models={["gpt-5.1"]}
          modelId="gpt-5.1"
          protocol="openai_responses"
          protocolOptions={[
            { value: "openai_responses", label: "OpenAI Responses" },
          ]}
          authScheme="bearer"
          authSchemeOptions={[
            { value: "bearer", label: "Authorization: Bearer" },
          ]}
          baseURL="https://api.openai.com/v1"
          apiKey="demo-key"
          apiKeyPlaceholder="API Key"
          apiDocsURL="https://platform.openai.com"
          onModelChange={vi.fn()}
          onProtocolChange={vi.fn()}
          onAuthSchemeChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />,
      );
    });

    expect(host.querySelectorAll(".model-provider-readonly-choice")).toHaveLength(2);
    expect(host.querySelectorAll(".aw-select__trigger")).toHaveLength(0);
    expect(host.textContent).not.toContain("API 密钥发送方式");
    expect(host.textContent).not.toContain("当前 Header：");
  });

  it("updates the auth header example and catalog refresh action from props", () => {
    const onRefreshCatalog = vi.fn();
    const renderForm = (
      authScheme: string,
      authSchemeOptions: Array<{ value: string; label: string }>,
    ) => (
      <AwakenModelProviderForm
        providerId="custom-provider"
        presentation="modal"
        modelMode="custom"
        models={["model-a", "model-b"]}
        modelId="model-a"
        authScheme={authScheme}
        authSchemeOptions={authSchemeOptions}
        showAuthScheme
        baseURL="https://example.com/v1"
        apiKey="demo-key"
        apiKeyPlaceholder="API Key"
        apiDocsURL="https://example.com/docs"
        showCatalogRefresh
        catalogStatus="目录已刷新"
        onModelChange={vi.fn()}
        onAuthSchemeChange={vi.fn()}
        onBaseURLChange={vi.fn()}
        onApiKeyChange={vi.fn()}
        onRefreshCatalog={onRefreshCatalog}
      />
    );

    act(() =>
      root.render(
        renderForm("x-api-key", [
          { value: "x-api-key", label: "x-api-key" },
          { value: "none", label: "无鉴权" },
        ]),
      ),
    );
    expect(host.textContent).toContain("x-api-key: <API_KEY>");
    expect(host.textContent).toContain("目录已刷新");

    const refreshButton = Array.from(host.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("刷新模型目录"),
    );
    expect(refreshButton).toBeTruthy();
    act(() => refreshButton?.click());
    expect(onRefreshCatalog).toHaveBeenCalledTimes(1);

    act(() =>
      root.render(
        renderForm("none", [
          { value: "x-api-key", label: "x-api-key" },
          { value: "none", label: "无鉴权" },
        ]),
      ),
    );
    expect(host.textContent).toContain("无鉴权（不发送认证 Header）");
  });

  it("keeps custom Base URL in the primary form without advanced capability controls", () => {
    act(() => {
      root.render(
        <AwakenModelProviderForm
          providerId="custom"
          presentation="modal"
          modelMode="custom"
          models={[]}
          modelId="custom-model"
          baseURL="https://example.com/v1"
          showBaseURL
          apiKey="demo-key"
          apiKeyPlaceholder="API Key"
          apiDocsURL="https://example.com/docs"
          onModelChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onApiKeyChange={vi.fn()}
        />,
      );
    });

    expect(host.textContent).toContain("Base URL");
    expect(host.textContent).not.toContain("高级设置");
    expect(host.textContent).not.toContain("输入能力");
    expect(host.textContent).not.toContain("已解析能力与来源");
  });
});
