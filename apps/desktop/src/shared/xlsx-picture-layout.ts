import type { XlsxPictureGeometry } from "./xlsx-picture-geometry.js";
type Marker = XlsxPictureGeometry["from"];

// Structural browser-only surface: importing the generated script from Main
// must not add lib.dom globals to Node's Fetch/ReadableStream types.
type LayoutElement = {
  dataset: Record<string, string | undefined>;
  style: { visibility: string };
  parentElement: LayoutElement | null;
  offsetParent: LayoutElement | null;
  scrollLeft: number;
  scrollTop: number;
  getClientRects(): { length: number };
  getBoundingClientRect(): { left: number; top: number };
  querySelector(selector: string): LayoutElement | null;
  querySelectorAll(
    selector: string,
  ): Iterable<LayoutElement> & {
    forEach(callback: (element: LayoutElement, index: number) => void): void;
  };
};
type LayoutDocument = Pick<LayoutElement, "querySelectorAll"> & {
  fonts: { ready: Promise<unknown> };
};
type LayoutObserver = new (callback: () => void) => {
  observe(element: LayoutElement): void;
};

// This runs only inside the isolated Office HTML. It consumes numeric metadata
// and rendered grid geometry, with no filesystem, IPC, or network capability.
export function positionPictures(
  sheets: XlsxPictureGeometry[][],
  document: LayoutDocument,
  ResizeObserver: LayoutObserver,
): void {
  const update = () => {
    document.querySelectorAll(".sheet-content[data-sheet]").forEach((sheet) => {
      if (!sheet.getClientRects().length) return;
      const index = Number(sheet.dataset.sheet),
        table = sheet.querySelector("table");
      if (!table) return;
      const cols = Array.from(table.querySelectorAll("col"));
      sheet
        .querySelectorAll("img.xlsx-picture")
        .forEach((image, pictureIndex) => {
          const geometry = sheets[index]?.[pictureIndex],
            box = image.parentElement;
          const parent = box?.offsetParent;
          if (!geometry || !box || !parent) return;
          const origin = parent.getBoundingClientRect();
          const point = (anchor: Marker) => {
            const col = cols[anchor.col + 1];
            const row = table.querySelector(
              `tr[data-row="${index}-${anchor.row + 1}"]`,
            );
            if (!col || !row) return undefined;
            return {
              x:
                col.getBoundingClientRect().left -
                origin.left +
                parent.scrollLeft +
                (anchor.dx * 4) / 3,
              y:
                row.getBoundingClientRect().top -
                origin.top +
                parent.scrollTop +
                (anchor.dy * 4) / 3,
            };
          };
          const from = point(geometry.from),
            to = geometry.to ? point(geometry.to) : undefined;
          if (!from || (geometry.to && !to)) {
            box.style.visibility = "hidden";
            return;
          }
          const width = to ? to.x - from.x : (geometry.width! * 4) / 3;
          const height = to ? to.y - from.y : (geometry.height! * 4) / 3;
          Object.assign(box.style, {
            left: `${from.x}px`,
            top: `${from.y}px`,
            width: `${Math.max(0, width)}px`,
            height: `${Math.max(0, height)}px`,
            visibility: "visible",
          });
        });
    });
  };
  const observer = new ResizeObserver(update);
  document
    .querySelectorAll(".sheet-content table")
    .forEach((table) => observer.observe(table));
  document.fonts.ready.then(update);
  update();
}
