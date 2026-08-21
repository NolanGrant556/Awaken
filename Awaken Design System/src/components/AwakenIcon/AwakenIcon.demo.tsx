import { useMemo, useState } from "react";
import {
  AWAKEN_ICON_CATALOG,
  AWAKEN_ICON_OPTICAL_SIZES,
  getAwakenIcon,
  resolveAwakenIconOpticalSize,
  searchAwakenIcons,
  type AwakenIconId,
  type AwakenIconOpticalSize,
} from "../../icons";
import { AwakenIcon } from "./AwakenIcon";
import "./AwakenIcon.demo.css";

const STYLES = ["outline", "filled"] as const;

export function AwakenIconDemo() {
  const [query, setQuery] = useState("");
  const [opticalSize, setOpticalSize] = useState<AwakenIconOpticalSize>(16);
  const [appearance, setAppearance] = useState<(typeof STYLES)[number]>("outline");
  const [selectedId, setSelectedId] = useState<AwakenIconId>("plug");
  const [previewScale, setPreviewScale] = useState(16);
  const entries = useMemo(() => searchAwakenIcons(query), [query]);
  const selected = getAwakenIcon(selectedId);
  const selectedOptical = resolveAwakenIconOpticalSize(previewScale);

  return (
    <section className="ads-component-demo aw-icon-library-demo" aria-label="图标库 AwakenIcon 组件演示">
      <header className="ads-component-demo-header">
        <h2>图标库 <code>AwakenIcon</code></h2>
        <p>
          界面图标按文章里的构造方式手绘：16 / 24 两套光学尺寸，1.25 / 1.5
          描边，outline 与 filled，每个概念只有一个图标。不复制 Cursor 图形，也不再走
          lucide 包装层。
        </p>
      </header>

      <div className="aw-icon-library-demo__toolbar">
        <label>
          搜索
          <input
            type="search"
            value={query}
            placeholder="发送、连接器、文件或概念"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <fieldset>
          <legend>光学尺寸</legend>
          {AWAKEN_ICON_OPTICAL_SIZES.map((size) => (
            <label key={size}>
              <input
                type="radio"
                name="aw-icon-optical-size"
                checked={opticalSize === size}
                onChange={() => {
                  setOpticalSize(size);
                  setPreviewScale(size);
                }}
              />
              {size}
            </label>
          ))}
        </fieldset>
        <fieldset>
          <legend>样式</legend>
          {STYLES.map((style) => (
            <label key={style}>
              <input
                type="radio"
                name="aw-icon-style"
                checked={appearance === style}
                onChange={() => setAppearance(style)}
              />
              {style}
            </label>
          ))}
        </fieldset>
      </div>

      <div className="aw-icon-library-demo__grid">
        {entries.map((entry) => (
          <article key={entry.id} className="aw-icon-library-demo__card">
            <button
              type="button"
              className="aw-icon-library-demo__select"
              data-selected={entry.id === selectedId}
              onClick={() => setSelectedId(entry.id)}
            >
              <div className="aw-icon-library-demo__preview">
                <AwakenIcon
                  id={entry.id}
                  opticalSize={opticalSize}
                  filled={appearance === "filled"}
                  size={opticalSize}
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

      <aside className="aw-icon-library-demo__detail" aria-label="选中图标细节">
        <div className="aw-icon-library-demo__detail-preview">
          <AwakenIcon
            id={selected.id}
            opticalSize={selectedOptical}
            filled={appearance === "filled"}
            size={previewScale}
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

      <section className="aw-icon-library-demo__concepts" aria-label="概念对照">
        <h3>这个概念用哪个图标？</h3>
        <table>
          <thead>
            <tr>
              <th>概念</th>
              <th>图标</th>
              <th>用法</th>
            </tr>
          </thead>
          <tbody>
            {AWAKEN_ICON_CATALOG.map((entry) => (
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
    </section>
  );
}
