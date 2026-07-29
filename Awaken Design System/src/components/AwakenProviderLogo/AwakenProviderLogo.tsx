import "./AwakenProviderLogo.css";
import kimiLogo from "../../assets/model-providers/kimi.png";
import minimaxLogo from "../../assets/model-providers/minimax.svg";
import glmLogo from "../../assets/model-providers/glm.png";
import qwenLogo from "../../assets/model-providers/qwen.svg";
import doubaoLogo from "../../assets/model-providers/doubao.png";
import deepseekLogo from "../../assets/model-providers/deepseek.svg";
import volcengineLogo from "../../assets/model-providers/volcengine.svg";

export type AwakenProviderBrand =
  | "kimi"
  | "minimax"
  | "glm"
  | "qwen"
  | "doubao"
  | "deepseek"
  | "volcengine";

const PROVIDER_LOGOS: Record<AwakenProviderBrand, string> = {
  kimi: kimiLogo,
  minimax: minimaxLogo,
  glm: glmLogo,
  qwen: qwenLogo,
  doubao: doubaoLogo,
  deepseek: deepseekLogo,
  volcengine: volcengineLogo,
};

export type AwakenProviderLogoProps = {
  brand: AwakenProviderBrand;
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
      <img src={PROVIDER_LOGOS[brand]} alt="" draggable={false} />
    </span>
  );
}
