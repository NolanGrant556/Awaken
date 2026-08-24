/**
 * 第三方官方品牌标记。
 *
 * 这些形体不是 Awaken 画的，因此不进入 geometry.ts 的 16 网格构造体系。
 * 品牌标记保留 authored paths；Awaken 只能在 viewBox 外围增加对称透明留白，
 * 让它与同尺寸线性图标使用一致的光学包围盒，不得改写原始 path。
 *
 * 每个标记都记录来源与许可，并声明一个 minSize：品牌标记的笔画密度
 * 由品牌方决定，调用点不得低于该尺寸。
 */

export type BrandMark = {
  /** 原始 viewBox，保持 authored geometry。 */
  readonly viewBox: string;
  readonly paths: readonly string[];
  /** stroke 型标记的原始描边宽度（相对 viewBox 单位）。fill 型为 undefined。 */
  readonly strokeWidth?: number;
  readonly mode: "stroke" | "fill";
  /** 低于这个 px 尺寸会粘连，调用点必须保留该最小尺寸。 */
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
 * 实机对比后为 authored geometry 增加对称透明留白：请求 16px 时，
 * 实际标记落在约 14px 光学盒内；请求 14px 时与工具轨迹的 14px 线性图标等重。
 * Retina Desktop 下 14px 请求仍可辨识，因此它是当前产品可读下限。
 */
export const MCP_OFFICIAL_MARK: BrandMark = {
  // 原始紧包围盒为 19 14.598 157.491 174.922；仅对称增加透明留白。
  viewBox: "7.751 2.104 179.99 199.911",
  paths: [
    "M25 97.8528L92.8823 29.9706C102.255 20.598 117.451 20.598 126.823 29.9706V29.9706C136.196 39.3431 136.196 54.5391 126.823 63.9117L75.5581 115.177",
    "M76.2653 114.47L126.823 63.9117C136.196 54.5391 151.392 54.5391 160.765 63.9117L161.118 64.2652C170.491 73.6378 170.491 88.8338 161.118 98.2063L99.7248 159.6C96.6006 162.724 96.6006 167.789 99.7248 170.913L112.331 183.52",
    "M109.853 46.9411L59.6482 97.1457C50.2757 106.518 50.2757 121.714 59.6482 131.087V131.087C69.0208 140.459 84.2168 140.459 93.5894 131.087L143.794 80.8822",
  ],
  strokeWidth: 12,
  mode: "stroke",
  minSize: 14,
  source: "modelcontextprotocol/docs · logo/light.svg（前三条 path，未改动）",
  license: "仓库文档 CC-BY-4.0，规范 Apache-2.0",
  trademark: "Model Context Protocol a Series of LF Projects, LLC",
};
