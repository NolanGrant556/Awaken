import type { AwakenProductLogoProps } from "./AwakenProductLogo";

export type AwakenProductLogoState = {
  id: string;
  label: string;
  props: AwakenProductLogoProps;
};

export const AwakenProductLogoStates: AwakenProductLogoState[] = [
  { id: "compact", label: "紧凑 32px", props: { size: "sm" } },
  { id: "standard", label: "标准 40px", props: { size: "md" } },
  { id: "display", label: "展示 64px", props: { size: "lg" } },
];
