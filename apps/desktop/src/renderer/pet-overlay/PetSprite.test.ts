import { describe, expect, it, vi } from "vitest";
import { renderToString } from "react-dom/server";
import React from "react";
import { PetSprite } from "./PetSprite";

vi.mock("./pet-animation", () => ({
  usePetAnimation: () => ({
    row: 0,
    column: 0,
    backgroundPosition: "0% 0%",
  }),
}));

describe("PetSprite", () => {
  it("renders with spritesheet and state", () => {
    const html = renderToString(
      React.createElement(PetSprite, {
        spritesheetUrl: "awaken-pet://asset/test/spritesheet",
        state: "idle",
        widthPx: 112,
      }),
    );
    expect(html).toContain('class="pet-sprite"');
    expect(html).toContain('role="img"');
    expect(html).toContain("awaken-pet://asset/test/spritesheet");
  });

  it("renders Codex v2 sprites with 11-row background sizing", () => {
    const html = renderToString(
      React.createElement(PetSprite, {
        spritesheetUrl: "awaken-pet://asset/test/spritesheet",
        state: "idle",
        widthPx: 112,
        spriteVersionNumber: 2,
      }),
    );
    expect(html).toContain("--pet-sheet-rows:11");
  });
});
