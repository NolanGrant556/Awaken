import {
  AWAKEN_BRAND_ICON_NAMES,
  AWAKEN_ICON_CONCEPTS,
  AWAKEN_ICON_SIZES,
  AWAKEN_ICON_STROKE,
  AWAKEN_LINE_ICON_NAMES,
  AWAKEN_OPTICAL_SHAPES,
  ICON_DEFINITIONS,
  MCP_OFFICIAL_MARK,
  getAwakenIcon,
  opticalVisual,
  type AwakenIconConcept,
  type AwakenIconName,
} from "../../icons";
import { AwakenIcon } from "./AwakenIcon";
import "./AwakenIcon.demo.css";

const HEXAGON_FAMILY: Array<{ name: AwakenIconName; label: string }> = [
  { name: "skill", label: "技能包（基型）" },
  { name: "skill-core", label: "已激活" },
  { name: "hook", label: "钩子" },
  { name: "connector", label: "连接器" },
];

export function AwakenIconDemo() {
  return (
    <section className="ads-component-demo aw-icon-demo" aria-label="图标库 AwakenIcon 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          图标库 <code>AwakenIcon</code>
        </h2>
        <p>
          Awaken 自有的 16 网格线性图标集，共 {AWAKEN_LINE_ICON_NAMES.length} 个原创字形与{" "}
          {AWAKEN_BRAND_ICON_NAMES.length} 个第三方官方品牌标记。全套由同一套几何约束生成：只用 0 / 45 / 90
          度直线后倒角、圆头端点、描边固定 {AWAKEN_ICON_STROKE}、按四种光学形状归一化视觉重量、重叠净间隙不小于 3
          个网格单位。<code>AwakenIcon</code> 是唯一渲染出口，尺寸只接受 {AWAKEN_ICON_SIZES.join(" / ")} 五档。
        </p>
      </header>

      <div className="aw-icon-demo__block">
        <h3>全套 · 真实尺寸</h3>
        <p className="aw-icon-demo__hint">
          每一行是一个尺寸档，与 <code>tokens.css</code> 里既有的 <code>--aw-icon-size-*</code> 对应。12px 是可读下限，16px
          是主用尺寸。
        </p>
        {AWAKEN_ICON_SIZES.map((size) => (
          <div key={size} className="aw-icon-demo__row">
            <span className="aw-icon-demo__row-label">{size}px</span>
            <div className="aw-icon-demo__row-icons">
              {AWAKEN_LINE_ICON_NAMES.map((name) => (
                <AwakenIcon key={name} name={name} size={size} />
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="aw-icon-demo__block">
        <h3>逐图标</h3>
        <p className="aw-icon-demo__hint">
          实测视觉尺寸贴合声明的光学形状，是「同屏视觉重量一致」的验收口径；等比缩放只能让一条轴贴合，另一条轴不得超出。
        </p>
        <div className="aw-icon-demo__grid">
          {AWAKEN_LINE_ICON_NAMES.map((name) => {
            const icon = getAwakenIcon(name);
            const def = ICON_DEFINITIONS[name];
            const [tw, th] = opticalVisual(def);
            const measured = icon.mode === "line" ? icon.measured : ([0, 0] as const);
            return (
              <div key={name} className="aw-icon-demo__card">
                <div className="aw-icon-demo__card-art">
                  <AwakenIcon name={name} size={24} />
                </div>
                <div className="aw-icon-demo__card-meta">
                  <code>{name}</code>
                  <span>{def.purpose}</span>
                  <em>
                    {def.shape} · 目标 {tw}×{th} · 实测 {measured[0].toFixed(2)}×{measured[1].toFixed(2)}
                    {icon.mode === "line" && icon.minSize > 12 ? ` · 下限 ${icon.minSize}px` : ""}
                  </em>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="aw-icon-demo__block">
        <h3>六边形形状族</h3>
        <p className="aw-icon-demo__hint">
          尖顶正六边形是 Awaken「可装配单元」的族形，全套里唯一，因此不会和方框族、文件族混淆。族内成员靠内标区分；带内标的成员实测要到
          16px 才分得开，因此它们的可读下限是 16px，只有基型 <code>skill</code> 能下到 12px。
        </p>
        <div className="aw-icon-demo__family">
          {HEXAGON_FAMILY.map(({ name, label }) => (
            <div key={name} className="aw-icon-demo__family-item">
              <AwakenIcon name={name} size={24} />
              <div className="aw-icon-demo__family-small">
                <AwakenIcon name={name} size={16} />
                <AwakenIcon name={name} size={14} />
                <AwakenIcon name={name} size={12} />
              </div>
              <code>{name}</code>
              <span>{label}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="aw-icon-demo__block">
        <h3>官方品牌标记与 compact 伴随字形</h3>
        <p className="aw-icon-demo__hint">
          MCP 使用官方标记（{MCP_OFFICIAL_MARK.source}），保留 authored geometry 不参与光学归一化。它的可读下限是{" "}
          {MCP_OFFICIAL_MARK.minSize}px：更小的尺寸下互扣勾形的平行笔画会粘连，这是标记结构决定的。因此 14–16px 的密集行改用
          Awaken 自绘的 <code>mcp-compact</code> 伴随字形，不改动官方标记。商标归 {MCP_OFFICIAL_MARK.trademark}。
        </p>
        <div className="aw-icon-demo__family">
          <div className="aw-icon-demo__family-item">
            <AwakenIcon name="mcp-mark" size={24} label="Model Context Protocol" />
            <div className="aw-icon-demo__family-small">
              <AwakenIcon name="mcp-mark" size={24} />
              <AwakenIcon name="mcp-mark" size={20} />
            </div>
            <code>mcp-mark</code>
            <span>官方标记 · 仅 ≥{MCP_OFFICIAL_MARK.minSize}px</span>
          </div>
          <div className="aw-icon-demo__family-item">
            <AwakenIcon name="mcp-compact" size={24} />
            <div className="aw-icon-demo__family-small">
              <AwakenIcon name="mcp-compact" size={16} />
              <AwakenIcon name="mcp-compact" size={14} />
            </div>
            <code>mcp-compact</code>
            <span>compact 伴随字形 · 14–16px</span>
          </div>
        </div>
      </div>

      <div className="aw-icon-demo__block">
        <h3>概念表</h3>
        <p className="aw-icon-demo__hint">
          每个「X 用哪个图标」只有一个答案。产品代码优先用 <code>concept</code> 而不是 <code>name</code>：概念是稳定的语义
          API，图形可以在不动调用点的前提下替换。
        </p>
        <div className="aw-icon-demo__concepts">
          {(Object.keys(AWAKEN_ICON_CONCEPTS) as AwakenIconConcept[]).map((concept) => (
            <div key={concept} className="aw-icon-demo__concept">
              <AwakenIcon concept={concept} size={16} />
              <code>{concept}</code>
              <em>{AWAKEN_ICON_CONCEPTS[concept]}</em>
            </div>
          ))}
        </div>
      </div>

      <div className="aw-icon-demo__block">
        <h3>光学形状</h3>
        <p className="aw-icon-demo__hint">
          四种光学形状各自的视觉尺寸经过补偿，使不同外形的图标读起来一样大 —— 圆必须画得比方稍大才显得等大。
        </p>
        <div className="aw-icon-demo__shapes">
          {(Object.keys(AWAKEN_OPTICAL_SHAPES) as Array<keyof typeof AWAKEN_OPTICAL_SHAPES>).map((shape) => {
            const [w, h] = AWAKEN_OPTICAL_SHAPES[shape];
            const sample = AWAKEN_LINE_ICON_NAMES.filter((n) => ICON_DEFINITIONS[n].shape === shape);
            return (
              <div key={shape} className="aw-icon-demo__shape">
                <div className="aw-icon-demo__shape-head">
                  <code>{shape}</code>
                  <em>
                    {w} × {h}
                  </em>
                </div>
                <div className="aw-icon-demo__shape-icons">
                  {sample.map((name) => (
                    <AwakenIcon key={name} name={name} size={20} />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
