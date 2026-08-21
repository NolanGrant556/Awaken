/**
 * Awaken 图标几何内核。
 *
 * 这一层是全套图标唯一的几何来源，它把「不可违反的构造约束」变成代码而不是约定：
 *
 * 1. 16 单位网格，所有坐标都是 stroke-center 坐标。
 * 2. 基本线只允许 0 / 45 / 90 度；其它角度必须由概念本身要求（例如齿轮的齿）。
 * 3. 先画直线，再倒圆角。不使用自由曲线，也不从圆上截取曲线。
 * 4. 圆头端点、圆角接合，全套统一。
 * 5. 每个图标声明一个光学形状，视觉包围盒被归一化到该形状尺寸，
 *    这样不同外形的图标在同一屏里视觉重量一致。
 * 6. 重叠元素之间的 stroke-center 净间隙不小于 3 个网格单位。
 *
 * 描边宽度是几何的一部分而不是可换肤 token：它参与光学归一化，
 * 改它会同时改变每个图标的视觉尺寸。因此它是这里的常量。
 */

/** 16 网格上线性图标的描边宽度。改这个值会改变全套图标的视觉尺寸。 */
export const AWAKEN_ICON_GRID = 16;
export const AWAKEN_ICON_STROKE = 1.25;

/** 光学形状的视觉尺寸（含描边），16 网格。 */
export const AWAKEN_OPTICAL_SHAPES = {
  square: [13, 13],
  circle: [14, 14],
  horizontal: [14, 11],
  vertical: [11, 14],
} as const;

export type AwakenOpticalShape = keyof typeof AWAKEN_OPTICAL_SHAPES;

export type Vec = readonly [number, number];

export type GeomOp =
  | { readonly op: "poly"; readonly pts: readonly Vec[]; readonly radii: readonly number[]; readonly closed: boolean; readonly measure: boolean }
  | { readonly op: "circle"; readonly c: Vec; readonly r: number; readonly measure: boolean }
  | { readonly op: "arc"; readonly c: Vec; readonly r: number; readonly a0: number; readonly a1: number; readonly measure: boolean }
  | {
      readonly op: "arrow";
      readonly c: Vec;
      readonly r: number;
      readonly angle: number;
      readonly forward: 1 | -1;
      readonly length: number;
      readonly spread: number;
      readonly measure: boolean;
    };

/** 归一化的适配方式：bbox 按包围盒，radial 按外接圆（圆形族用它才能等重量）。 */
export type AwakenIconFit = "bbox" | "radial";

export function poly(
  pts: readonly Vec[],
  radii: readonly number[] | number,
  closed = false,
  measure = true,
): GeomOp {
  const r = typeof radii === "number" ? pts.map(() => radii) : radii;
  return { op: "poly", pts, radii: r, closed, measure };
}

export function circle(cx: number, cy: number, r: number, measure = true): GeomOp {
  return { op: "circle", c: [cx, cy], r, measure };
}

export function arc(cx: number, cy: number, r: number, a0: number, a1: number, measure = true): GeomOp {
  return { op: "arc", c: [cx, cy], r, a0, a1, measure };
}

/**
 * 圆弧端点上的两段式箭头。默认 measure=false —— 箭头按设计允许溢出光学盒，
 * 类比字体里逗号可以下沉到基线以下。
 */
export function arrow(
  cx: number,
  cy: number,
  r: number,
  angle: number,
  forward: 1 | -1 = 1,
  length = 3.1,
  spread = 32,
  measure = false,
): GeomOp {
  return { op: "arrow", c: [cx, cy], r, angle, forward, length, spread, measure };
}

/**
 * 按弦与拱高构造圆弧。
 *
 * 少数概念本质上就是圆弧：眼睛的上下眼睑、月牙、信号弧。它们无法用直线倒角表达。
 * 这个辅助让它们仍然是「从圆上截取」而不是自由曲线：给定两端点与拱高，
 * 反解出唯一的圆心与半径，因此同一族的弧曲率可以精确复用。
 */
