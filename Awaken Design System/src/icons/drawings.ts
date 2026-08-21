import type { AwakenIconId, AwakenIconOp } from "./types";

export function line(
  a: readonly [number, number],
  b: readonly [number, number],
  detail: AwakenIconOp["detail"] = "both",
): AwakenIconOp {
  return { kind: "line", a, b, detail };
}

export function poly(
  points: ReadonlyArray<readonly [number, number]>,
  close = false,
  fillable = false,
  detail: AwakenIconOp["detail"] = "both",
): AwakenIconOp {
  return { kind: "poly", points, close, fillable, detail };
}

export function circle(
  c: readonly [number, number],
  r: number,
  fillable = false,
  detail: AwakenIconOp["detail"] = "both",
): AwakenIconOp {
  return { kind: "circle", c, r, fillable, detail };
}

export function rect(
  x: number,
  y: number,
  w: number,
  h: number,
  r = 0,
  fillable = false,
  detail: AwakenIconOp["detail"] = "both",
): AwakenIconOp {
  return { kind: "rect", x, y, w, h, r, fillable, detail };
}

export const AWAKEN_ICON_DRAWINGS: Record<AwakenIconId, readonly AwakenIconOp[]> = {
  "plus": [
    line([12, 5], [12, 19], "both"),
    line([5, 12], [19, 12], "both"),
  ],
  "minus": [
    line([5, 12], [19, 12], "both"),
  ],
  "x": [
    line([6, 6], [18, 18], "both"),
    line([18, 6], [6, 18], "both"),
  ],
  "check": [
    poly([[5, 12.5], [10, 17.5], [19, 6.5]], false, false, "both"),
  ],
  "check-check": [
    poly([[4, 12], [8.5, 16.5], [16, 7]], false, false, "both"),
    poly([[9, 14.5], [12, 17.5], [20.5, 8]], false, false, "24"),
  ],
  "search": [
    circle([10.5, 10.5], 5.2, false, "both"),
    line([14.4, 14.4], [19.5, 19.5], "both"),
  ],
  "more-horizontal": [
    circle([6, 12], 1.15, true, "both"),
    circle([12, 12], 1.15, true, "both"),
    circle([18, 12], 1.15, true, "both"),
  ],
  "chevron-right": [
    poly([[9, 6], [15, 12], [9, 18]], false, false, "both"),
  ],
  "chevron-left": [
    poly([[15, 6], [9, 12], [15, 18]], false, false, "both"),
  ],
  "chevron-down": [
    poly([[6, 9], [12, 15], [18, 9]], false, false, "both"),
  ],
  "chevron-up": [
    poly([[6, 15], [12, 9], [18, 15]], false, false, "both"),
  ],
  "chevrons-up-down": [
    poly([[7, 9], [12, 5], [17, 9]], false, false, "both"),
    poly([[7, 15], [12, 19], [17, 15]], false, false, "both"),
  ],
  "chevrons-down-up": [
    poly([[7, 6], [12, 10], [17, 6]], false, false, "both"),
    poly([[7, 18], [12, 14], [17, 18]], false, false, "both"),
  ],
  "arrow-left": [
    line([19, 12], [5, 12], "both"),
    poly([[10, 7], [5, 12], [10, 17]], false, false, "both"),
  ],
  "arrow-right": [
    line([5, 12], [19, 12], "both"),
    poly([[14, 7], [19, 12], [14, 17]], false, false, "both"),
  ],
  "arrow-up": [
    line([12, 19], [12, 5], "both"),
    poly([[7, 10], [12, 5], [17, 10]], false, false, "both"),
  ],
  "arrow-up-right": [
    line([7, 17], [17, 7], "both"),
    line([10, 7], [17, 7], "both"),
    line([17, 7], [17, 14], "both"),
  ],
  "arrow-down-circle": [
    circle([12, 12], 8.2, false, "both"),
    line([12, 7.5], [12, 15.5], "both"),
    poly([[8.5, 12.5], [12, 16], [15.5, 12.5]], false, false, "both"),
  ],
  "external-link": [
    rect(4.5, 8.5, 11, 11, 1.5, true, "both"),
    line([13, 5], [19.5, 5], "both"),
    line([19.5, 5], [19.5, 11.5], "both"),
    line([12.5, 11.5], [19.5, 5], "both"),
  ],
  "folder": [
    poly([[3.5, 8], [3.5, 19], [20.5, 19], [20.5, 8], [12.5, 8], [10.5, 5.5], [3.5, 5.5]], true, true, "both"),
  ],
  "folder-open": [
    poly([[3.5, 10], [3.5, 19.5], [20.5, 19.5], [20.5, 10], [12.2, 10], [10.4, 7.2], [3.5, 7.2]], true, true, "both"),
    poly([[3.5, 12.5], [9, 8.2], [21, 8.2], [20.5, 12.5]], false, false, "24"),
  ],
  "folder-plus": [
    poly([[3.5, 8], [3.5, 19], [13.2, 19], [13.2, 13.2], [20.5, 13.2], [20.5, 8], [12.5, 8], [10.5, 5.5], [3.5, 5.5]], true, true, "both"),
    line([16.8, 14.8], [16.8, 21], "both"),
    line([13.6, 17.9], [20, 17.9], "both"),
  ],
  "folders": [
    poly([[6, 6.5], [6, 16], [21, 16], [21, 6.5], [14.2, 6.5], [12.6, 4.5], [6, 4.5]], true, true, "both"),
    poly([[3, 9.5], [3, 20], [17.5, 20]], false, false, "24"),
  ],
  "folder-search": [
    poly([[3.5, 8], [3.5, 19], [13, 19], [13, 14], [20.5, 14], [20.5, 8], [12.5, 8], [10.5, 5.5], [3.5, 5.5]], true, true, "both"),
    circle([16.6, 17.2], 3.1, false, "both"),
    line([18.9, 19.5], [21.4, 21.8], "both"),
  ],
  "file": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
  ],
  "file-text": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    line([8, 11], [16, 11], "both"),
    line([8, 14], [16, 14], "both"),
    line([8, 17], [13.5, 17], "24"),
  ],
  "file-spreadsheet": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    rect(8, 10, 8, 8, 0, false, "both"),
    line([12, 10], [12, 18], "both"),
    line([8, 14], [16, 14], "both"),
  ],
  "presentation": [
    rect(4, 6, 16, 10, 1.2, true, "both"),
    line([12, 16], [12, 19.5], "both"),
    line([8, 19.5], [16, 19.5], "both"),
  ],
  "file-type": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[8.5, 17], [8.5, 11], [11.2, 15], [13.9, 11], [13.9, 17]], false, false, "both"),
  ],
  "file-type-2": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[8, 17.5], [8, 11], [11.5, 11], [11.5, 14], [8, 14]], false, false, "both"),
    line([8, 14], [12.2, 17.5], "both"),
  ],
  "file-code": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[9, 12], [7.2, 14.5], [9, 17]], false, false, "both"),
    poly([[15, 12], [16.8, 14.5], [15, 17]], false, false, "both"),
  ],
  "file-json": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[9, 11.5], [7.5, 14.5], [9, 17.5]], false, false, "both"),
    poly([[15, 11.5], [16.5, 14.5], [15, 17.5]], false, false, "both"),
    line([11.2, 11], [12.8, 18], "24"),
  ],
  "file-image": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[8, 17.5], [11, 13.2], [13.2, 15.6], [15, 13.6], [16.8, 17.5]], true, true, "both"),
    circle([10.2, 11.6], 1.05, true, "24"),
  ],
  "file-archive": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    line([12, 9.5], [12, 18], "both"),
    line([9.5, 12], [14.5, 12], "both"),
    line([9.5, 15], [14.5, 15], "24"),
  ],
  "file-audio": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    circle([10.2, 16.2], 1.6, true, "both"),
    poly([[11.8, 16.2], [11.8, 10.8], [16, 9.8], [16, 12]], false, false, "both"),
  ],
  "file-video": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    poly([[8.2, 11.5], [8.2, 17.2], [13.2, 14.4]], true, true, "both"),
  ],
  "file-up": [
    poly([[7, 3], [15, 3], [19, 7], [19, 21], [5, 21], [5, 3]], true, true, "both"),
    poly([[15, 3], [15, 7], [19, 7]], false, false, "24"),
    line([12, 18], [12, 11], "both"),
    poly([[9, 13.5], [12, 10.5], [15, 13.5]], false, false, "both"),
  ],
  "book-open-text": [
    poly([[12, 5.5], [5, 7.2], [5, 19], [12, 17.2]], true, true, "both"),
    poly([[12, 5.5], [19, 7.2], [19, 19], [12, 17.2]], true, true, "both"),
    line([7.4, 10.5], [10.2, 9.9], "24"),
    line([13.8, 9.9], [16.6, 10.5], "24"),
  ],
  "package": [
    poly([[4.5, 8.5], [12, 4.8], [19.5, 8.5], [19.5, 16.8], [12, 20.2], [4.5, 16.8]], true, true, "both"),
    line([4.5, 8.5], [19.5, 8.5], "both"),
    line([12, 4.8], [12, 20.2], "both"),
  ],
  "message-circle": [
    circle([12, 10.8], 7, false, "both"),
    poly([[8.2, 16.4], [7.2, 20.4], [12.2, 17.2]], false, false, "both"),
  ],
  "message-circle-plus": [
    circle([10.6, 10.4], 6.4, false, "both"),
    poly([[7.4, 15.6], [6.6, 19.4], [11.2, 16.4]], false, false, "both"),
    line([17.2, 15.6], [17.2, 21.2], "both"),
    line([14.4, 18.4], [20, 18.4], "both"),
  ],
  "message-square": [
    rect(4.5, 4.5, 15, 12.2, 1.6, true, "both"),
    poly([[8, 16.6], [8, 20.2], [12.6, 16.6]], false, false, "both"),
  ],
  "user-round": [
    circle([12, 8.4], 3.2, true, "both"),
    poly([[5.5, 19.5], [7.2, 14.2], [16.8, 14.2], [18.5, 19.5]], false, false, "both"),
  ],
  "monitor": [
    rect(3.5, 4.5, 17, 12, 1.4, true, "both"),
    line([12, 16.5], [12, 19], "both"),
    line([8, 19.5], [16, 19.5], "both"),
  ],
  "monitor-cog": [
    rect(3.5, 4.5, 17, 11.2, 1.3, true, "both"),
    line([12, 15.7], [12, 18.2], "both"),
    line([8.2, 18.8], [15.8, 18.8], "both"),
    circle([17.4, 17.4], 2.5, false, "24"),
  ],
  "cpu": [
    rect(7, 7, 10, 10, 1.2, true, "both"),
    line([12, 4], [12, 7], "both"),
    line([12, 17], [12, 20], "both"),
    line([4, 12], [7, 12], "both"),
    line([17, 12], [20, 12], "both"),
    line([7.6, 5.2], [9.2, 7], "24"),
    line([16.4, 5.2], [14.8, 7], "24"),
  ],
  "bot": [
    rect(6, 8, 12, 10, 2, true, "both"),
    line([12, 8], [12, 5.2], "both"),
    circle([12, 4.6], 1.1, true, "both"),
    circle([9.4, 12.2], 1.05, true, "both"),
    circle([14.6, 12.2], 1.05, true, "both"),
  ],
  "sun": [
    circle([12, 12], 3.6, true, "both"),
    line([12, 4.2], [12, 6.4], "both"),
    line([12, 17.6], [12, 19.8], "both"),
    line([4.2, 12], [6.4, 12], "both"),
    line([17.6, 12], [19.8, 12], "both"),
    line([6.4, 6.4], [7.9, 7.9], "both"),
    line([16.1, 16.1], [17.6, 17.6], "both"),
    line([17.6, 6.4], [16.1, 7.9], "both"),
    line([7.9, 16.1], [6.4, 17.6], "both"),
  ],
  "moon": [
    poly([[15.6, 5.2], [8.4, 6.4], [6.2, 13.8], [10.8, 19], [18.2, 16.6], [14.6, 12.2]], true, true, "both"),
  ],
  "palette": [
    poly([[4.8, 14.5], [6.2, 7.2], [12, 4.6], [18.6, 7.8], [19.2, 14], [14.8, 19.2], [9.2, 18.6]], true, true, "both"),
    circle([9.2, 10.4], 1.05, true, "both"),
    circle([12.6, 8.6], 1.05, true, "both"),
    circle([16, 10.8], 1.05, true, "both"),
  ],
  "panel-left": [
    rect(3.5, 4.5, 17, 15, 1.4, true, "both"),
    line([9.2, 4.5], [9.2, 19.5], "both"),
  ],
  "panel-right-open": [
    rect(3.5, 4.5, 17, 15, 1.4, true, "both"),
    line([14.8, 4.5], [14.8, 19.5], "both"),
    poly([[9, 9], [12.4, 12], [9, 15]], false, false, "both"),
  ],
  "panel-right-close": [
    rect(3.5, 4.5, 17, 15, 1.4, true, "both"),
    line([14.8, 4.5], [14.8, 19.5], "both"),
    poly([[12.6, 9], [9.2, 12], [12.6, 15]], false, false, "both"),
  ],
  "app-window": [
    rect(3.5, 5, 17, 14, 1.4, true, "both"),
    line([3.5, 9], [20.5, 9], "both"),
    circle([6.2, 7], 0.7, true, "24"),
    circle([8.4, 7], 0.7, true, "24"),
  ],
  "clock": [
    circle([12, 12], 8.1, false, "both"),
    line([12, 12], [12, 7.4], "both"),
    line([12, 12], [16.2, 13.6], "both"),
  ],
  "clock-3": [
    circle([12, 12], 8.1, false, "both"),
    line([12, 12], [16.4, 12], "both"),
    line([12, 12], [12, 8], "both"),
  ],
  "calendar-days": [
    rect(4.5, 6.5, 15, 13.5, 1.3, true, "both"),
    line([4.5, 10.4], [19.5, 10.4], "both"),
    line([8.2, 4.6], [8.2, 8], "both"),
    line([15.8, 4.6], [15.8, 8], "both"),
    circle([9, 13.6], 0.85, true, "24"),
    circle([12, 13.6], 0.85, true, "24"),
    circle([15, 13.6], 0.85, true, "24"),
  ],
  "calendar-clock": [
    rect(4, 6.2, 12.6, 13.4, 1.2, true, "both"),
    line([4, 10], [16.6, 10], "both"),
    line([7.2, 4.4], [7.2, 7.6], "both"),
    line([13.2, 4.4], [13.2, 7.6], "both"),
    circle([18, 17.2], 4.1, false, "both"),
    line([18, 15], [18, 17.2], "both"),
    line([18, 17.2], [20.1, 18.4], "both"),
  ],
  "history": [
    circle([12.4, 12.4], 7.2, false, "both"),
    poly([[7.2, 5.6], [7.2, 9.6], [11.2, 9.6]], false, false, "both"),
    line([12.4, 9.4], [12.4, 13], "both"),
    line([12.4, 13], [15.4, 14.6], "both"),
  ],
  "settings": [
    circle([12, 12], 3.1, false, "both"),
    poly([[12, 4.4], [13.4, 6.6], [16.2, 5.8], [17.6, 8.4], [15.6, 10], [15.6, 14], [17.6, 15.6], [16.2, 18.2], [13.4, 17.4], [12, 19.6], [10.6, 17.4], [7.8, 18.2], [6.4, 15.6], [8.4, 14], [8.4, 10], [6.4, 8.4], [7.8, 5.8], [10.6, 6.6]], true, true, "both"),
  ],
  "wrench": [
    poly([[7.2, 4.8], [10.4, 4.8], [10.4, 8.6], [18.8, 17], [16.6, 19.2], [8.2, 10.8], [4.8, 10.8], [4.8, 7.6], [7.2, 7.6]], true, true, "both"),
  ],
  "plug": [
    line([8.4, 3.8], [8.4, 7.6], "both"),
    line([15.6, 3.8], [15.6, 7.6], "both"),
    poly([[6.2, 7.6], [17.8, 7.6], [17.8, 14.2], [12, 20], [6.2, 14.2]], true, true, "both"),
  ],
  "plug-zap": [
    line([7.6, 3.8], [7.6, 7], "both"),
    line([13.2, 3.8], [13.2, 7], "both"),
    poly([[5.6, 7], [15.2, 7], [15.2, 12.8], [10.4, 17.8], [5.6, 12.8]], true, true, "both"),
    poly([[15.2, 13.6], [19.4, 13.6], [16.6, 18.2], [20.4, 18.2], [14.8, 22]], false, false, "both"),
  ],
  "server": [
    rect(4.5, 4.2, 15, 5.2, 1, true, "both"),
    rect(4.5, 10.4, 15, 5.2, 1, true, "both"),
    rect(4.5, 16.6, 15, 3.6, 1, true, "both"),
    circle([7.4, 6.8], 0.7, true, "both"),
    circle([7.4, 13], 0.7, true, "both"),
  ],
  "wifi": [
    poly([[5, 10.2], [12, 5.6], [19, 10.2]], false, false, "both"),
    poly([[7.4, 13.2], [12, 10.2], [16.6, 13.2]], false, false, "24"),
    circle([12, 17.6], 1.2, true, "both"),
  ],
  "webhook": [
    circle([8.2, 8.4], 3.1, false, "both"),
    circle([16.4, 9.2], 3.1, false, "both"),
    circle([12, 17.2], 3.1, false, "both"),
    line([10.6, 10.2], [12.2, 14.6], "both"),
    line([14.2, 11.4], [13.4, 14.4], "both"),
  ],
  "brain": [
    poly([[6.2, 14.8], [5.2, 10.2], [7.4, 6.2], [12, 4.8], [16.8, 6.4], [18.8, 10.6], [17.6, 15], [12, 19.4]], true, true, "both"),
    line([12, 8.2], [12, 16.8], "both"),
    poly([[8.4, 11.2], [12, 13], [15.6, 11.2]], false, false, "24"),
  ],
  "paw-print": [
    circle([12, 15.4], 3.3, true, "both"),
    circle([6.8, 10.2], 1.7, true, "both"),
    circle([10.2, 7.4], 1.7, true, "both"),
    circle([13.8, 7.4], 1.7, true, "both"),
    circle([17.2, 10.2], 1.7, true, "both"),
  ],
  "wallet": [
    rect(3.8, 7.2, 16.4, 11.4, 1.4, true, "both"),
    rect(13.6, 11, 6.6, 4.2, 1, true, "both"),
  ],
  "gauge": [
    poly([[5.2, 16.5], [6.4, 8.6], [12, 5.4], [17.6, 8.6], [18.8, 16.5]], false, false, "both"),
    line([12, 14.6], [16.2, 9.2], "both"),
    circle([12, 14.8], 1.15, true, "both"),
  ],
  "key-round": [
    circle([8.2, 12], 4.2, false, "both"),
    line([12.2, 12], [20.2, 12], "both"),
    line([17.4, 12], [17.4, 15.4], "both"),
    line([19.6, 12], [19.6, 14.4], "24"),
  ],
  "eye": [
    poly([[3.6, 12], [7.6, 7.8], [12, 6.6], [16.4, 7.8], [20.4, 12], [16.4, 16.2], [12, 17.4], [7.6, 16.2]], true, true, "both"),
    circle([12, 12], 2.3, true, "both"),
  ],
  "eye-off": [
    poly([[3.6, 12], [7.6, 7.8], [12, 6.6], [16.4, 7.8], [20.4, 12], [16.4, 16.2], [12, 17.4], [7.6, 16.2]], true, true, "both"),
    line([5, 19], [19, 5], "both"),
  ],
  "copy": [
    rect(8.2, 3.8, 11, 11, 1.3, true, "both"),
    rect(4.8, 8.6, 11, 11.4, 1.3, true, "both"),
  ],
  "trash": [
    line([8.4, 6], [15.6, 6], "both"),
    line([10.4, 6], [10.4, 4.4], "both"),
    line([13.6, 6], [13.6, 4.4], "both"),
    poly([[6.4, 6], [17.6, 6], [16.6, 20], [7.4, 20]], true, true, "both"),
  ],
  "refresh": [
    poly([[16.8, 8.2], [19.2, 8.2], [19.2, 5.6]], false, false, "both"),
    poly([[7.4, 7.2], [12, 4.6], [17.2, 7.4]], false, false, "both"),
    poly([[7.2, 15.8], [4.8, 15.8], [4.8, 18.4]], false, false, "both"),
    poly([[16.6, 16.8], [12, 19.4], [6.8, 16.6]], false, false, "both"),
  ],
  "rotate-cw": [
    poly([[16.8, 8], [19.4, 8], [19.4, 5.4]], false, false, "both"),
    poly([[7.2, 7.4], [12, 4.6], [17.4, 7.6]], false, false, "both"),
    circle([12, 13.2], 5.4, false, "24"),
  ],
  "rotate-ccw": [
    poly([[7.2, 8], [4.6, 8], [4.6, 5.4]], false, false, "both"),
    poly([[16.8, 7.4], [12, 4.6], [6.6, 7.6]], false, false, "both"),
    circle([12, 13.2], 5.4, false, "24"),
  ],
  "loader": [
    poly([[12, 4.6], [12, 8.2]], false, false, "both"),
    poly([[18.6, 7.4], [16.4, 9.6]], false, false, "24"),
    poly([[19.4, 12], [16, 12]], false, false, "both"),
  ],
  "play": [
    poly([[8, 6], [8, 18], [18.5, 12]], true, true, "both"),
  ],
  "square": [
    rect(6.5, 6.5, 11, 11, 1.4, true, "both"),
  ],
  "circle": [
    circle([12, 12], 7.2, false, "both"),
  ],
  "circle-check": [
    circle([12, 12], 8.1, false, "both"),
    poly([[8, 12.2], [11, 15.4], [16.6, 9.2]], false, false, "both"),
  ],
  "circle-x": [
    circle([12, 12], 8.1, false, "both"),
    line([9, 9], [15, 15], "both"),
    line([15, 9], [9, 15], "both"),
  ],
  "circle-alert": [
    circle([12, 12], 8.1, false, "both"),
    line([12, 7.4], [12, 13.2], "both"),
    circle([12, 16.4], 0.85, true, "both"),
  ],
  "circle-slash": [
    circle([12, 12], 8.1, false, "both"),
    line([7.2, 16.8], [16.8, 7.2], "both"),
  ],
  "triangle-alert": [
    poly([[12, 4.6], [20.6, 19.2], [3.4, 19.2]], true, true, "both"),
    line([12, 9.6], [12, 14.2], "both"),
    circle([12, 16.6], 0.8, true, "both"),
  ],
  "octagon-x": [
    poly([[8.4, 4.4], [15.6, 4.4], [19.6, 8.4], [19.6, 15.6], [15.6, 19.6], [8.4, 19.6], [4.4, 15.6], [4.4, 8.4]], true, true, "both"),
    line([9, 9], [15, 15], "both"),
    line([15, 9], [9, 15], "both"),
  ],
  "ban": [
    circle([12, 12], 8.1, false, "both"),
    line([7.2, 16.8], [16.8, 7.2], "both"),
  ],
  "shield-check": [
    poly([[12, 3.8], [19.2, 6.4], [18.4, 13.8], [12, 20.2], [5.6, 13.8], [4.8, 6.4]], true, true, "both"),
    poly([[8.6, 12.2], [11.2, 15], [16, 9.4]], false, false, "both"),
  ],
  "shield-off": [
    poly([[12, 3.8], [19.2, 6.4], [18.4, 13.8], [12, 20.2], [5.6, 13.8], [4.8, 6.4]], true, true, "both"),
    line([5, 19], [19, 5], "both"),
  ],
  "image": [
    rect(4, 5.5, 16, 13, 1.4, true, "both"),
    poly([[6, 16.2], [10.2, 11.2], [13, 14], [15.4, 11.8], [18.2, 16.2]], true, true, "both"),
    circle([9.2, 9], 1.15, true, "both"),
  ],
  "video": [
    rect(3.6, 7, 12.6, 10, 1.3, true, "both"),
    poly([[16.4, 9.2], [20.6, 7.2], [20.6, 16.8], [16.4, 14.8]], true, true, "both"),
  ],
  "maximize": [
    poly([[5, 9.2], [5, 5], [9.2, 5]], false, false, "both"),
    poly([[14.8, 5], [19, 5], [19, 9.2]], false, false, "both"),
    poly([[5, 14.8], [5, 19], [9.2, 19]], false, false, "both"),
    poly([[14.8, 19], [19, 19], [19, 14.8]], false, false, "both"),
  ],
  "minimize": [
    poly([[5, 9.2], [9.2, 9.2], [9.2, 5]], false, false, "both"),
    poly([[14.8, 5], [14.8, 9.2], [19, 9.2]], false, false, "both"),
    poly([[5, 14.8], [9.2, 14.8], [9.2, 19]], false, false, "both"),
    poly([[14.8, 19], [14.8, 14.8], [19, 14.8]], false, false, "both"),
  ],
  "zoom-in": [
    circle([10.4, 10.4], 5.1, false, "both"),
    line([14.2, 14.2], [19.4, 19.4], "both"),
    line([10.4, 8], [10.4, 12.8], "both"),
    line([8, 10.4], [12.8, 10.4], "both"),
  ],
  "zoom-out": [
    circle([10.4, 10.4], 5.1, false, "both"),
    line([14.2, 14.2], [19.4, 19.4], "both"),
    line([8, 10.4], [12.8, 10.4], "both"),
  ],
  "scan": [
    poly([[5, 9], [5, 5], [9, 5]], false, false, "both"),
    poly([[15, 5], [19, 5], [19, 9]], false, false, "both"),
    poly([[5, 15], [5, 19], [9, 19]], false, false, "both"),
    poly([[15, 19], [19, 19], [19, 15]], false, false, "both"),
  ],
  "pencil": [
    poly([[14.8, 4.6], [19.4, 9.2], [8.6, 20], [4, 20], [4, 15.4]], true, true, "both"),
    line([13.4, 6.2], [17.8, 10.6], "both"),
  ],
  "save": [
    poly([[5, 4.8], [16.4, 4.8], [19, 7.4], [19, 19.2], [5, 19.2]], true, true, "both"),
    rect(8, 4.8, 6.4, 4.4, 0.6, true, "both"),
    rect(8.2, 12.4, 7.6, 6.8, 0.8, true, "both"),
  ],
  "wrap-text": [
    line([4.5, 7], [19.5, 7], "both"),
    line([4.5, 12], [14.5, 12], "both"),
    poly([[14.5, 12], [18.6, 12], [18.6, 16.4], [15.2, 16.4]], false, false, "both"),
    poly([[16.8, 14.6], [15, 16.6], [16.8, 18.6]], false, false, "both"),
  ],
  "list-filter": [
    line([4.5, 7], [19.5, 7], "both"),
    line([7, 12], [17, 12], "both"),
    line([9.5, 17], [14.5, 17], "both"),
  ],
  "list-checks": [
    poly([[4.4, 7.2], [6.4, 9.2], [9.4, 5.8]], false, false, "both"),
    line([11.2, 7.4], [20, 7.4], "both"),
    poly([[4.4, 14.8], [6.4, 16.8], [9.4, 13.4]], false, false, "both"),
    line([11.2, 15], [20, 15], "both"),
  ],
  "link": [
    poly([[9.2, 14.8], [7.2, 16.8], [5.6, 15.2], [5.6, 12.4], [8.4, 9.6], [11.2, 9.6], [13, 11.4]], false, false, "both"),
    poly([[14.8, 9.2], [16.8, 7.2], [18.4, 8.8], [18.4, 11.6], [15.6, 14.4], [12.8, 14.4], [11, 12.6]], false, false, "both"),
  ],
  "upload": [
    line([12, 19.2], [12, 6.4], "both"),
    poly([[7.4, 10.6], [12, 5.8], [16.6, 10.6]], false, false, "both"),
    line([5, 19.2], [19, 19.2], "both"),
  ],
  "globe": [
    circle([12, 12], 8.1, false, "both"),
    line([12, 4], [12, 20], "both"),
    poly([[4.4, 12], [19.6, 12]], false, false, "both"),
    poly([[6.2, 8], [12, 7.2], [17.8, 8]], false, false, "24"),
    poly([[6.2, 16], [12, 16.8], [17.8, 16]], false, false, "24"),
  ],
  "terminal": [
    rect(3.6, 5.2, 16.8, 13.6, 1.4, true, "both"),
    poly([[6.4, 9], [9.6, 11.6], [6.4, 14.2]], false, false, "both"),
    line([11.2, 14.6], [16.8, 14.6], "both"),
  ],
  "terminal-square": [
    rect(4.2, 4.2, 15.6, 15.6, 1.6, true, "both"),
    poly([[7, 9.2], [10.2, 12], [7, 14.8]], false, false, "both"),
    line([11.6, 15.2], [16.4, 15.2], "both"),
  ],
  "code": [
    poly([[8.6, 6.5], [4.6, 12], [8.6, 17.5]], false, false, "both"),
    poly([[15.4, 6.5], [19.4, 12], [15.4, 17.5]], false, false, "both"),
  ],
  "workflow": [
    circle([6.2, 7.2], 2.3, true, "both"),
    circle([17.8, 7.2], 2.3, true, "both"),
    circle([12, 16.8], 2.3, true, "both"),
    line([8.4, 7.2], [15.6, 7.2], "both"),
    line([6.2, 9.5], [6.2, 16.8], "both"),
    line([12, 14.5], [12, 9.5], "24"),
  ],
  "zap": [
    poly([[13.6, 3.8], [7.2, 13.2], [11.6, 13.2], [10.4, 20.2], [17.2, 10.4], [12.8, 10.4]], true, true, "both"),
  ],
  "power": [
    circle([12, 13.2], 7.2, false, "both"),
    line([12, 4.6], [12, 12], "both"),
  ],
};

export function awakenIconDrawing(id: AwakenIconId): { ops: readonly AwakenIconOp[] } {
  const ops = AWAKEN_ICON_DRAWINGS[id];
  if (!ops) {
    throw new Error(`Missing Awaken icon drawing: ${id}`);
  }
  return { ops };
}
