import type { CSSProperties } from "react";
import "./AwakenLogo.css";
import {
  awakenLogoOpticalSizeForDisplay,
  getAwakenLogo,
  PRODUCT_MARK_PATHS,
  PRODUCT_MARK_VIEWBOX,
  productMarkStrokeWidth,
  type AwakenLogoId,
  type AwakenLogoOpticalSize,
  type AwakenLogoStyle,
  type AwakenProviderLogoId,
} from "../../logos";
import productIcon from "../../assets/product-logo.png";
import productWordmark from "../../assets/logo-wordmark.svg";
import kimiLogo from "../../assets/model-providers/kimi.png";
import minimaxLogo from "../../assets/model-providers/minimax.svg";
import glmLogo from "../../assets/model-providers/glm.png";
import qwenLogo from "../../assets/model-providers/qwen.svg";
import doubaoLogo from "../../assets/model-providers/doubao.png";
import deepseekLogo from "../../assets/model-providers/deepseek.svg";
import volcengineLogo from "../../assets/model-providers/volcengine.svg";
import openaiLogo from "../../assets/model-providers/openai.svg";
import anthropicLogo from "../../assets/model-providers/anthropic.svg";
import geminiLogo from "../../assets/model-providers/gemini.svg";

const PROVIDER_SOURCES: Record<AwakenProviderLogoId, string> = {
  openai: openaiLogo,
  anthropic: anthropicLogo,
  gemini: geminiLogo,
  kimi: kimiLogo,
  minimax: minimaxLogo,
  glm: glmLogo,
  qwen: qwenLogo,
  doubao: doubaoLogo,
  deepseek: deepseekLogo,
  volcengine: volcengineLogo,
};

export type AwakenLogoProps = {
  id: AwakenLogoId;
  opticalSize?: AwakenLogoOpticalSize;
  appearance?: AwakenLogoStyle;
  displaySize?: number;
  className?: string;
  decorative?: boolean;
  alt?: string;
};

function resolveOpticalSize(
  id: AwakenLogoId,
  opticalSize: AwakenLogoOpticalSize | undefined,
  displaySize: number | undefined,
): AwakenLogoOpticalSize {
  const entry = getAwakenLogo(id);
  const requested =
    opticalSize ??
    (typeof displaySize === "number" ? awakenLogoOpticalSizeForDisplay(displaySize) : 24);
  return entry.opticalSizes.includes(requested) ? requested : entry.opticalSizes[0];
}

function resolveStyle(id: AwakenLogoId, appearance: AwakenLogoStyle | undefined): AwakenLogoStyle {
  const entry = getAwakenLogo(id);
  const requested = appearance ?? "filled";
  return entry.styles.includes(requested) ? requested : entry.styles[0];
}

export function AwakenLogo({
  id,
  opticalSize,
  appearance,
  displaySize,
  className,
  decorative = false,
  alt,
}: AwakenLogoProps) {
  const entry = getAwakenLogo(id);
  const size = resolveOpticalSize(id, opticalSize, displaySize);
  const style = resolveStyle(id, appearance);
  const classNames = ["aw-logo", className].filter(Boolean).join(" ");
  const label = alt ?? entry.name;
  const displayPx = displaySize ?? (id === "product-wordmark" ? 220 : size);

  if (id === "product-mark") {
    const paths = PRODUCT_MARK_PATHS[size][style];
    const strokeWidth = productMarkStrokeWidth(size, style);
    return (
      <span
        className={classNames}
        data-logo-id={id}
        data-optical-size={size}
        data-logo-style={style}
        style={{ "--aw-logo-display": `${displayPx}px` } as CSSProperties}
      >
        <svg
          className="aw-logo__mark"
          viewBox={PRODUCT_MARK_VIEWBOX[size]}
          fill={strokeWidth ? "none" : "currentColor"}
          role={decorative ? undefined : "img"}
          aria-hidden={decorative ? true : undefined}
          aria-label={decorative ? undefined : label}
        >
          {decorative ? null : <title>{label}</title>}
          {paths.map((d) => (
            <path key={d} d={d} />
          ))}
        </svg>
      </span>
    );
  }

  const src =
    id === "product-wordmark"
      ? productWordmark
      : id === "product-icon"
        ? productIcon
        : PROVIDER_SOURCES[id];

  return (
    <img
      className={classNames}
      data-logo-id={id}
      data-optical-size={size}
      data-logo-style={style}
      data-provider-brand={entry.kind === "provider" ? id : undefined}
      style={{ "--aw-logo-display": `${displayPx}px` } as CSSProperties}
      src={src}
      alt={decorative ? "" : label}
      aria-hidden={decorative ? true : undefined}
      draggable={false}
    />
  );
}

export function awakenLogoSource(id: AwakenProviderLogoId): string {
  return PROVIDER_SOURCES[id];
}
