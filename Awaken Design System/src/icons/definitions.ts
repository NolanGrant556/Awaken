/**
 * Awaken 原创线性图标定义。
 *
 * 采用「精密」声音：描边 1.25、小圆角、线延伸到光学边界、闭合优先、
 * 交汇处留光学缺口。这套参数是在 12/13/14/16px 真实渲染下逐轮筛出来的，
 * 每条注释里的「实测」都指真实抗锯齿结果，不是推断。
 *
 * 每个定义只声明几何与光学形状；最终尺寸、居中和 path 编译由 geometry.ts 负责。
 */
import {
  AWAKEN_OPTICAL_SHAPES,
  type AwakenIconFit,
  type AwakenOpticalShape,
  type GeomOp,
  type Vec,
  arc,
  arrow,
  circle,
  poly,
  rotate,
} from "./geometry";
import {
  chevron,
  crossMark,
  folderBody,
  gear,
  hexagon,
  hexagonCore,
  hexagonHook,
  hexagonSlots,
  minusMark,
  plusMark,
} from "./primitives";

export type IconDefinition = {
  readonly shape: AwakenOpticalShape;
  /** 覆盖光学形状的默认视觉尺寸。仅在概念本身不属于四种形状时使用。 */
  readonly visual?: readonly [number, number];
  readonly fit?: AwakenIconFit;
  readonly build: () => GeomOp[];
  /**
   * 可读下限。默认 12 —— 全套的设计目标就是 12px 可读。
   * 内标密度高的图标（六边形族带内标的成员）实测要到 16px 才分得开，
   * 由 registry 暴露给审计测试与调用点检查。
   */
  readonly minSize?: number;
  /** 该图标在产品中的用处。concepts.ts 负责概念到名字的映射，这里只描述图形本身。 */
  readonly purpose: string;
};

const SQRT_HALF = 1 / Math.SQRT2;