export function arcThroughChord(
  from: Vec,
  to: Vec,
  sagitta: number,
  side: "left" | "right" = "left",
): GeomOp {
  const mx = (from[0] + to[0]) / 2;
  const my = (from[1] + to[1]) / 2;
  const dx = to[0] - from[0];
  const dy = to[1] - from[1];
  const chord = Math.hypot(dx, dy);
  const half = chord / 2;
  const r = (half * half + sagitta * sagitta) / (2 * sagitta);
  const sign = side === "left" ? 1 : -1;
  const nx = (-dy / chord) * sign;
  const ny = (dx / chord) * sign;
  const cx = mx - nx * (r - sagitta);
  const cy = my - ny * (r - sagitta);
  const deg = (v: number) => (v * 180) / Math.PI;
  const a0 = deg(Math.atan2(from[1] - cy, from[0] - cx));
  const a1raw = deg(Math.atan2(to[1] - cy, to[0] - cx));
  const peak = deg(Math.atan2(my + ny * sagitta - cy, mx + nx * sagitta - cx));
  const norm = (v: number) => ((v % 360) + 360) % 360;
  // 两个方向都试，取真正经过拱点的那一段。拱点按构造就是弧的中点，
  // 因此「经过拱点」唯一确定了应该走哪一边。
  const forward = norm(a1raw - a0);
  const backward = norm(a0 - a1raw);
  const peakForward = norm(peak - a0);
  const a1 = peakForward <= forward ? a0 + forward : a0 - backward;
  return arc(cx, cy, r, a0, a1);
}

export function rotate(pts: readonly Vec[], deg: number, about: Vec = [0, 0]): Vec[] {
  const a = (deg * Math.PI) / 180;
  const ca = Math.cos(a);
  const sa = Math.sin(a);
  return pts.map(([x, y]) => {
    const dx = x - about[0];
    const dy = y - about[1];
    return [about[0] + dx * ca - dy * sa, about[1] + dx * sa + dy * ca] as Vec;
  });
}

function arrowPoints(o: Extract<GeomOp, { op: "arrow" }>): Vec[] {
  const a = (o.angle * Math.PI) / 180;
  const tip: Vec = [o.c[0] + o.r * Math.cos(a), o.c[1] + o.r * Math.sin(a)];
  const tx = -Math.sin(a) * o.forward;
  const ty = Math.cos(a) * o.forward;
  const back = Math.atan2(-ty, -tx);
  const out: Vec[] = [tip];
  for (const sign of [1, -1]) {
    const b = back + (o.spread * sign * Math.PI) / 180;
    out.push([tip[0] + Math.cos(b) * o.length, tip[1] + Math.sin(b) * o.length]);
  }
  return out;
}

function samplePoints(ops: readonly GeomOp[], onlyMeasured: boolean): Vec[] {
  const pts: Vec[] = [];
  for (const o of ops) {
    if (onlyMeasured && !o.measure) continue;
    if (o.op === "poly") {
      pts.push(...o.pts);
    } else if (o.op === "circle") {
      for (let i = 0; i < 48; i += 1) {
        const a = (Math.PI * 2 * i) / 48;
        pts.push([o.c[0] + o.r * Math.cos(a), o.c[1] + o.r * Math.sin(a)]);
      }
    } else if (o.op === "arc") {
      const steps = Math.max(6, Math.round(Math.abs(o.a1 - o.a0) / 6));
      for (let i = 0; i <= steps; i += 1) {
        const a = ((o.a0 + ((o.a1 - o.a0) * i) / steps) * Math.PI) / 180;
        pts.push([o.c[0] + o.r * Math.cos(a), o.c[1] + o.r * Math.sin(a)]);
      }
    } else {
      pts.push(...arrowPoints(o));
    }
  }
  return pts;
}

function bounds(pts: readonly Vec[]) {
  let x0 = Infinity;
  let y0 = Infinity;
  let x1 = -Infinity;
  let y1 = -Infinity;
  for (const [x, y] of pts) {
    if (x < x0) x0 = x;
    if (y < y0) y0 = y;
    if (x > x1) x1 = x;
    if (y > y1) y1 = y;
  }
  return { x0, y0, x1, y1, w: x1 - x0, h: y1 - y0, cx: (x0 + x1) / 2, cy: (y0 + y1) / 2 };
}

