import { AWAKEN_LOGO_CATALOG, getAwakenLogo } from "@awaken/design-system";
import { describe, expect, it } from "vitest";
import { providerBrandForId } from "./model-provider-brand";

describe("providerBrandForId", () => {
  it("maps every current provider entry to its official product brand", () => {
    expect({
      openai: providerBrandForId("openai"),
      anthropic: providerBrandForId("anthropic"),
      google: providerBrandForId("google"),
      "kimi-coding-plan": providerBrandForId("kimi-coding-plan"),
      "kimi-cn": providerBrandForId("kimi-cn"),
      volcengine: providerBrandForId("volcengine"),
      "volcengine-agent-plan": providerBrandForId("volcengine-agent-plan"),
      "qwen-token-plan": providerBrandForId("qwen-token-plan"),
      qwen: providerBrandForId("qwen"),
      minimax: providerBrandForId("minimax"),
      glm: providerBrandForId("glm"),
      doubao: providerBrandForId("doubao"),
      deepseek: providerBrandForId("deepseek"),
    }).toEqual({
      openai: "openai",
      anthropic: "anthropic",
      google: "gemini",
      "kimi-coding-plan": "kimi",
      "kimi-cn": "kimi",
      volcengine: "volcengine",
      "volcengine-agent-plan": "volcengine",
      "qwen-token-plan": "qwen",
      qwen: "qwen",
      minimax: "minimax",
      glm: "glm",
      doubao: "doubao",
      deepseek: "deepseek",
    });
  });

  it("does not invent a logo for an unknown provider", () => {
    expect(providerBrandForId("custom")).toBeUndefined();
    expect(providerBrandForId("custom-provider")).toBeUndefined();
  });

  it("only maps onto Design System catalog provider logos", () => {
    const mapped = [
      "openai",
      "anthropic",
      "gemini",
      "kimi",
      "volcengine",
      "qwen",
      "minimax",
      "glm",
      "doubao",
      "deepseek",
    ] as const;
    for (const brand of mapped) {
      expect(getAwakenLogo(brand).kind).toBe("provider");
    }
    expect(
      AWAKEN_LOGO_CATALOG.filter((entry) => entry.kind === "provider")
        .map((entry) => entry.id)
        .sort(),
    ).toEqual([...mapped].sort());
  });
});
