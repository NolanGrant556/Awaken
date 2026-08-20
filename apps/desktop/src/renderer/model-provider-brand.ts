import type { AwakenProviderBrand } from "@awaken/design-system";

/** Maps Core providerId onto Design System logo catalog ids. Unknown ids stay unmapped. */

const PROVIDER_BRAND_BY_ID: Record<string, AwakenProviderBrand> = {
  openai: "openai",
  anthropic: "anthropic",
  google: "gemini",
  "kimi-coding-plan": "kimi",
  "kimi-cn": "kimi",
  volcengine: "volcengine",
  "volcengine-agent-plan": "volcengine",
  "qwen-token-plan": "qwen",
  qwen: "qwen",
  "minimax-token-plan": "minimax",
  minimax: "minimax",
  "glm-coding-plan": "glm",
  glm: "glm",
  doubao: "doubao",
  deepseek: "deepseek",
};

export function providerBrandForId(
  providerId: string,
): AwakenProviderBrand | undefined {
  return PROVIDER_BRAND_BY_ID[providerId];
}
