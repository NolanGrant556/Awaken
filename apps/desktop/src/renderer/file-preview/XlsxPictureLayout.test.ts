// @vitest-environment jsdom
import { expect, it } from "vitest";
import { normalizeXlsxPictureHtml } from "../../main/xlsx-preview-pictures";

it("positions the real image box from grid coordinates and updates after layout changes", async () => {
  const html = `<body><div class="sheet-content active" data-sheet="0"><div class="table-wrapper"><table><colgroup>${"<col>".repeat(13)}</colgroup><tbody><tr data-row="0-4"><td/></tr></tbody></table><div style="position:absolute;width:350pt;height:640pt" data-from-col="11" data-from-row="3"><img class="xlsx-picture" src="data:image/png;base64,AA=="/></div></div></div></body>`;
  const normalized = normalizeXlsxPictureHtml(html, [
    [{ from: { col: 11, row: 3, dx: 7.5, dy: 3.75 }, width: 66, height: 84 }],
  ]);
  document.body.innerHTML = normalized;
  const doc = document,
    box = doc.querySelector<HTMLImageElement>(".xlsx-picture")!.parentElement!;
  let rowY = 105,
    resized: () => void = () => {};
  const rect = (left: number, top: number) =>
    ({ left, top, width: 60, height: 122 }) as DOMRect;
  Object.defineProperty(box, "offsetParent", {
    value: doc.querySelector(".table-wrapper"),
  });
  doc.querySelector(".sheet-content")!.getClientRects = () =>
    [rect(0, 0)] as unknown as DOMRectList;
  doc.querySelector(".table-wrapper")!.getBoundingClientRect = () => rect(0, 0);
  doc.querySelectorAll("col")[12].getBoundingClientRect = () => rect(920, 0);
  doc.querySelector("tr")!.getBoundingClientRect = () => rect(0, rowY);
  Object.defineProperty(doc, "fonts", { value: { ready: Promise.resolve() } });
  Object.assign(window, {
    ResizeObserver: class {
      constructor(callback: () => void) {
        resized = callback;
      }
      observe() {}
    },
  });
  window.eval(doc.querySelector("script")!.textContent!);
  expect(box.style.width).toBe("88px");
  expect(box.style.height).toBe("112px");
  expect(box.style.left).toBe("930px");
  expect(box.style.top).toBe("110px");
  rowY = 140;
  resized();
  expect(box.style.top).toBe("145px");
  await Promise.resolve();
  document.body.replaceChildren();
});