function scaleOps(ops: readonly GeomOp[], s: number, dx: number, dy: number): GeomOp[] {
  const p = ([x, y]: Vec): Vec => [x * s + dx, y * s + dy];
  return ops.map((o) => {
    if (o.op === "poly") return { ...o, pts: o.pts.map(p), radii: o.radii.map((r) => r * s) };
    if (o.op === "circle") return { ...o, c: p(o.c), r: o.r * s };
    if (o.op === "arc") return { ...o, c: p(o.c), r: o.r * s };
    return { ...o, c: p(o.c), r: o.r * s, length: o.length * s };
  });
}

/**
 * 把几何归一化到声明的光学形状：等比缩放并居中，使视觉包围盒贴合目标尺寸。
 * 返回归一化后的 ops 与实测视觉尺寸（供审计测试核对）。
 */
export function normalizeToOptical(
  ops: readonly GeomOp[],
  visual: readonly [number, number],
  fit: AwakenIconFit,
  stroke: number = AWAKEN_ICON_STROKE,
): { ops: GeomOp[]; measured: [number, number] } {
  const measured = samplePoints(ops, true);
  const b = bounds(measured);
  const targetW = visual[0] - stroke;
  const targetH = visual[1] - stroke;

  let s: number;
  let anchorX = b.cx;
  let anchorY = b.cy;
  if (fit === "radial") {
    // 圆形族按外接圆归一化。锚点必须是圆本身的圆心，不能用包围盒中心 ——
    // 缺口圆弧（如 spinner）的包围盒并不以圆心为中心，用包围盒中心会算错半径。
    const round = ops.find((o) => (o.op === "circle" || o.op === "arc") && o.measure);
    if (round && (round.op === "circle" || round.op === "arc")) {
      anchorX = round.c[0];
      anchorY = round.c[1];
    }
    let rMax = 0;
    for (const [x, y] of measured) {
      rMax = Math.max(rMax, Math.hypot(x - anchorX, y - anchorY));
    }
    s = rMax > 1e-9 ? Math.min(targetW, targetH) / 2 / rMax : 1;
  } else {
    const sw = b.w > 1e-9 ? targetW / b.w : Infinity;
    const sh = b.h > 1e-9 ? targetH / b.h : Infinity;
    s = Math.min(sw, sh);
  }

  const center = AWAKEN_ICON_GRID / 2;
  const out = scaleOps(ops, s, center - anchorX * s, center - anchorY * s);
  const nb = bounds(samplePoints(out, true));
  return { ops: out, measured: [nb.w + stroke, nb.h + stroke] };
}

const fmt = (v: number): string => {
  const r = Math.round(v * 1000) / 1000;
  return Object.is(r, -0) ? "0" : String(r);
};

function norm(v: Vec): Vec {
  const m = Math.hypot(v[0], v[1]);
  return m ? [v[0] / m, v[1] / m] : [0, 0];
}