const RAW_DEFINITIONS = {
  // ---- 容器族 ----
  folder: {
    shape: "horizontal",
    purpose: "文件夹 / 项目",
    build: () => [folderBody()],
  },
  "folder-plus": {
    // 修饰符改变容器比例：容器从 14×11 变成 12.4×12，加号才有净空。
    shape: "square",
    purpose: "新建 / 添加文件夹或项目",
    build: () => [folderBody(5, 2.9, 12.4, 12), ...plusMark(6.2, 7.45, 2.35)],
  },
  "folder-open": {
    shape: "horizontal",
    purpose: "展开中的文件夹",
    build: () => [
      poly(
        [
          [0, 0],
          [5.2, 0],
          [8.2, 3],
          [12.6, 3],
          [12.6, 5.1],
          [1.6, 5.1],
          [0, 10.7],
        ],
        [1.35, 0.9, 0.6, 1.1, 0.8, 0.8, 1.1],
        false,
      ),
      poly(
        [
          [0, 10.7],
          [14, 10.7],
          [15.4, 5.1],
          [1.6, 5.1],
        ],
        [1.1, 1.1, 0.8, 0.8],
        false,
      ),
    ],
  },

  // ---- 圆形族 ----
  "magnifying-glass": {
    shape: "circle",
    purpose: "搜索",
    build: () => {
      const r = 4.35;
      const lens: Vec = [6.25, 6.25];
      // 光学缺口：手柄从镜片描边外 0.6 单位起笔，避免交汇处淤积成一团黑。
      const start: Vec = [lens[0] + (r + 0.6) * SQRT_HALF, lens[1] + (r + 0.6) * SQRT_HALF];
      return [circle(lens[0], lens[1], r), poly([start, [13.7, 13.7]], 0)];
    },
  },
  cog: {
    shape: "circle",
    fit: "radial",
    purpose: "设置",
    build: () => gear(),
  },
  "arrows-cw": {
    shape: "circle",
    fit: "radial",
    purpose: "刷新 / 重新加载",
    build: () => {
      const r = 5.3;
      // 缺口中心落在 45° / 225°，即 Awaken 指针的对角线方向。
      const gap = 26;
      return [
        arc(8, 8, r, 45 + gap, 225 - gap),
        arrow(8, 8, r, 225 - gap, 1, 3.1, 32),
        arc(8, 8, r, 225 + gap, 405 - gap),
        arrow(8, 8, r, 405 - gap, 1, 3.1, 32),
      ];
    },
  },
  wrench: {
    shape: "circle",
    purpose: "工具",
    build: () => {
      // 在局部坐标系里画好再整体旋转 -45°：钳口落在右上，遵循指针方向规则。
      const local: Vec[] = [
        [0, -1.2],
        [5.4, -1.2],
        [7.05, -2.85],
        [10.4, -2.85],
        [10.4, -0.95],
        [8.15, -0.95],
        [8.15, 0.95],
        [10.4, 0.95],
        [10.4, 2.85],
        [7.05, 2.85],
        [5.4, 1.2],
        [0, 1.2],
      ];
      const radii = [1.2, 0.9, 0.75, 0.6, 0.45, 0.85, 0.85, 0.45, 0.6, 0.75, 0.9, 1.2];
      return [poly(rotate(local, -45), radii, true)];
    },
  },

  // ---- 六边形族：Awaken 可装配单元 ----
  skill: {
    shape: "circle",
    fit: "radial",
    purpose: "技能包（可装配单元基型）",
    build: () => [hexagon()],
  },
  "skill-core": {
    shape: "circle",
    fit: "radial",
    minSize: 16,
    purpose: "已激活的技能包",
    build: () => [hexagon(), hexagonCore()],
  },
  hook: {
    shape: "circle",
    fit: "radial",
    // 内标只有一个直角肘，14px 以下会与外形粘连，因此下限是 16。
    minSize: 16,
    purpose: "钩子（可装配单元）",
    build: () => [hexagon(), hexagonHook()],
  },
  connector: {
    shape: "circle",
    fit: "radial",
    minSize: 16,
    purpose: "连接器（可装配单元）",
    build: () => [hexagon(), ...hexagonSlots()],
  },

  // ---- 方形族 ----
  plus: {
    shape: "square",
    purpose: "新增",
    build: () => plusMark(8, 8, 5.875),
  },
  x: {
    // X 由两条对角线构成，视觉上比同尺寸方形更大，因此光学盒收到 11.2。
    shape: "square",
    visual: [11.2, 11.2],
    purpose: "关闭 / 移除",
    build: () => crossMark(8, 8, 4.975),
  },
  minus: {
    shape: "square",
    visual: [13, 2],
    purpose: "减少 / 折叠",
    build: () => [minusMark(8, 8, 5.875)],
  },

  // ---- 方向与状态 ----
  "chevron-right": {
    shape: "vertical",
    visual: [6.5, 11.8],
    purpose: "展开 / 下一级",
    build: () => [chevron()],
  },
  "chevron-down": {
    shape: "horizontal",
    visual: [11.8, 6.5],
    purpose: "下拉 / 展开",
    build: () => [poly(rotate([[0, 0], [5.25, 5.25], [0, 10.5]], 90, [2.625, 5.25]), [0, 1, 0])],
  },
  check: {
    shape: "horizontal",
    visual: [13.5, 9.5],
    purpose: "完成 / 选中",
    build: () => [
      poly(
        [
          [0, 4.2],
          [4.05, 8.25],
          [12.25, 0.05],
        ],
        [0, 1.2, 0],
      ),
    ],
  },
  circle: {
    shape: "circle",
    purpose: "待开始",
    build: () => [circle(8, 8, 6.375)],
  },
  checks: {
    // 两个勾错开叠放而不是交叉：交叉处在 14px 会淤积成一团黑。
    shape: "horizontal",
    visual: [13.5, 10],
    purpose: "全选 / 全部完成",
    build: () => [
      poly(
        [
          [0, 3.2],
          [3.1, 6.3],
          [7.6, 1.8],
        ],
        [0, 1.1, 0],
      ),
      poly(
        [
          [4, 6.6],
          [7.1, 9.7],
          [11.6, 5.2],
        ],
        [0, 1.1, 0],
      ),
    ],
  },
  "check-square": {
    shape: "square",
    minSize: 14,
    purpose: "多选模式",
    build: () => [
      poly(
        [
          [0, 0],
          [11.75, 0],
          [11.75, 11.75],
          [0, 11.75],
        ],
        1.9,
        true,
      ),
      poly(
        [
          [2.9, 5.6],
          [5.1, 7.8],
          [8.85, 4.05],
        ],
        [0, 1, 0],
      ),
    ],
  },

  // ---- 容器与删除 ----
  trash: {
    shape: "vertical",
    purpose: "删除",
    build: () => [
      // 盖沿延长到光学边界，是这套图标的等宽字体式签名。
      poly(
        [
          [0, 3],
          [9.2, 3],
        ],
        0,
      ),
      poly(
        [
          [2.95, 3],
          [2.95, 0.7],
          [6.25, 0.7],
          [6.25, 3],
        ],
        [0, 1, 1, 0],
      ),
      // 桶身开口在顶部由盖沿封住，因此轮廓读起来是闭合的。
      poly(
        [
          [1.2, 3],
          [1.95, 12.75],
          [7.25, 12.75],
          [8, 3],
        ],
        [0, 1.5, 1.5, 0],
      ),
    ],
  },

  clock: {
    shape: "circle",
    purpose: "时间 / 定时任务",
    build: () => [
      circle(8, 8, 6.375),
      // 指针指向 12 点与 4 点：两条针都落在允许的角度上，且不与表盘描边粘连。
      poly(
        [
          [8, 4.4],
          [8, 8],
          [10.9, 8],
        ],
        [0, 0.7, 0],
      ),
    ],
  },
  "chat-bubble-plus": {
    shape: "square",
    minSize: 14,
    purpose: "新建会话 / 线程",
    build: () => [
      // 气泡主体加左下尾巴；尾巴用 45 度斜边，和全套构造法一致。
      poly(
        [
          [0, 0],
          [12.6, 0],
          [12.6, 9.4],
          [3.6, 9.4],
          [0, 12.6],
        ],
        [1.9, 1.9, 1.9, 0.4, 0.4],
        true,
      ),
      ...plusMark(6.3, 4.7, 2.1),
    ],
  },
  "dots-horizontal": {
    shape: "horizontal",
    visual: [13, 2],
    purpose: "更多操作",
    build: () => {
      // 三个点用极短线段实现，靠圆头端点成为正圆，尺寸与描边严格一致。
      const dots: GeomOp[] = [];
      for (const x of [2.1, 8, 13.9]) {
        dots.push(poly([[x, 8], [x, 8]], 0));
      }
      return dots;
    },
  },
  "chevrons-up-down": {
    shape: "vertical",
    visual: [8.5, 13],
    purpose: "全部展开",
    build: () => [
      poly(
        [
          [0, 3.6],
          [3.6, 0],
          [7.2, 3.6],
        ],
        [0, 1, 0],
      ),
      poly(
        [
          [0, 8.2],
          [3.6, 11.8],
          [7.2, 8.2],
        ],
        [0, 1, 0],
      ),
    ],
  },
  "chevrons-down-up": {
    shape: "vertical",
    visual: [8.5, 13],
    purpose: "全部折叠",
    build: () => [
      poly(
        [
          [0, 0],
          [3.6, 3.6],
          [7.2, 0],
        ],
        [0, 1, 0],
      ),
      poly(
        [
          [0, 11.8],
          [3.6, 8.2],
          [7.2, 11.8],
        ],
        [0, 1, 0],
      ),
    ],
  },

  // ---- Awaken 专有概念 ----
  subagent: {
    shape: "horizontal",
    purpose: "子任务组：一个 run 分叉成并行子任务",
    build: () => [
      poly(
        [
          [0, 5.9],
          [2.4, 5.9],
        ],
        0,
      ),
      poly(
        [
          [2.4, 2.8],
          [2.4, 9],
        ],
        0,
      ),
      poly(
        [
          [2.4, 2.8],
          [5.4, 2.8],
        ],
        0,
      ),
      poly(
        [
          [2.4, 9],
          [5.4, 9],
        ],
        0,
      ),
      poly(
        [
          [5.4, 1.4],
          [11.75, 1.4],
          [11.75, 4.2],
          [5.4, 4.2],
        ],
        1.4,
        true,
      ),
      poly(
        [
          [5.4, 7.6],
          [11.75, 7.6],
          [11.75, 10.4],
          [5.4, 10.4],
        ],
        1.4,
        true,
      ),
    ],
  },
  /**
   * MCP 官方标记在 13/14px 无法读出（见 brandMarks.ts 的 minSize 说明），
   * 因此工具轨迹这类密集行需要一个 compact 伴随字形。
   * 这里不改动官方标记，而是用 Awaken 自己的几何表达「两个互扣的环」，
   * 保留互扣语义但把笔画减到 14px 能分辨的数量。
   */
  "mcp-compact": {
    shape: "circle",
    minSize: 14,
    purpose: "MCP 来源（14–16px 的 compact 伴随字形）",
    build: () => {
      const linkA: Vec[] = [
        [0, -2.15],
        [3.1, -2.15],
        [3.1, 2.15],
        [0, 2.15],
      ];
      const linkB: Vec[] = [
        [2.2, -2.15],
        [5.3, -2.15],
        [5.3, 2.15],
        [2.2, 2.15],
      ];
      return [
        poly(rotate(linkA, -45, [2.65, 0]), 1.6, true),
        poly(rotate(linkB, -45, [2.65, 0]), 1.6, true),
      ];
    },
  },
} as const satisfies Record<string, IconDefinition>;

export type AwakenLineIconName = keyof typeof RAW_DEFINITIONS;

/**
 * `as const satisfies` 会把每个条目窄化到它自己的字面量类型，可选字段在缺省的条目上
 * 直接从联合里消失。这里重新标注成统一的值类型，让 registry 与测试能安全读 fit / minSize，
 * 同时保留 key 的字面量类型用于 AwakenLineIconName。
 */
export const ICON_DEFINITIONS: Record<AwakenLineIconName, IconDefinition> = RAW_DEFINITIONS;

export function opticalVisual(def: IconDefinition): readonly [number, number] {
  return def.visual ?? AWAKEN_OPTICAL_SHAPES[def.shape];
}
