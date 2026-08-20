import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  AWAKEN_LOGO_CATALOG,
  assertAwakenLogoCatalog,
  findAwakenLogoByConcept,
  getAwakenLogo,
  searchAwakenLogos,
} from "./catalog";
import { awakenLogoOpticalSizeForDisplay } from "./construction";
import { PRODUCT_MARK_PATHS } from "./productMarks";

describe("Awaken logo catalog", () => {
  it("keeps unique ids and one logo per concept", () => {
    assert.doesNotThrow(() => assertAwakenLogoCatalog());
    assert.equal(getAwakenLogo("product-mark").concept, "Awaken 产品标记");
    assert.equal(findAwakenLogoByConcept("Awaken 横向字标")?.id, "product-wordmark");
  });

  it("picks 16px optical cuts at chrome size and 24px from 22px up", () => {
    assert.equal(awakenLogoOpticalSizeForDisplay(16), 16);
    assert.equal(awakenLogoOpticalSizeForDisplay(20), 16);
    assert.equal(awakenLogoOpticalSizeForDisplay(22), 24);
    assert.equal(awakenLogoOpticalSizeForDisplay(40), 24);
  });

  it("keeps separate 16 and 24 product-mark drawings", () => {
    assert.notDeepEqual(PRODUCT_MARK_PATHS[16].filled, PRODUCT_MARK_PATHS[24].filled);
    assert.equal(PRODUCT_MARK_PATHS[16].filled.length, 2);
    assert.equal(PRODUCT_MARK_PATHS[24].outline.length, 2);
  });

  it("finds logos by tag as well as name", () => {
    const hits = searchAwakenLogos("双柱");
    assert.equal(hits[0]?.id, "product-mark");
    assert.ok(searchAwakenLogos("claude").some((entry) => entry.id === "anthropic"));
  });

  it("registers every current provider brand", () => {
    for (const id of [
      "openai",
      "anthropic",
      "gemini",
      "kimi",
      "minimax",
      "glm",
      "qwen",
      "doubao",
      "deepseek",
      "volcengine",
    ] as const) {
      assert.equal(getAwakenLogo(id).kind, "provider");
    }
    assert.equal(AWAKEN_LOGO_CATALOG.filter((entry) => entry.kind === "product").length, 3);
  });
});
