/**
 * Awaken 图标复用原语。
 *
 * 每个反复出现的形体在这里只有一份实现，图标是原语的组合而不是逐个重画。
 * 十个图标里的文件夹是同一个文件夹，这件事由这一层强制，不靠记性。
 *
 * 所有原语都在 16 网格的 stroke-center 坐标系里定义，最终位置与尺寸
 * 由 geometry.ts 的光学归一化统一决定，因此这里的坐标只需保证比例正确。
 */
import { type GeomOp, type Vec, poly, circle } from "./geometry";

/**
 * 文件夹容器。tabHeight 同时是页签台阶高度和 45 度斜边长度。
 *
 * 页签必须保持 3 单位左右的台阶：实测 2 单位时会被 1.35 的圆角吃掉，
 * 12px 下整个图标糊成一团。
 */
export function folderBody(
  tabWidth = 5.2,
  tabHeight = 3,
  width = 14,
  height = 10.7,
): GeomOp {
  const pts: Vec[] = [
    [0, 0],
    [tabWidth, 0],
    [tabWidth + tabHeight, tabHeight],
    [width, tabHeight],
    [width, height],
    [0, height],
  ];
  return poly(pts, [1.35, 0.9, 0.6, 1.35, 1.35, 1.35], true);
}

/** 文件主体：右上角折角。折角边长同时是 45 度斜边。 */
export function fileBody(width = 11, height = 13.4, fold = 3.6): GeomOp {
  const pts: Vec[] = [
    [0, 0],
    [width - fold, 0],
    [width, fold],
    [width, height],
    [0, height],
  ];
  return poly(pts, [1.35, 0.5, 1.35, 1.35, 1.35], true);
}

/** 加号修饰符。全套的 plus 只有这一个画法。 */
export function plusMark(cx: number, cy: number, arm: number): GeomOp[] {
  return [
    poly(
      [
        [cx - arm, cy],
        [cx + arm, cy],
      ],
      0,
    ),
    poly(
      [
        [cx, cy - arm],
        [cx, cy + arm],
      ],
      0,
    ),
  ];
}

/** 减号修饰符。 */
export function minusMark(cx: number, cy: number, arm: number): GeomOp {
  return poly(
    [
      [cx - arm, cy],
      [cx + arm, cy],
    ],
    0,
  );
}

/** 叉号修饰符。 */
export function crossMark(cx: number, cy: number, arm: number): GeomOp[] {
  return [
    poly(
      [
        [cx - arm, cy - arm],
        [cx + arm, cy + arm],
      ],
      0,
    ),
    poly(
      [
        [cx + arm, cy - arm],
        [cx - arm, cy + arm],
      ],
      0,
    ),
  ];
}

/** 折叠箭头。严格 45 度，顶点单独倒角。 */
export function chevron(size = 5.25): GeomOp {
  return poly(
    [
      [0, 0],
      [size, size],
      [0, size * 2],
    ],
    [0, 1, 0],
  );
}

/**
 * 尖顶正六边形 —— Awaken「可装配单元」形状族的基型。
 *
 * 全套里唯一的六边形，因此不会和方框族、文件族混淆。
 * 这个外形与 Agent Skills 生态的事实标记一致（见 brandMarks.ts 的说明）。
 * 长宽比固定为 cos30 ≈ 0.866，落在 Circle 与 Vertical 光学形状之间，
 * 所以该族一律用 radial 归一化，才能与圆形族等视觉重量。
 *
 * 圆角必须很小：实测 1.15 时 12–16px 下六个角被磨圆，整个形状读成圆形，
 * 和同套里的 `circle` 撞车。0.45 能让六条边在 12px 仍然是直的。
 */
export function hexagon(r = 6.375, cx = 8, cy = 8): GeomOp {
  const pts = [-90, -30, 30, 90, 150, 210].map((deg): Vec => {
    const a = (deg * Math.PI) / 180;
    return [cx + r * Math.cos(a), cy + r * Math.sin(a)];
  });
  return poly(pts, 0.45, true);
}

/** 六边形族的内核标记，用来区分族内不同成员。 */
export function hexagonCore(radius = 1.55, cx = 8, cy = 8): GeomOp {
  return circle(cx, cy, radius);
}

/**
 * 六边形族的插座内标：两条横向槽。
 *
 * 不用加号 —— 实测「六边形 + 加号」被读成「添加」，和 plus 撞车。
 * 也不用两根竖线 —— 那是媒体「暂停」的通用符号。横向双槽读作插座开口。
 */
export function hexagonSlots(cx = 8, cy = 8, half = 1.85, gap = 1.3): GeomOp[] {
  return [
    poly(
      [
        [cx - half, cy - gap],
        [cx + half, cy - gap],
      ],
      0,
    ),
    poly(
      [
        [cx - half, cy + gap],
        [cx + half, cy + gap],
      ],
      0,
    ),
  ];
}

/**
 * 六边形族的挂载内标：一根竖干加一段短脚，表示「挂在事件上」。
 *
 * 短脚明显短于竖干，避免读成字母 L。
 */
export function hexagonHook(cx = 8, cy = 8): GeomOp {
  return poly(
    [
      [cx - 0.8, cy - 2.1],
      [cx - 0.8, cy + 1.6],
      [cx + 1.9, cy + 1.6],
    ],
    [0, 1.1, 0],
  );
}

/**
 * 齿轮。8 齿而不是 6 齿：六重对称无法让齿同时落在水平与垂直轴上，
 * 轮廓会是六边形而不是正圆（实测 6 齿是 12.3 宽 × 14 高）。
 * 中心孔必须占外径 40% 以上，否则 12px 下先糊掉的就是它。
 */
export function gear(outer = 6.3, inner = 4.6, hub = 2.6, cx = 8, cy = 8): GeomOp[] {
  const pts: Vec[] = [];
  const radii: number[] = [];
  for (let i = 0; i < 16; i += 1) {
    const a = ((-90 + i * 22.5) * Math.PI) / 180;
    const r = i % 2 === 0 ? outer : inner;
    pts.push([cx + r * Math.cos(a), cy + r * Math.sin(a)]);
    radii.push(i % 2 === 0 ? 0.5 : 0.62);
  }
  return [poly(pts, radii, true), circle(cx, cy, hub)];
}
