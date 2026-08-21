/**
 * 第三方官方品牌标记。
 *
 * 这些形体不是 Awaken 画的，因此不进入 geometry.ts 的 16 网格构造体系，
 * 也不参与光学归一化 —— 按 DESIGN_SYSTEM_GUIDE 的既有规则，
 * 品牌标记保留 authored geometry。它们只被等比缩放到请求尺寸。
 *
 * 每个标记都记录来源与许可，并声明一个 minSize：品牌标记的笔画密度
 * 由品牌方决定，低于 minSize 时会粘连，必须换用 compact 伴随字形。
 */

export type BrandMark = {
  /** 原始 viewBox，保持 authored geometry。 */
  readonly viewBox: string;
  readonly paths: readonly string[];
  /** stroke 型标记的原始描边宽度（相对 viewBox 单位）。fill 型为 undefined。 */
  readonly strokeWidth?: number;
  readonly mode: "stroke" | "fill";
  /** 低于这个 px 尺寸会粘连，必须换用 compact 伴随字形。 */
  readonly minSize: number;
  readonly source: string;
  readonly license: string;
  readonly trademark: string;
};

/**
 * Model Context Protocol 官方标记。
 *
 * 取自 modelcontextprotocol/docs 的 logo/light.svg。该文件本身是横向字标
 * （viewBox 0 0 1338 195），标记是其中前三条 path，占 x 25–170.5。
 * 这里只保留标记部分，未对路径本身做任何改动。
 *
 * 官方描边比例 12/145.5 = 8.2%，Awaken 16 网格是 1.25/12.75 = 9.8%，
 * 两者接近，所以等比缩放进 Circle 光学盒后与自绘图标的重量差异可以接受。
 *
 * minSize 20 是实测结论：12/14px 下三条互扣勾形的平行笔画会粘成一团，
 * 16px 勉强，20px 才读得出。Simple Icons 的实心版与其它字体版本同样如此，
 * 这是标记结构决定的，不是缩放方式的问题。
 */
export const MCP_OFFICIAL_MARK: BrandMark = {
  viewBox: "19 14.598 157.491 174.922",
  paths: [
    "M25 97.8528L92.8823 29.9706C102.255 20.598 117.451 20.598 126.823 29.9706V29.9706C136.196 39.3431 136.196 54.5391 126.823 63.9117L75.5581 115.177",
    "M76.2653 114.47L126.823 63.9117C136.196 54.5391 151.392 54.5391 160.765 63.9117L161.118 64.2652C170.491 73.6378 170.491 88.8338 161.118 98.2063L99.7248 159.6C96.6006 162.724 96.6006 167.789 99.7248 170.913L112.331 183.52",
    "M109.853 46.9411L59.6482 97.1457C50.2757 106.518 50.2757 121.714 59.6482 131.087V131.087C69.0208 140.459 84.2168 140.459 93.5894 131.087L143.794 80.8822",
  ],
  strokeWidth: 12,
  mode: "stroke",
  minSize: 20,
  source: "modelcontextprotocol/docs · logo/light.svg（前三条 path，未改动）",
  license: "仓库文档 CC-BY-4.0，规范 Apache-2.0",
  trademark: "Model Context Protocol a Series of LF Projects, LLC",
};

/**
 * Agent Skills 的事实标记说明。
 *
 * agentskills.io 没有发布品牌规范页，站点首页的 logo carousel 只列各家客户端
 * 的 logo，标准本身没有正式 mark。Anthropic 官方 skills 仓库里的
 * brand-guidelines skill 只覆盖颜色与字体，并明确把 logo 与商标工作排除在外。
 *
 * 但 agentskills.io/favicon.svg 的内容是一个实心尖顶正六边形：
 *   M16 0.5L29.4234 8.25V23.75L16 31.5L2.57661 23.75V8.25L16 0.5Z
 * 宽 26.85 × 高 31，长宽比 0.866 = cos30°，即标准正六边形。
 *
 * 因此 skill 采用六边形外形，与生态事实标记一致；但几何由 Awaken 自己的
 * hexagon 原语生成，而不是引用该文件 —— 正六边形是基本几何形，
 * 不构成商标标的，自绘可以让描边、圆角与全套统一。
 */
export const SKILL_SHAPE_PROVENANCE = {
  shape: "尖顶正六边形",
  deFactoSource: "agentskills.io/favicon.svg（实心版）",
  note: "无正式品牌规范；正六边形为基本几何形，Awaken 用自有 hexagon 原语绘制描边版",
} as const;
