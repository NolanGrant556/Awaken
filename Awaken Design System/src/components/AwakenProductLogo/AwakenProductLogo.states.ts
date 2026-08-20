import type { AwakenProductLogoProps } from "./AwakenProductLogo";

export type AwakenProductLogoState = {
  id: string;
  label: string;
  props: AwakenProductLogoProps;
};

export const AwakenProductLogoStates: AwakenProductLogoState[] = [
  { id: "mark-compact", label: "标记 32px", props: { size: "sm", variant: "mark" } },
  { id: "mark-standard", label: "标记 40px", props: { size: "md", variant: "mark" } },
  { id: "mark-display", label: "标记 64px", props: { size: "lg", variant: "mark" } },
  { id: "icon-standard", label: "应用图标 40px", props: { size: "md", variant: "icon" } },
];
