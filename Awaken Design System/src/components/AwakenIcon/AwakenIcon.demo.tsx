import {
  AGENT_PLUGINS_OFFICIAL_MARK,
  AWAKEN_BRAND_ICON_NAMES,
  AWAKEN_ICON_CONCEPTS,
  AWAKEN_ICON_SIZES,
  AWAKEN_ICON_STROKE,
  AWAKEN_LINE_ICON_NAMES,
  AWAKEN_OPTICAL_SHAPES,
  APPROVED_LUCIDE_ICON_NAMES,
  APPROVED_LUCIDE_LICENSE,
  APPROVED_LUCIDE_STROKE,
  APPROVED_LUCIDE_VERSION,
  ICON_DEFINITIONS,
  MCP_OFFICIAL_MARK,
  getAwakenIcon,
  opticalVisual,
  type AwakenIconConcept,
  type AwakenIconName,
} from "../../icons";
import { AwakenIcon } from "./AwakenIcon";
import "./AwakenIcon.demo.css";

const CAPABILITY_FAMILY: Array<{ name: AwakenIconName; label: string }> = [
  { name: "connector", label: "连接器" },
  { name: "book-open", label: "技能" },
  { name: "puzzle", label: "插件" },
  { name: "terminal", label: "CLI" },
  { name: "hook", label: "钩子" },
];

export function AwakenIconDemo() {
  return (
    <section className="ads-component-demo aw-icon-demo" aria-label="图标库 AwakenIcon 组件演示">
      <header className="ads-component-demo-header">
        <h2>
          图标库 <code>AwakenIcon</code>
        </h2>
        <p>
          Awaken 的 16 网格线性图标集，共 {AWAKEN_LINE_ICON_NAMES.length} 个字形与{" "}
          {AWAKEN_BRAND_ICON_NAMES.length} 个第三方官方品牌标记。其中 {APPROVED_LUCIDE_ICON_NAMES.length} 个字形来自经用户确认的
          Lucide v{APPROVED_LUCIDE_VERSION} 母形（{APPROVED_LUCIDE_LICENSE}），已经离线固化为本地 path，不产生运行时依赖；其余字形由 Awaken 几何或有来源的产品恢复轮廓持有。
          Awaken 几何使用固定 {AWAKEN_ICON_STROKE} 描边，Lucide 固化轮廓使用等比后的 {APPROVED_LUCIDE_STROKE} 描边，二者都保持圆头端点与圆角接合。
          <code>AwakenIcon</code> 仍是唯一渲染出口，尺寸只接受 {AWAKEN_ICON_SIZES.join(" / ")} 五档。
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
          Awaken 构造几何继续显示目标与实测光学盒；Lucide authored 字形保留独立实验室中确认的原始比例，不再二次拉伸。
        </p>
        <div className="aw-icon-demo__grid">
          {AWAKEN_LINE_ICON_NAMES.map((name) => {
            const icon = getAwakenIcon(name);
            const def = ICON_DEFINITIONS[name];
            const [tw, th] = opticalVisual(def);
            const measured = icon.mode === "line" ? icon.measured : undefined;
            const provenance = icon.mode === "line" ? icon.provenance : undefined;
            return (
              <div key={name} className="aw-icon-demo__card">
                <div className="aw-icon-demo__card-art">
                  <AwakenIcon name={name} size={24} />
                </div>
                <div className="aw-icon-demo__card-meta">
                  <code>{name}</code>
                  <span>{def.purpose}</span>
                  <em>
                    {provenance?.kind === "licensed-authored"
                      ? `${def.shape} · ${provenance.source} · ${provenance.license}`
                      : measured
                        ? `${def.shape} · 目标 ${tw}×${th} · 实测 ${measured[0].toFixed(2)}×${measured[1].toFixed(2)}`
                        : `${def.shape} · authored`}
                    {icon.mode === "line" && icon.minSize > 12 ? ` · 下限 ${icon.minSize}px` : ""}
                  </em>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="aw-icon-demo__block">
        <h3>能力与连接器入口</h3>
        <p className="aw-icon-demo__hint">
          产品一级入口优先使用熟悉、彼此可区分的比喻：连接器使用插头，Skill 使用打开的书，插件使用四边凸榫拼图块，CLI 使用终端；Hook 使用 Webhook 路由结构。
        </p>
        <div className="aw-icon-demo__family">
          {CAPABILITY_FAMILY.map(({ name, label }) => (
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
        <h3>官方品牌标记</h3>
        <p className="aw-icon-demo__hint">
          MCP 与 Agent Plugins 都使用各自官方 authored paths，只通过对称透明留白对齐 Awaken 的光学尺寸。MCP
          标记可读下限是 {MCP_OFFICIAL_MARK.minSize}px；Agent Plugins 标记因节点与括号密度较高，可读下限是{" "}
          {AGENT_PLUGINS_OFFICIAL_MARK.minSize}px。产品插件入口已改用线性 <code>puzzle</code>，本标记只按名字取用。
        </p>
        <div className="aw-icon-demo__family">
          <div className="aw-icon-demo__family-item">
            <AwakenIcon name="agent-plugins-mark" size={24} label="Agent Plugins" />
            <div className="aw-icon-demo__family-small">
              <AwakenIcon name="agent-plugins-mark" size={20} />
            </div>
            <code>agent-plugins-mark</code>
            <span>官方标记 · 仅 ≥{AGENT_PLUGINS_OFFICIAL_MARK.minSize}px</span>
          </div>
          <div className="aw-icon-demo__family-item">
            <AwakenIcon name="mcp-mark" size={24} label="Model Context Protocol" />
            <div className="aw-icon-demo__family-small">
              <AwakenIcon name="mcp-mark" size={16} />
              <AwakenIcon name="mcp-mark" size={14} />
            </div>
            <code>mcp-mark</code>
            <span>官方标记 · 仅 ≥{MCP_OFFICIAL_MARK.minSize}px</span>
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
          四种光学形状继续作为分组、定制几何归一化和尺寸审计口径；licensed authored 字形保留已确认比例，不用目标盒再次拉伸。
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
