import "./AwakenProviderLogo.css";
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

export type AwakenProviderBrand =
  | "openai"
  | "anthropic"
  | "gemini"
  | "kimi"
  | "minimax"
  | "glm"
  | "qwen"
  | "doubao"
  | "deepseek"
  | "volcengine";

const PROVIDER_LOGOS: Record<AwakenProviderBrand, string> = {
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

export type AwakenProviderLogoProps = {
  brand?: AwakenProviderBrand;
  name: string;
  className?: string;
};

export function AwakenProviderLogo({
  brand,
  name,
  className,
}: AwakenProviderLogoProps) {
  return (
    <span
      className={["aw-provider-logo", className ?? ""].filter(Boolean).join(" ")}
      data-provider-brand={brand}
      role="img"
      aria-label={`${name} Logo`}
    >
      {brand ? <img src={PROVIDER_LOGOS[brand]} alt="" draggable={false} />
        : <span className="aw-provider-logo__initial">{name.trim().slice(0, 1) || "M"}</span>}
    </span>
  );
}
