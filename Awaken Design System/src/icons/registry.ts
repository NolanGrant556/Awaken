/**
 * Awaken 图标注册表 —— 全套图标唯一的查询入口。
 *
 * 这一层是 Cursor 那招「换字体不改引用」在组件层的等价物：
 * 调用点只认名字，几何可以逐个替换而调用点零改动。
 *
 * 线性图标由 geometry.ts 在模块初始化时编译一次（纯数学，成本可忽略），
 * 品牌标记保留 authored geometry 不参与归一化。
 */
import { MCP_OFFICIAL_MARK, type BrandMark } from "./brandMarks";
import {
  ICON_DEFINITIONS,
  type AwakenLineIconName,
  opticalVisual,
} from "./definitions";
import {
  AWAKEN_ICON_GRID,
  AWAKEN_ICON_STROKE,
  type AwakenOpticalShape,
  emitPaths,
  normalizeToOptical,
} from "./geometry";

export type AwakenIconRender =
  | {
      readonly mode: "line";
      readonly viewBox: string;
      readonly paths: readonly string[];
      readonly strokeWidth: number;
      /** 实测视觉尺寸（16 网格单位），供光学审计核对。 */
      readonly measured: readonly [number, number];
      readonly shape: AwakenOpticalShape;
      /** 可读下限（px）。低于它的调用点由守卫测试拦下。 */
      readonly minSize: number;
    }
  | {
      readonly mode: "brand-stroke";
      readonly viewBox: string;
      readonly paths: readonly string[];
      readonly strokeWidth: number;
      readonly minSize: number;
    }
  | {
      readonly mode: "brand-fill";
      readonly viewBox: string;
      readonly paths: readonly string[];
      readonly minSize: number;
    };

const LINE_VIEW_BOX = `0 0 ${AWAKEN_ICON_GRID} ${AWAKEN_ICON_GRID}`;

function compileLineIcons(): Record<AwakenLineIconName, AwakenIconRender> {
  const out = {} as Record<AwakenLineIconName, AwakenIconRender>;
  for (const key of Object.keys(ICON_DEFINITIONS) as AwakenLineIconName[]) {
    const def = ICON_DEFINITIONS[key];
    const { ops, measured } = normalizeToOptical(
      def.build(),
      opticalVisual(def),
      def.fit ?? "bbox",
      AWAKEN_ICON_STROKE,
    );
    out[key] = {
      mode: "line",
      viewBox: LINE_VIEW_BOX,
      paths: emitPaths(ops),
      strokeWidth: AWAKEN_ICON_STROKE,
      measured,
      shape: def.shape,
      minSize: def.minSize ?? 12,
    };
  }
  return out;
}

function compileBrandMark(mark: BrandMark): AwakenIconRender {
  return mark.mode === "stroke"
    ? {
        mode: "brand-stroke",
        viewBox: mark.viewBox,
        paths: mark.paths,
        strokeWidth: mark.strokeWidth ?? 1,
        minSize: mark.minSize,
      }
    : {
        mode: "brand-fill",
        viewBox: mark.viewBox,
        paths: mark.paths,
        minSize: mark.minSize,
      };
}

const LINE_ICONS = compileLineIcons();

/** 第三方官方品牌标记。名字带 `-mark` 后缀，和自绘线性图标明确区分。 */
export const BRAND_ICONS = {
  "mcp-mark": compileBrandMark(MCP_OFFICIAL_MARK),
} as const;

export type AwakenBrandIconName = keyof typeof BRAND_ICONS;
export type AwakenIconName = AwakenLineIconName | AwakenBrandIconName;

const ALL_ICONS: Record<string, AwakenIconRender> = { ...LINE_ICONS, ...BRAND_ICONS };

export function getAwakenIcon(name: AwakenIconName): AwakenIconRender {
  const found = ALL_ICONS[name];
  if (!found) {
    throw new Error(`未注册的 Awaken 图标：${name}`);
  }
  return found;
}

/** 全部图标名，按字母序。Gallery 与审计测试用它遍历。 */
export const AWAKEN_ICON_NAMES = Object.keys(ALL_ICONS).sort() as AwakenIconName[];

export const AWAKEN_LINE_ICON_NAMES = Object.keys(LINE_ICONS).sort() as AwakenLineIconName[];
export const AWAKEN_BRAND_ICON_NAMES = Object.keys(BRAND_ICONS).sort() as AwakenBrandIconName[];

export function isAwakenIconName(value: string): value is AwakenIconName {
  return Object.prototype.hasOwnProperty.call(ALL_ICONS, value);
}
