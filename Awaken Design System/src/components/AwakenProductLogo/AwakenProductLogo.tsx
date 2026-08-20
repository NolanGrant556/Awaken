import "./AwakenProductLogo.css";
import { AwakenLogo } from "../AwakenLogo/AwakenLogo";
import { awakenLogoOpticalSizeForDisplay } from "../../logos";

export type AwakenProductLogoSize = 16 | 24 | "sm" | "md" | "lg";
export type AwakenProductLogoVariant = "mark" | "icon";
export type AwakenProductLogoAppearance = "filled" | "outline";

export type AwakenProductLogoProps = {
  size?: AwakenProductLogoSize;
  variant?: AwakenProductLogoVariant;
  appearance?: AwakenProductLogoAppearance;
  className?: string;
  decorative?: boolean;
  alt?: string;
};

const DISPLAY_SIZE: Record<Exclude<AwakenProductLogoSize, 16 | 24>, number> = {
  sm: 32,
  md: 40,
  lg: 64,
};

function displaySizeFor(size: AwakenProductLogoSize): number {
  return size === 16 || size === 24 ? size : DISPLAY_SIZE[size];
}

export function AwakenProductLogo({
  size = "md",
  variant = "mark",
  appearance = "filled",
  className,
  decorative = false,
  alt = "Awaken",
}: AwakenProductLogoProps) {
  const displaySize = displaySizeFor(size);
  const opticalSize =
    variant === "mark" ? awakenLogoOpticalSizeForDisplay(displaySize) : 24;

  return (
    <AwakenLogo
      id={variant === "icon" ? "product-icon" : "product-mark"}
      opticalSize={opticalSize}
      appearance={variant === "mark" ? appearance : "filled"}
      displaySize={displaySize}
      className={["aw-product-logo", className].filter(Boolean).join(" ")}
      decorative={decorative}
      alt={alt}
    />
  );
}
