import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  AWAKEN_ICON_CATALOG,
  assertAwakenIconCatalog,
  findAwakenIconByConcept,
  getAwakenIcon,
  searchAwakenIcons,
} from "./catalog";
import { AWAKEN_ICON_DRAWINGS } from "./drawings";
import { awakenLucideClassName } from "./lucideClass";
import { awakenIconPaint } from "./paint";
import { resolveAwakenIconOpticalSize } from "./stroke";
import { AWAKEN_ICON_IDS, AWAKEN_LUCIDE_GLYPH_IDS } from "./types";

describe("Awaken icon catalog", () => {
  it("keeps unique ids and one icon per concept", () => {
    assert.doesNotThrow(() => assertAwakenIconCatalog());
    assert.equal(getAwakenIcon("plug").concept, "连接器");
    assert.equal(findAwakenIconByConcept("发送")?.id, "arrow-up");
    assert.equal(AWAKEN_ICON_CATALOG.length, AWAKEN_ICON_IDS.length);
  });

  it("picks 16 optical cuts for chrome and 24 from 22px up", () => {
    assert.equal(resolveAwakenIconOpticalSize(12), 16);
    assert.equal(resolveAwakenIconOpticalSize(16), 16);
    assert.equal(resolveAwakenIconOpticalSize(20), 16);
    assert.equal(resolveAwakenIconOpticalSize(22), 24);
    assert.equal(resolveAwakenIconOpticalSize(40), 24);
  });

  it("keeps a drawing for every catalog id and drops 24-only detail at 16", () => {
    for (const id of AWAKEN_ICON_IDS) {
      const ops = AWAKEN_ICON_DRAWINGS[id];
      assert.ok(ops?.length, `missing drawing for ${id}`);
      assert.ok(
        ops.some((op) => op.detail === "both" || op.detail === "16"),
        `${id} has no 16-capable ops`,
      );
    }
    assert.ok(AWAKEN_ICON_DRAWINGS["file-text"].some((op) => op.detail === "24"));
    assert.ok(AWAKEN_ICON_DRAWINGS.loader.every((op) => op.kind === "poly" || op.kind === "line"));
  });

  it("maps every lucide-compatible name onto one catalog id", () => {
    for (const [name, id] of Object.entries(AWAKEN_LUCIDE_GLYPH_IDS)) {
      assert.ok(AWAKEN_ICON_IDS.includes(id), `${name} points at unknown ${id}`);
    }
    assert.equal(AWAKEN_LUCIDE_GLYPH_IDS.AlertCircle, AWAKEN_LUCIDE_GLYPH_IDS.CircleAlert);
    assert.equal(AWAKEN_LUCIDE_GLYPH_IDS.Loader2, "loader");
    assert.equal(AWAKEN_LUCIDE_GLYPH_IDS.FileType2, "file-type-2");
  });

  it("keeps lucide class names that current Desktop tests look for", () => {
    assert.equal(awakenLucideClassName("FileText"), "lucide lucide-file-text");
    assert.equal(awakenLucideClassName("Monitor"), "lucide lucide-monitor");
    assert.equal(awakenLucideClassName("FileType2"), "lucide lucide-file-type2");
    assert.equal(awakenLucideClassName("FileCode2"), "lucide lucide-file-code2");
  });

  it("fills closed shapes in filled style and keeps line icons as strokes", () => {
    const plugBody = AWAKEN_ICON_DRAWINGS.plug.find((op) => op.kind === "poly" && op.fillable);
    assert.ok(plugBody);
    assert.deepEqual(awakenIconPaint(plugBody, false), { fill: "none", stroke: "currentColor" });
    assert.deepEqual(awakenIconPaint(plugBody, true), { fill: "currentColor", stroke: "none" });
    assert.deepEqual(awakenIconPaint(AWAKEN_ICON_DRAWINGS.plus[0], true), {
      fill: "none",
      stroke: "currentColor",
    });
  });

  it("finds icons by concept and usage tags", () => {
    assert.ok(searchAwakenIcons("连接器").some((entry) => entry.id === "plug"));
    assert.ok(searchAwakenIcons("发送").some((entry) => entry.id === "arrow-up"));
    assert.equal(searchAwakenIcons("").length, AWAKEN_ICON_CATALOG.length);
  });
});
