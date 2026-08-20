import { useMemo, useState } from "react";
import {
  AWAKEN_LOGO_CATALOG,
  AWAKEN_LOGO_OPTICAL_SIZES,
  AWAKEN_LOGO_STYLES,
  awakenLogoOpticalSizeForDisplay,
  getAwakenLogo,
  searchAwakenLogos,
  type AwakenLogoId,
  type AwakenLogoOpticalSize,
  type AwakenLogoStyle,
} from "../../logos";
import { AwakenLogo } from "./AwakenLogo";
import { AwakenProductLogo } from "../AwakenProductLogo/AwakenProductLogo";
import { AwakenProductLogoStates } from "../AwakenProductLogo/AwakenProductLogo.states";
import "./AwakenLogo.demo.css";

export function AwakenLogoDemo() {
  const [query, setQuery] = useState("");
  const [opticalSize, setOpticalSize] = useState<AwakenLogoOpticalSize>(24);
  const [appearance, setAppearance] = useState<AwakenLogoStyle>("filled");
  const [selectedId, setSelectedId] = useState<AwakenLogoId>("product-mark");
  const [previewScale, setPreviewScale] = useState(32);
  const entries = useMemo(() => searchAwakenLogos(query), [query]);
  const selected = getAwakenLogo(selectedId);
  const selectedOptical = selected.opticalSizes.includes(
    awakenLogoOpticalSizeForDisplay(previewScale),
  )
    ? awakenLogoOpticalSizeForDisplay(previewScale)
    : selected.opticalSizes[0];

  return (
    <section className="ads-component-demo aw-logo-library-demo" aria-label="Logo 库 AwakenLogo 组件演示">
      <header className="ads-component-demo-header">
        <h2>Logo 库 <code>AwakenLogo</code></h2>
        <p>
          产品标记按 16 / 24 光学尺寸和 filled / outline 两套切法绘制。供应商 Logo
          只登记、不重绘。每个概念只有一个标记。
        </p>
      </header>

      <div className="aw-logo-library-demo__toolbar">
        <label>
          搜索
          <input
            type="search"
            value={query}
            placeholder="标记、字标、供应商或概念"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <fieldset>
          <legend>光学尺寸</legend>
          {AWAKEN_LOGO_OPTICAL_SIZES.map((size) => (
            <label key={size}>
              <input
                type="radio"
                name="aw-logo-optical-size"
                checked={opticalSize === size}
                onChange={() => setOpticalSize(size)}
              />
              {size}
            </label>
          ))}
        </fieldset>
        <fieldset>
          <legend>样式</legend>
          {AWAKEN_LOGO_STYLES.map((style) => (
            <label key={style}>
              <input
                type="radio"
                name="aw-logo-style"
                checked={appearance === style}
                onChange={() => setAppearance(style)}
              />
              {style}
            </label>
          ))}
        </fieldset>
      </div>

      <div className="aw-logo-library-demo__grid">
        {entries.map((entry) => (
          <article key={entry.id} className="aw-logo-library-demo__card">
            <button
              type="button"
              className="aw-logo-library-demo__select"
              data-selected={entry.id === selectedId}
              onClick={() => setSelectedId(entry.id)}
            >
              <div className="aw-logo-library-demo__preview" data-kind={entry.kind}>
                <AwakenLogo
                  id={entry.id}
                  opticalSize={opticalSize}
                  appearance={appearance}
                  displaySize={entry.id === "product-wordmark" ? 160 : opticalSize}
                  decorative
                />
              </div>
              <h3>{entry.name}</h3>
              <p>{entry.concept}</p>
              <code>{entry.id}</code>
            </button>
          </article>
        ))}
      </div>

      <aside className="aw-logo-library-demo__detail" aria-label="选中 Logo 细节">
        <div className="aw-logo-library-demo__detail-preview" data-kind={selected.kind}>
          <AwakenLogo
            id={selected.id}
            opticalSize={selectedOptical}
            appearance={appearance}
            displaySize={selected.id === "product-wordmark" ? Math.max(previewScale * 5, 120) : previewScale}
            decorative
          />
        </div>
        <div>
          <h3>{selected.name}</h3>
          <p>{selected.concept}</p>
          <p>
            显示 {previewScale}px · 光学切法 {selectedOptical} · {selected.opticalShape}
          </p>
          <label>
            缩放
            <input
              type="range"
              min={12}
              max={64}
              value={previewScale}
              onChange={(event) => setPreviewScale(Number(event.target.value))}
            />
          </label>
          <p>
            <code>{selected.id}</code>
          </p>
        </div>
      </aside>

      <section className="aw-logo-library-demo__concepts" aria-label="概念对照">
        <h3>这个概念用哪个 Logo？</h3>
        <table>
          <thead>
            <tr>
              <th>概念</th>
              <th>Logo</th>
              <th>用法</th>
            </tr>
          </thead>
          <tbody>
            {AWAKEN_LOGO_CATALOG.map((entry) => (
              <tr key={entry.id}>
                <td>{entry.concept}</td>
                <td>
                  {entry.name} <code>{entry.id}</code>
                </td>
                <td>{entry.usage}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section aria-label="产品 LOGO 组件入口">
        <h3>产品入口 <code>AwakenProductLogo</code></h3>
        <p>Desktop 继续使用现有产品组件。它从 Logo 库取标记，不再内联第二份 path。</p>
        <div className="ads-component-demo-grid">
          {AwakenProductLogoStates.map((state) => (
            <article key={state.id} className="ads-component-demo-state">
              <h4>{state.label}</h4>
              <div className="ads-component-demo-canvas">
                <AwakenProductLogo {...state.props} />
              </div>
            </article>
          ))}
        </div>
      </section>
    </section>
  );
}
