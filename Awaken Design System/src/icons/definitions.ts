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
  arcThroughChord,
  arrow,
  circle,
  poly,
  rotate,
} from "./geometry";
import {
  bracketPair,
  chevron,
  cornerBrackets,
  crescent,
  crossMark,
  display,
  exclamationMark,
  eye,
  fileBody,
  fileWithMark,
  folderBody,
  gear,
  globe,
  hexagon,
  hexagonCore,
  hexagonHook,
  hexagonSlots,
  magnifier,
  minusMark,
  panelFrame,
  personBust,
  plusMark,
  questionMark,
  shield,
  signalWaves,
  statusRing,
  sun,
  triangle,
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
  /**
   * 实心字形。实心图标的视觉包围盒就是几何包围盒，不需要为描边留出半个描边宽度，
   * 因此光学归一化按 stroke 0 计算。只给产品真正需要实心的概念做实心版。
   */
  readonly filled?: boolean;
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

  // ---- 状态族：同一个容器圆 + 不同内标 ----
  // 迁移前状态语义是混乱的：错误有 AlertCircle / CircleAlert / AlertTriangle /
  // TriangleAlert 四个同义图标，unsupported 与 error 共用一个，loading 有
  // LoaderCircle / Loader2 两个。这一族把每个状态收敛到唯一图形。
  spinner: {
    shape: "circle",
    fit: "radial",
    purpose: "进行中",
    // 留 60 度缺口，旋转时才看得出在转；缺口起点落在 -60 度，与指针对角线一致。
    build: () => [arc(8, 8, 6.375, -60, 240)],
  },
  "check-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "成功 / 就绪",
    build: () => [
      statusRing(),
      poly(
        [
          [5.4, 8.2],
          [7.3, 10.1],
          [10.7, 6.7],
        ],
        [0, 0.9, 0],
      ),
    ],
  },
  "x-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "失败 / 缺失",
    build: () => [statusRing(), ...crossMark(8, 8, 2.3)],
  },
  "exclamation-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "错误",
    build: () => [statusRing(), ...exclamationMark()],
  },
  "question-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "不支持 / 未知",
    build: () => [statusRing(), ...questionMark()],
  },
  "minus-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "已取消",
    build: () => [statusRing(), minusMark(8, 8, 2.6)],
  },
  "slash-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "被拒绝",
    // 斜杠按「禁止」的通用画法贯穿到接近圆周，是对 ≥3 单位净间隙规则的
    // 有意例外：缩短反而读不出禁止。方向与其它斜杠一致，从左下切到右上的反向。
    build: () => [
      statusRing(),
      poly(
        [
          [3.6, 12.4],
          [12.4, 3.6],
        ],
        0,
      ),
    ],
  },
  "stop-circle": {
    shape: "circle",
    minSize: 14,
    purpose: "被中断",
    build: () => [
      statusRing(),
      poly(
        [
          [6.1, 6.1],
          [9.9, 6.1],
          [9.9, 9.9],
          [6.1, 9.9],
        ],
        0.7,
        true,
      ),
    ],
  },
  "shield-x": {
    shape: "vertical",
    minSize: 14,
    purpose: "被权限策略阻断",
    build: () => [shield(), ...crossMark(8, 6.3, 1.6)],
  },
  "exclamation-triangle": {
    // 声明为 horizontal 会把三角压扁成又宽又矮；警告三角接近等边，用显式 visual。
    shape: "horizontal",
    visual: [13.5, 12],
    minSize: 14,
    purpose: "警告",
    build: () => [
      triangle(),
      poly(
        [
          [8, 6],
          [8, 8.9],
        ],
        0,
      ),
      poly(
        [
          [8, 11.1],
          [8, 11.1],
        ],
        0,
      ),
    ],
  },

  // ---- 工具来源与终端 ----
  terminal: {
    shape: "square",
    purpose: "终端类工具",
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
          [2.9, 4.1],
          [5.1, 6.3],
          [2.9, 8.5],
        ],
        [0, 0.8, 0],
      ),
      poly(
        [
          [6.6, 8.5],
          [9.1, 8.5],
        ],
        0,
      ),
    ],
  },

  // ---- Composer ----
  "arrow-up": {
    shape: "vertical",
    purpose: "发送",
    build: () => [
      poly(
        [
          [8, 14.4],
          [8, 1.6],
        ],
        0,
      ),
      poly(
        [
          [3.3, 6.3],
          [8, 1.6],
          [12.7, 6.3],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  stop: {
    // 唯一的实心字形。停止必须比发送更「重」，描边版在 16px 下读起来像一个空框。
    shape: "square",
    filled: true,
    visual: [10.5, 10.5],
    purpose: "停止运行",
    build: () => [
      poly(
        [
          [2.75, 2.75],
          [13.25, 2.75],
          [13.25, 13.25],
          [2.75, 13.25],
        ],
        2.2,
        true,
      ),
    ],
  },
  "chevron-left": {
    shape: "vertical",
    visual: [6.5, 11.8],
    purpose: "返回上一级",
    build: () => [poly(rotate([[0, 0], [5.25, 5.25], [0, 10.5]], 180, [2.625, 5.25]), [0, 1, 0])],
  },
  "file-list": {
    shape: "vertical",
    purpose: "文件 / 上下文文件 token",
    build: () => [
      fileBody(),
      poly(
        [
          [2.7, 8.1],
          [8.3, 8.1],
        ],
        0,
      ),
      poly(
        [
          [2.7, 10.7],
          [6.4, 10.7],
        ],
        0,
      ),
    ],
  },
  "display-pointer": {
    shape: "horizontal",
    minSize: 14,
    purpose: "计算机操作",
    build: () => [
      ...display(),
      // 屏幕里是一支指向右上的箭头，遵循全套「方向跟随指针」的规则。
      // 实心三角形在 14px 会被圆角揉成一个水滴，因此用描边箭头。
      poly(
        [
          [5.9, 8],
          [9.4, 4.5],
        ],
        0,
      ),
      poly(
        [
          [9.4, 6.4],
          [9.4, 4.5],
          [7.5, 4.5],
        ],
        [0, 0.6, 0],
      ),
    ],
  },
  copy: {
    shape: "square",
    minSize: 14,
    purpose: "复制",
    build: () => [
      // 后层只画露出的两条边，避免与前层重叠形成双描边。
      poly(
        [
          [3.4, 1.1],
          [11.6, 1.1],
          [11.6, 9.3],
        ],
        [1.6, 1.6, 0],
      ),
      poly(
        [
          [1.1, 4.4],
          [8.6, 4.4],
          [8.6, 11.9],
          [1.1, 11.9],
        ],
        1.6,
        true,
      ),
    ],
  },

  // ---- 设置页导航 ----
  chip: {
    shape: "square",
    minSize: 14,
    purpose: "模型 / 算力",
    build: () => {
      const ops: GeomOp[] = [
        poly(
          [
            [2.4, 2.4],
            [13.6, 2.4],
            [13.6, 13.6],
            [2.4, 13.6],
          ],
          1.5,
          true,
        ),
        poly(
          [
            [5.7, 5.7],
            [10.3, 5.7],
            [10.3, 10.3],
            [5.7, 10.3],
          ],
          0.7,
          true,
        ),
      ];
      // 四边各两根引脚，长度一致。
      for (const t of [-1.8, 1.8]) {
        ops.push(poly([[8 + t, 0.6], [8 + t, 2.4]], 0));
        ops.push(poly([[8 + t, 13.6], [8 + t, 15.4]], 0));
        ops.push(poly([[0.6, 8 + t], [2.4, 8 + t]], 0));
        ops.push(poly([[13.6, 8 + t], [15.4, 8 + t]], 0));
      }
      return ops;
    },
  },
  stack: {
    // 长期记忆用「层叠」而不是大脑。
    //
    // 试过两版大脑：圆加中缝被读成被分割的圆；交替半径的叶状轮廓在倒角后
    // 退化成五边形。大脑的识别特征是脑回的起伏，而这套图标只用直线倒角，
    // 在 16px 下画不出可辨的脑回 —— 与其上一个读不出来的字形，不如换比喻。
    //
    // 层叠在语义上更贴 Awaken 的记忆：它就是不断累积的 Markdown 记忆层。
    // 顶层画完整菱形，下面两层只画下半的 V，这样三层在 16 网格里放得开。
    shape: "square",
    minSize: 16,
    purpose: "长期记忆（累积的记忆层）",
    build: () => [
      poly(
        [
          [8, 1.8],
          [13.8, 5],
          [8, 8.2],
          [2.2, 5],
        ],
        0.8,
        true,
      ),
      poly(
        [
          [2.2, 8],
          [8, 11.2],
          [13.8, 8],
        ],
        [0, 0.8, 0],
      ),
      poly(
        [
          [2.2, 10.6],
          [8, 13.8],
          [13.8, 10.6],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  "color-mode": {
    // 第一版是「圆 + 三个颜料井」，实测三点在圆里被读成一张脸。
    // 改成对比度画法：圆加一条竖直分界，右半用等距斜线填充，
    // 这是「浅色 / 深色外观」的通用表达，且斜线方向与全套斜杠一致。
    shape: "circle",
    minSize: 14,
    purpose: "外观",
    build: () => {
      const ops: GeomOp[] = [
        circle(8, 8, 6.375),
        poly(
          [
            [8, 1.6],
            [8, 14.4],
          ],
          0,
        ),
      ];
      // 三条斜线的两端都落在圆内，长度按弦长收缩。
      for (const [y, len] of [
        [4.9, 2.4],
        [8, 4.9],
        [11.1, 2.4],
      ] as const) {
        ops.push(poly([[8.6, y], [8.6 + len, y]], 0));
      }
      return ops;
    },
  },
  paw: {
    shape: "circle",
    minSize: 16,
    purpose: "桌面宠物",
    build: () => {
      const ops: GeomOp[] = [];
      // 四个脚趾沿一段弧分布，掌垫在下方。
      for (const [x, y, r] of [
        [3.6, 6.1, 1.35],
        [6.6, 4.3, 1.4],
        [9.7, 4.3, 1.4],
        [12.6, 6.1, 1.35],
      ] as const) {
        ops.push(circle(x, y, r));
      }
      ops.push(
        poly(
          [
            [5.5, 9.2],
            [10.7, 9.2],
            [10.7, 12.6],
            [5.5, 12.6],
          ],
          2.3,
          true,
        ),
      );
      return ops;
    },
  },
  robot: {
    shape: "square",
    minSize: 14,
    purpose: "ACP runner",
    build: () => [
      poly(
        [
          [1.8, 4.6],
          [14.2, 4.6],
          [14.2, 13.4],
          [1.8, 13.4],
        ],
        2,
        true,
      ),
      poly(
        [
          [8, 2],
          [8, 4.6],
        ],
        0,
      ),
      poly([[5.6, 8.4], [5.6, 8.4]], 0),
      poly([[10.4, 8.4], [10.4, 8.4]], 0),
      poly(
        [
          [6, 11.1],
          [10, 11.1],
        ],
        0,
      ),
    ],
  },

  // ---- 主题三选 ----
  sun: {
    shape: "circle",
    fit: "radial",
    purpose: "浅色主题",
    build: () => sun(),
  },
  moon: {
    shape: "circle",
    fit: "radial",
    purpose: "黑色主题",
    build: () => crescent(),
  },
  display: {
    shape: "horizontal",
    purpose: "跟随系统",
    build: () => display(),
  },

  // ---- 账户与用量 ----
  person: {
    shape: "circle",
    minSize: 14,
    purpose: "用户",
    build: () => personBust(),
  },
  gauge: {
    shape: "horizontal",
    minSize: 14,
    purpose: "用量",
    build: () => [
      arcThroughChord([1.6, 11.4], [14.4, 11.4], 6.1, "right"),
      poly(
        [
          [8, 11.4],
          [11.3, 6.9],
        ],
        0,
      ),
    ],
  },
  "credit-card": {
    shape: "horizontal",
    purpose: "余额",
    build: () => [
      poly(
        [
          [1, 3.1],
          [15, 3.1],
          [15, 12.9],
          [1, 12.9],
        ],
        1.7,
        true,
      ),
      poly(
        [
          [1, 6.6],
          [15, 6.6],
        ],
        0,
      ),
    ],
  },
  key: {
    shape: "circle",
    minSize: 14,
    purpose: "API 密钥",
    build: () => [
      circle(4.9, 11.1, 3.1),
      poly(
        [
          [7.1, 8.9],
          [13.7, 2.3],
        ],
        0,
      ),
      poly(
        [
          [10.3, 5.7],
          [12.4, 7.8],
        ],
        0,
      ),
    ],
  },
  eye: {
    shape: "horizontal",
    purpose: "显示明文",
    build: () => eye(),
  },
  "eye-slash": {
    shape: "horizontal",
    minSize: 14,
    purpose: "隐藏明文",
    build: () => [
      ...eye(),
      // 斜杠方向与全套一致：从左上切到右下，与「跟随指针」的方向相反。
      poly(
        [
          [2.4, 2.4],
          [13.6, 13.6],
        ],
        0,
      ),
    ],
  },

  // ---- 通用动作补充 ----
  "arrow-left": {
    shape: "horizontal",
    purpose: "返回 / 后退",
    build: () => [
      poly(
        [
          [14.4, 8],
          [1.6, 8],
        ],
        0,
      ),
      poly(
        [
          [6.3, 3.3],
          [1.6, 8],
          [6.3, 12.7],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  "arrow-right-up": {
    // 遵循「方向跟随指针」：斜向箭头一律走左下到右上。
    shape: "square",
    purpose: "外部链接跳转",
    build: () => [
      poly(
        [
          [3.1, 12.9],
          [12.9, 3.1],
        ],
        0,
      ),
      poly(
        [
          [5.9, 3.1],
          [12.9, 3.1],
          [12.9, 10.1],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  "chevron-up": {
    shape: "horizontal",
    visual: [11.8, 6.5],
    purpose: "向上展开",
    build: () => [poly(rotate([[0, 0], [5.25, 5.25], [0, 10.5]], -90, [2.625, 5.25]), [0, 1, 0])],
  },
  "arrow-cw": {
    shape: "circle",
    fit: "radial",
    purpose: "重启 / 顺时针重试",
    build: () => {
      const r = 5.6;
      return [arc(8, 8, r, -50, 260), arrow(8, 8, r, 260, 1, 3.1, 32)];
    },
  },
  "arrow-circle-down": {
    shape: "circle",
    minSize: 14,
    purpose: "有新版本可下载",
    build: () => [
      statusRing(),
      poly(
        [
          [8, 4.4],
          [8, 11],
        ],
        0,
      ),
      poly(
        [
          [5.4, 8.4],
          [8, 11],
          [10.6, 8.4],
        ],
        [0, 0.7, 0],
      ),
    ],
  },
  upload: {
    shape: "vertical",
    purpose: "导入 / 上传",
    build: () => [
      poly(
        [
          [8, 13],
          [8, 2.4],
        ],
        0,
      ),
      poly(
        [
          [4.4, 6],
          [8, 2.4],
          [11.6, 6],
        ],
        [0, 0.8, 0],
      ),
      poly(
        [
          [2.2, 14.6],
          [13.8, 14.6],
        ],
        0,
      ),
    ],
  },
  "external-link": {
    shape: "square",
    minSize: 14,
    purpose: "在外部打开",
    build: () => [
      // 方框缺右上角，让斜向箭头从缺口出去；这是「开口只在必要处」的例外。
      poly(
        [
          [8.6, 2.6],
          [2.6, 2.6],
          [2.6, 13.4],
          [13.4, 13.4],
          [13.4, 7.4],
        ],
        [0, 1.5, 1.5, 1.5, 0],
      ),
      poly(
        [
          [7.8, 8.2],
          [13.4, 2.6],
        ],
        0,
      ),
      poly(
        [
          [9.6, 2.6],
          [13.4, 2.6],
          [13.4, 6.4],
        ],
        [0, 0.7, 0],
      ),
    ],
  },
  "floppy-disc": {
    shape: "square",
    minSize: 14,
    purpose: "保存",
    build: () => [
      poly(
        [
          [2.2, 2.2],
          [11.1, 2.2],
          [13.8, 4.9],
          [13.8, 13.8],
          [2.2, 13.8],
        ],
        [1.6, 0.5, 1.6, 1.6, 1.6],
        true,
      ),
      poly(
        [
          [5.5, 2.2],
          [5.5, 6],
          [10.5, 6],
          [10.5, 2.2],
        ],
        [0, 0.6, 0.6, 0],
      ),
      poly(
        [
          [5.5, 13.8],
          [5.5, 9.6],
          [10.5, 9.6],
          [10.5, 13.8],
        ],
        [0, 0.6, 0.6, 0],
      ),
    ],
  },
  "shield-check": {
    shape: "vertical",
    minSize: 14,
    purpose: "已授信",
    build: () => [
      shield(),
      poly(
        [
          [5.6, 6.4],
          [7.4, 8.2],
          [10.6, 5],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  wifi: {
    shape: "horizontal",
    purpose: "网络 / 连通性",
    build: () => signalWaves(),
  },
  "brackets-square": {
    shape: "horizontal",
    purpose: "代码 / opencode runner",
    build: () => bracketPair(),
  },
  lightning: {
    shape: "vertical",
    purpose: "Claude Code runner",
    build: () => [
      poly(
        [
          [9.6, 1.4],
          [4.2, 8.7],
          [7.9, 8.7],
          [6.4, 14.6],
          [11.8, 7.3],
          [8.1, 7.3],
        ],
        0.6,
        true,
      ),
    ],
  },
  history: {
    shape: "circle",
    fit: "radial",
    minSize: 14,
    purpose: "最近更新与整理",
    build: () => {
      const r = 6.1;
      return [
        arc(8, 8, r, -130, 180),
        arrow(8, 8, r, -130, -1, 2.9, 32),
        poly(
          [
            [8, 4.6],
            [8, 8],
            [10.8, 8],
          ],
          [0, 0.7, 0],
        ),
      ];
    },
  },
  "chat-bubble": {
    shape: "square",
    purpose: "会话 / 反馈",
    build: () => [
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
    ],
  },
  "book-open": {
    shape: "horizontal",
    minSize: 14,
    purpose: "参考资料",
    build: () => [
      poly(
        [
          [8, 4],
          [8, 13.4],
        ],
        0,
      ),
      poly(
        [
          [8, 4],
          [4.4, 2.4],
          [1.2, 2.4],
          [1.2, 11.8],
          [4.4, 11.8],
          [8, 13.4],
        ],
        [0, 0.6, 1.1, 1.1, 0.6, 0],
      ),
      poly(
        [
          [8, 4],
          [11.6, 2.4],
          [14.8, 2.4],
          [14.8, 11.8],
          [11.6, 11.8],
          [8, 13.4],
        ],
        [0, 0.6, 1.1, 1.1, 0.6, 0],
      ),
    ],
  },

  // ---- 文件类型族：同一个文件外形 + 不同内标 ----
  // 十一个类型共用 fileWithMark，比逐个手画更一致；内标一律只有一个形体，
  // 因为文件外形已经占掉大部分空间，15px 下容不下第二个细节。
  file: {
    shape: "vertical",
    purpose: "未识别类型文件",
    build: () => fileWithMark(),
  },
  "file-markdown": {
    shape: "vertical",
    minSize: 14,
    purpose: "Markdown 文件",
    build: () =>
      fileWithMark(
        poly(
          [
            [5.5, 7.4],
            [5.5, 11.2],
          ],
          0,
        ),
        poly(
          [
            [3.6, 9.3],
            [5.5, 11.2],
            [7.4, 9.3],
          ],
          [0, 0.7, 0],
        ),
      ),
  },
  "file-code": {
    shape: "vertical",
    minSize: 14,
    purpose: "代码文件",
    build: () =>
      fileWithMark(
        poly(
          [
            [4.3, 7.9],
            [2.6, 9.6],
            [4.3, 11.3],
          ],
          [0, 0.7, 0],
        ),
        poly(
          [
            [6.9, 7.9],
            [8.6, 9.6],
            [6.9, 11.3],
          ],
          [0, 0.7, 0],
        ),
      ),
  },
  "file-data": {
    shape: "vertical",
    minSize: 14,
    purpose: "数据文件（JSON / YAML）",
    build: () => fileWithMark(...bracketPair(5.5, 9.6, 3.1, 1.5, 2.1)),
  },
  "file-image": {
    shape: "vertical",
    minSize: 14,
    purpose: "图片文件",
    build: () => fileWithMark(circle(5.5, 9.6, 1.9)),
  },
  "file-archive": {
    shape: "vertical",
    minSize: 14,
    purpose: "压缩包",
    build: () =>
      fileWithMark(
        poly([[5.5, 7.3], [5.5, 7.3]], 0),
        poly([[5.5, 9.6], [5.5, 9.6]], 0),
        poly([[5.5, 11.9], [5.5, 11.9]], 0),
      ),
  },
  "file-audio": {
    shape: "vertical",
    minSize: 14,
    purpose: "音频文件",
    build: () =>
      fileWithMark(
        circle(4.4, 11.3, 1.5),
        poly(
          [
            [5.9, 11.3],
            [5.9, 7.4],
            [8.2, 8.1],
          ],
          [0, 0.6, 0],
        ),
      ),
  },
  "file-video": {
    shape: "vertical",
    minSize: 14,
    purpose: "视频文件",
    build: () =>
      fileWithMark(
        poly(
          [
            [4.2, 7.4],
            [8.3, 9.7],
            [4.2, 12],
          ],
          0.6,
          true,
        ),
      ),
  },
  "file-pdf": {
    shape: "vertical",
    minSize: 14,
    purpose: "PDF",
    // PDF 没有能在 15px 读出的字母标，改用一条贯穿的实心横带，
    // 在文件族里是唯一的「带」，因此可辨。
    build: () =>
      fileWithMark(
        poly(
          [
            [1.6, 8.2],
            [9.4, 8.2],
            [9.4, 11],
            [1.6, 11],
          ],
          0.8,
          true,
        ),
      ),
  },
  table: {
    shape: "square",
    minSize: 14,
    purpose: "表格 / Excel",
    build: () => [
      poly(
        [
          [1.6, 2.6],
          [14.4, 2.6],
          [14.4, 13.4],
          [1.6, 13.4],
        ],
        1.7,
        true,
      ),
      poly(
        [
          [1.6, 6.2],
          [14.4, 6.2],
        ],
        0,
      ),
      poly(
        [
          [1.6, 9.8],
          [14.4, 9.8],
        ],
        0,
      ),
      poly(
        [
          [6.4, 6.2],
          [6.4, 13.4],
        ],
        0,
      ),
    ],
  },
  "display-play": {
    shape: "horizontal",
    minSize: 14,
    purpose: "演示 / PowerPoint",
    build: () => [
      ...display(),
      poly(
        [
          [6.6, 4.5],
          [10.2, 6.8],
          [6.6, 9.1],
        ],
        0.6,
        true,
      ),
    ],
  },
  symlink: {
    // 第一版画成两个互扣圆环，与 mcp-compact 撞形（后者刻意呼应 MCP 官方标记的
    // 互扣语义）。改成折向箭头：先上行再右转，表示「指向别处」，
    // 与 external-link 的斜向箭头也区分得开。
    shape: "square",
    purpose: "符号链接",
    build: () => [
      poly(
        [
          [3.4, 13],
          [3.4, 6.6],
          [11.4, 6.6],
        ],
        [0, 1.3, 0],
      ),
      poly(
        [
          [8.6, 3.8],
          [11.4, 6.6],
          [8.6, 9.4],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  folders: {
    shape: "horizontal",
    minSize: 14,
    purpose: "文件夹组",
    build: () => [
      // 后层只画露出的两条边，避免与前层重叠形成双描边。
      poly(
        [
          [3.4, 4.4],
          [3.4, 1.9],
          [7.2, 1.9],
          [8.8, 3.5],
          [14.6, 3.5],
        ],
        [0, 1.1, 0.8, 0.6, 0],
      ),
      poly(
        [
          [1.4, 5.4],
          [5.2, 5.4],
          [6.8, 7],
          [12.6, 7],
          [12.6, 13.6],
          [1.4, 13.6],
        ],
        [1.2, 0.8, 0.6, 1.2, 1.2, 1.2],
        true,
      ),
    ],
  },
  "folder-search": {
    shape: "horizontal",
    minSize: 16,
    purpose: "定位到文件夹",
    build: () => [
      poly(
        [
          [1.4, 12.8],
          [1.4, 3.2],
          [6.2, 3.2],
          [8.2, 5.2],
          [13, 5.2],
          [13, 7.4],
        ],
        [1.2, 1.2, 0.8, 0.6, 1.2, 0],
      ),
      circle(10.2, 10.4, 2.6),
      poly(
        [
          [12.2, 12.4],
          [14.6, 14.8],
        ],
        0,
      ),
    ],
  },

  // ---- 预览与缩放 ----
  "magnifying-glass-plus": {
    shape: "circle",
    minSize: 14,
    purpose: "放大",
    build: () => magnifier(plusMark(6.25, 6.25, 2.1)),
  },
  "magnifying-glass-minus": {
    shape: "circle",
    minSize: 14,
    purpose: "缩小",
    build: () => magnifier([minusMark(6.25, 6.25, 2.1)]),
  },
  "rect-magnifying-glass": {
    shape: "circle",
    minSize: 16,
    purpose: "适应页面",
    build: () =>
      magnifier([
        poly(
          [
            [4.1, 4.6],
            [8.4, 4.6],
            [8.4, 7.9],
            [4.1, 7.9],
          ],
          0.6,
          true,
        ),
      ]),
  },
  "corners-out": {
    shape: "square",
    purpose: "放大 / 全屏",
    build: () => cornerBrackets(true),
  },
  "corners-in": {
    shape: "square",
    purpose: "退出全屏",
    build: () => cornerBrackets(false),
  },
  "arrow-ccw": {
    shape: "circle",
    fit: "radial",
    purpose: "逆时针旋转 / 重置",
    build: () => {
      const r = 5.6;
      return [arc(8, 8, r, 230, -80), arrow(8, 8, r, -80, -1, 3.1, 32)];
    },
  },
  pencil: {
    shape: "circle",
    purpose: "编辑",
    build: () => [
      // 笔身沿 45 度，笔尖在左下，遵循「方向跟随指针」的反向（书写方向）。
      poly(
        [
          [2.1, 13.9],
          [3.2, 10.4],
          [11.4, 2.2],
          [13.8, 4.6],
          [5.6, 12.8],
        ],
        [0.5, 0.6, 1.1, 1.1, 0.6],
        true,
      ),
      poly(
        [
          [9.6, 4],
          [12, 6.4],
        ],
        0,
      ),
    ],
  },
  paragraph: {
    shape: "square",
    purpose: "自动换行",
    build: () => [
      poly(
        [
          [1.6, 3.4],
          [14.4, 3.4],
        ],
        0,
      ),
      poly(
        [
          [1.6, 8],
          [11.2, 8],
          [11.2, 11.4],
          [4.6, 11.4],
        ],
        [0, 1.5, 1.5, 0],
      ),
      poly(
        [
          [6.6, 9.4],
          [4.6, 11.4],
          [6.6, 13.4],
        ],
        [0, 0.7, 0],
      ),
    ],
  },

  // ---- 布局与窗口 ----
  "layout-sidebar-left": {
    shape: "horizontal",
    purpose: "切换左侧栏",
    build: () => panelFrame("left"),
  },
  "layout-sidebar-right": {
    shape: "horizontal",
    purpose: "切换右侧 Inspector",
    build: () => panelFrame("right"),
  },
  window: {
    // 标题栏三个点是与 credit-card 区分的关键：两者都是圆角矩形加一条顶部横线，
    // 没有点的话在任何尺寸下都分不开。
    shape: "horizontal",
    minSize: 14,
    purpose: "用其他应用打开",
    build: () => [
      poly(
        [
          [1.6, 2.8],
          [14.4, 2.8],
          [14.4, 13.2],
          [1.6, 13.2],
        ],
        1.8,
        true,
      ),
      poly(
        [
          [1.6, 6.2],
          [14.4, 6.2],
        ],
        0,
      ),
      poly([[4, 4.5], [4, 4.5]], 0),
      poly([[6.2, 4.5], [6.2, 4.5]], 0),
      poly([[8.4, 4.5], [8.4, 4.5]], 0),
    ],
  },
  globe: {
    shape: "circle",
    purpose: "浏览器 / 时区 / 联网",
    build: () => globe(),
  },

  // ---- 媒体 ----
  image: {
    shape: "square",
    minSize: 14,
    purpose: "图片",
    build: () => [
      poly(
        [
          [1.6, 2.6],
          [14.4, 2.6],
          [14.4, 13.4],
          [1.6, 13.4],
        ],
        1.7,
        true,
      ),
      circle(5.5, 6.5, 1.4),
      poly(
        [
          [1.6, 11.6],
          [6.1, 7.6],
          [14.4, 13.4],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  play: {
    shape: "circle",
    filled: true,
    visual: [11.5, 12.5],
    purpose: "播放",
    build: () => [
      poly(
        [
          [3.6, 2.2],
          [13.4, 8],
          [3.6, 13.8],
        ],
        1.1,
        true,
      ),
    ],
  },
  "video-camera": {
    shape: "horizontal",
    minSize: 14,
    purpose: "视频",
    build: () => [
      poly(
        [
          [1.4, 4.6],
          [9.6, 4.6],
          [9.6, 11.4],
          [1.4, 11.4],
        ],
        1.6,
        true,
      ),
      poly(
        [
          [9.6, 8.6],
          [14.6, 11.4],
          [14.6, 4.6],
          [9.6, 7.4],
        ],
        [0, 0.8, 0.8, 0],
        true,
      ),
    ],
  },

  // ---- 连接器 ----
  server: {
    shape: "vertical",
    minSize: 14,
    purpose: "MCP server",
    build: () => {
      const ops: GeomOp[] = [];
      for (const y of [2.4, 9]) {
        ops.push(
          poly(
            [
              [2.6, y],
              [13.4, y],
              [13.4, y + 4.6],
              [2.6, y + 4.6],
            ],
            1.3,
            true,
          ),
        );
        ops.push(poly([[5.2, y + 2.3], [5.2, y + 2.3]], 0));
      }
      return ops;
    },
  },
  "plug-slash": {
    shape: "circle",
    minSize: 16,
    purpose: "结束接管 / 断开",
    build: () => [
      poly(
        [
          [5.2, 10.8],
          [2.4, 13.6],
        ],
        0,
      ),
      poly(
        [
          [6.1, 5.1],
          [10.9, 9.9],
          [8.6, 12.2],
          [3.8, 7.4],
        ],
        1.4,
        true,
      ),
      poly(
        [
          [9.4, 3.6],
          [11.2, 5.4],
        ],
        0,
      ),
      poly(
        [
          [12, 6.2],
          [13.8, 8],
        ],
        0,
      ),
      // 斜杠从左上切到右下，与全套斜杠方向一致。
      poly(
        [
          [2.4, 2.4],
          [13.6, 13.6],
        ],
        0,
      ),
    ],
  },

  // ---- 其它补齐 ----
  "arrow-right": {
    shape: "horizontal",
    purpose: "前进",
    build: () => [
      poly(
        [
          [1.6, 8],
          [14.4, 8],
        ],
        0,
      ),
      poly(
        [
          [9.7, 3.3],
          [14.4, 8],
          [9.7, 12.7],
        ],
        [0, 0.8, 0],
      ),
    ],
  },
  funnel: {
    shape: "vertical",
    purpose: "筛选",
    build: () => [
      poly(
        [
          [1.6, 2.6],
          [14.4, 2.6],
          [9.4, 8.4],
          [9.4, 14],
          [6.6, 12],
          [6.6, 8.4],
        ],
        [1, 1, 0.7, 0.7, 0.7, 0.7],
        true,
      ),
    ],
  },
  comment: {
    shape: "circle",
    minSize: 14,
    purpose: "会话（圆形气泡）",
    build: () => [
      circle(8, 7.2, 5.9),
      poly(
        [
          [4.6, 11.9],
          [3.2, 14.4],
        ],
        0,
      ),
    ],
  },
  package: {
    shape: "square",
    minSize: 14,
    purpose: "包 / 依赖 token",
    build: () => [
      poly(
        [
          [8, 1.8],
          [14.2, 5.1],
          [14.2, 11.5],
          [8, 14.8],
          [1.8, 11.5],
          [1.8, 5.1],
        ],
        1.1,
        true,
      ),
      poly(
        [
          [1.8, 5.1],
          [8, 8.4],
          [14.2, 5.1],
        ],
        [0, 0.8, 0],
      ),
      poly(
        [
          [8, 8.4],
          [8, 14.8],
        ],
        0,
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
