import { describe, expect, it } from "vitest";
import { providerBrandForId } from "./model-provider-brand";

describe("providerBrandForId", () => {
  it("maps every current provider entry to its official product brand", () => {
    expect({
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
    expect(providerBrandForId("custom-provider")).toBeUndefined();
  });
});
