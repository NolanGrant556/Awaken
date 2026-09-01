import "./AwakenPluginBrandIcon.css";
import outlookLogo from "../../assets/plugin-brands/outlook.svg";

// Outlook asset source: Microsoft Fluent product icon CDN,
// fabric-cdn-prod_20251008.001/assets/brand-icons/product/svg/outlook_48x1.svg.

export type AwakenPluginBrand = "outlook";
export type AwakenPluginBrandIconSize = "menu" | "card" | "detail";

const BRAND_ASSETS: Record<AwakenPluginBrand, string> = {
  outlook: outlookLogo,
};

export function awakenPluginBrandAsset(brand: AwakenPluginBrand): string {
  return BRAND_ASSETS[brand];
}

export function isAwakenPluginBrand(value: string | undefined): value is AwakenPluginBrand {
  return Boolean(value && value in BRAND_ASSETS);
}

export type AwakenPluginBrandIconProps = {
  brand: AwakenPluginBrand;
  size?: AwakenPluginBrandIconSize;
  className?: string;
};

// Product marks remain unmodified inside three fixed optical boxes: 14px in
// Composer menus, 28px in 36px cards, and 34px in 52px detail headers.
export function AwakenPluginBrandIcon({
  brand,
  size = "card",
  className,
}: AwakenPluginBrandIconProps) {
  return (
    <span
      className={["aw-plugin-brand-icon", className ?? ""].filter(Boolean).join(" ")}
      data-brand={brand}
      data-size={size}
      aria-hidden="true"
    >
      <img src={awakenPluginBrandAsset(brand)} alt="" draggable={false} />
    </span>
  );
}
