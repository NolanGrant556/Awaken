/**
 * Awaken 图标注册表 —— 全套图标唯一的查询入口。
 *
 * 这一层是 Cursor 那招「换字体不改引用」在组件层的等价物：
 * 调用点只认名字，几何可以逐个替换而调用点零改动。
 *
 * Awaken 构造几何由 geometry.ts 在模块初始化时编译一次；用户确认的 licensed
 * authored 字形已经离线固化为同名 16-grid path，并在这里覆盖基础定义而不改变 API。
 * 品牌标记保留自己的 authored paths 与 viewBox。
 */
import { MCP_OFFICIAL_MARK, type BrandMark } from "./brandMarks";
import {
  APPROVED_LUCIDE_GLYPHS,
  type ApprovedLucideGlyph,
} from "./approvedLucideGlyphs";
import {
  APPROVED_LUCIDE_LICENSE,
  APPROVED_LUCIDE_SOURCES,
  APPROVED_LUCIDE_STROKE,
  APPROVED_LUCIDE_VERSION,
  type ApprovedLucideIconName,
} from "./approvedLucideSources";
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
      /** 实心字形为 0，渲染时改用 fill。 */
      readonly strokeWidth: number;
      readonly filled: boolean;
      /** 少数 authored 子路径需要保留自身的 currentColor 填充。 */
      readonly filledPathIndexes?: readonly number[];
      /** 实测视觉尺寸（16 网格单位），供光学审计核对。 */
      readonly measured?: readonly [number, number];
      readonly shape: AwakenOpticalShape;
      /** 可读下限（px）。低于它的调用点由守卫测试拦下。 */
      readonly minSize: number;
      readonly provenance:
        | { readonly kind: "awaken-geometry" }
        | {
            readonly kind: "licensed-authored" | "restored-authored";
            readonly source: string;
            readonly license: string;
          };
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
    const approved = (
      APPROVED_LUCIDE_GLYPHS as Partial<Record<AwakenLineIconName, ApprovedLucideGlyph>>
    )[key];
    if (approved) {
      const sourceIcon = APPROVED_LUCIDE_SOURCES[key as ApprovedLucideIconName];
      out[key] = {
        mode: "line",
        viewBox: LINE_VIEW_BOX,
        paths: approved.paths,
        strokeWidth: def.filled ? 0 : APPROVED_LUCIDE_STROKE,
        filled: def.filled ?? false,
        filledPathIndexes: approved.filledPathIndexes,
        shape: def.shape,
        minSize: def.minSize ?? 12,
        provenance: {
          kind: "licensed-authored",
          source: `lucide-react v${APPROVED_LUCIDE_VERSION} · ${sourceIcon}`,
          license: APPROVED_LUCIDE_LICENSE,
        },
      };
      continue;
    }
    if (def.authored) {
      out[key] = {
        mode: "line",
        viewBox: LINE_VIEW_BOX,
        paths: def.authored.paths,
        strokeWidth: def.filled ? 0 : AWAKEN_ICON_STROKE,
        filled: def.filled ?? false,
        measured: def.authored.measured,
        shape: def.shape,
        minSize: def.minSize ?? 12,
        provenance: {
          kind: "restored-authored",
          source: def.authored.source,
          license: def.authored.license,
        },
      };
      continue;
    }
    if (!def.build) {
      throw new Error(`Awaken 图标 ${key} 缺少 build 或 authored path`);
    }
    const stroke = def.filled ? 0 : AWAKEN_ICON_STROKE;
    const { ops, measured } = normalizeToOptical(
      def.build(),
      opticalVisual(def),
      def.fit ?? "bbox",
      stroke,
    );
    out[key] = {
      mode: "line",
      viewBox: LINE_VIEW_BOX,
      paths: emitPaths(ops),
      strokeWidth: stroke,
      filled: def.filled ?? false,
      measured,
      shape: def.shape,
      minSize: def.minSize ?? 12,
      provenance: { kind: "awaken-geometry" },
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
