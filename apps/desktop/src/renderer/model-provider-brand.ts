import type { AwakenProviderBrand } from "@awaken/design-system";

const PROVIDER_BRAND_BY_ID: Record<string, AwakenProviderBrand> = {
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
};

export function providerBrandForId(
  providerId: string,
): AwakenProviderBrand | undefined {
  return PROVIDER_BRAND_BY_ID[providerId];
}
