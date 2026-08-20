import "./AwakenProductLogo.css";
import productLogo from "../../assets/product-logo.png";

export type AwakenProductLogoSize = "sm" | "md" | "lg";
export type AwakenProductLogoVariant = "mark" | "icon";

export type AwakenProductLogoProps = {
  size?: AwakenProductLogoSize;
  variant?: AwakenProductLogoVariant;
  className?: string;
  decorative?: boolean;
  alt?: string;
};

export function AwakenProductLogo({
  size = "md",
  variant = "mark",
  className,
  decorative = false,
  alt = "Awaken",
}: AwakenProductLogoProps) {
  const classNames = ["aw-product-logo", className].filter(Boolean).join(" ");

  if (variant === "icon") {
    return (
      <img
        className={classNames}
        data-size={size}
        data-variant="icon"
        src={productLogo}
        alt={decorative ? "" : alt}
        aria-hidden={decorative ? true : undefined}
        draggable={false}
      />
    );
  }

  return (
    <svg
      className={classNames}
      data-size={size}
      data-variant="mark"
      viewBox="0 0 24 24"
      fill="currentColor"
      role={decorative ? undefined : "img"}
      aria-hidden={decorative ? true : undefined}
      aria-label={decorative ? undefined : alt}
    >
      {decorative ? null : <title>{alt}</title>}
      <path d="M10.9 2v6.3L6 16.1 2 22v-5.9L6.9 8.3z" />
      <path d="M13.1 2v6.3l4 7.8 4.9 5.9v-5.9L17.1 8.3z" />
    </svg>
  );
}
