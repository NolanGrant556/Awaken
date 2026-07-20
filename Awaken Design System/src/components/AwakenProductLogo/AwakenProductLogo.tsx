import "./AwakenProductLogo.css";
import productLogo from "../../assets/product-logo.png";

export type AwakenProductLogoSize = "sm" | "md" | "lg";

export type AwakenProductLogoProps = {
  size?: AwakenProductLogoSize;
  className?: string;
  decorative?: boolean;
  alt?: string;
};

export function AwakenProductLogo({
  size = "md",
  className,
  decorative = false,
  alt = "Awaken",
}: AwakenProductLogoProps) {
  return (
    <img
      className={["aw-product-logo", className].filter(Boolean).join(" ")}
      data-size={size}
      src={productLogo}
      alt={decorative ? "" : alt}
      aria-hidden={decorative ? true : undefined}
      draggable={false}
    />
  );
}
