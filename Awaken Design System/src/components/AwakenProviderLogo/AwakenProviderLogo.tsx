import "./AwakenProviderLogo.css";
import { AwakenLogo } from "../AwakenLogo/AwakenLogo";
import { getAwakenLogo, type AwakenProviderLogoId } from "../../logos";

export type AwakenProviderBrand = AwakenProviderLogoId;

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
  const entry = getAwakenLogo(brand);
  return (
    <span
      className={["aw-provider-logo", className ?? ""].filter(Boolean).join(" ")}
      data-provider-brand={brand}
      data-logo-id={entry.id}
      role="img"
      aria-label={`${name} Logo`}
    >
      <AwakenLogo id={brand} decorative />
    </span>
  );
}