/** 折线/多边形按顶点半径倒圆角。半径过大时按边长自动回退。 */
function roundedPath(pts: readonly Vec[], radii: readonly number[], closed: boolean): string {
  const n = pts.length;
  if (n < 2) return "";

  type Corner = { t1: Vec; t2: Vec; r: number; sweep: 0 | 1 };
  const corners = new Map<number, Corner>();
  const indices = closed ? [...Array(n).keys()] : [...Array(n - 2).keys()].map((i) => i + 1);

  for (const i of indices) {
    const p = pts[(i - 1 + n) % n];
    const c = pts[i];
    const q = pts[(i + 1) % n];
    const e1: Vec = [p[0] - c[0], p[1] - c[1]];
    const e2: Vec = [q[0] - c[0], q[1] - c[1]];
    const l1 = Math.hypot(e1[0], e1[1]);
    const l2 = Math.hypot(e2[0], e2[1]);
    if (l1 < 1e-9 || l2 < 1e-9) continue;
    const u1 = norm(e1);
    const u2 = norm(e2);
    const cosang = Math.max(-1, Math.min(1, u1[0] * u2[0] + u1[1] * u2[1]));
    const ang = Math.acos(cosang);
    if (ang < 1e-6 || Math.abs(ang - Math.PI) < 1e-6) continue;
    let r = radii[i] ?? 0;
    if (r <= 0) continue;
    const half = Math.tan(ang / 2);
    let d = r / half;
    const dMax = Math.min(l1, l2) / 2;
    if (d > dMax) {
      d = dMax;
      r = d * half;
    }
    const cross = e1[0] * e2[1] - e1[1] * e2[0];
    corners.set(i, {
      t1: [c[0] + u1[0] * d, c[1] + u1[1] * d],
      t2: [c[0] + u2[0] * d, c[1] + u2[1] * d],
      r,
      sweep: cross > 0 ? 0 : 1,
    });
  }

  // 闭合路径必须从顶点 0 的「出边」切点起笔。用入边切点 t1 起笔会让最后的 Z
  // 沿着顶点 0 的弦直接闭合，把这个角切掉 —— 描边形状上表现为第一个角变平，
  // 实心形状上表现为缺一块。
  const start = closed && corners.has(0) ? corners.get(0)!.t2 : pts[0];
  const seg: string[] = [`M${fmt(start[0])} ${fmt(start[1])}`];
  const order = closed ? [...Array(n).keys()].map((i) => i + 1) : [...Array(n - 1).keys()].map((i) => i + 1);

  for (const k of order) {
    const i = k % n;
    const corner = corners.get(i);
    if (corner) {
      seg.push(`L${fmt(corner.t1[0])} ${fmt(corner.t1[1])}`);
      seg.push(
        `A${fmt(corner.r)} ${fmt(corner.r)} 0 0 ${corner.sweep} ${fmt(corner.t2[0])} ${fmt(corner.t2[1])}`,
      );
    } else {
      seg.push(`L${fmt(pts[i][0])} ${fmt(pts[i][1])}`);
    }
  }
  if (closed) seg.push("Z");
  return seg.join("");
}

/** 把 ops 编译成 SVG path 的 d 字符串列表。 */
export function emitPaths(ops: readonly GeomOp[]): string[] {
  const out: string[] = [];
  for (const o of ops) {
    if (o.op === "poly") {
      out.push(roundedPath(o.pts, o.radii, o.closed));
    } else if (o.op === "circle") {
      const { c, r } = o;
      out.push(
        `M${fmt(c[0] - r)} ${fmt(c[1])}A${fmt(r)} ${fmt(r)} 0 0 1 ${fmt(c[0] + r)} ${fmt(c[1])}` +
          `A${fmt(r)} ${fmt(r)} 0 0 1 ${fmt(c[0] - r)} ${fmt(c[1])}Z`,
      );
    } else if (o.op === "arc") {
      const { c, r, a0, a1 } = o;
      const r0 = (a0 * Math.PI) / 180;
      const r1 = (a1 * Math.PI) / 180;
      const large = Math.abs(a1 - a0) > 180 ? 1 : 0;
      const sweep = a1 > a0 ? 1 : 0;
      out.push(
        `M${fmt(c[0] + r * Math.cos(r0))} ${fmt(c[1] + r * Math.sin(r0))}` +
          `A${fmt(r)} ${fmt(r)} 0 ${large} ${sweep} ${fmt(c[0] + r * Math.cos(r1))} ${fmt(c[1] + r * Math.sin(r1))}`,
      );
    } else {
      const [tip, b1, b2] = arrowPoints(o);
      out.push(`M${fmt(b1[0])} ${fmt(b1[1])}L${fmt(tip[0])} ${fmt(tip[1])}L${fmt(b2[0])} ${fmt(b2[1])}`);
    }
  }
  return out;
}

/** 最小净间隙检查：返回两组 op 的 stroke-center 最近距离，用于审计 ≥3 单位的规则。 */
export function minGap(a: readonly GeomOp[], b: readonly GeomOp[]): number {
  const pa = samplePoints(a, false);
  const pb = samplePoints(b, false);
  let best = Infinity;
  for (const p of pa) {
    for (const q of pb) {
      best = Math.min(best, Math.hypot(p[0] - q[0], p[1] - q[1]));
    }
  }
  return best;
}
