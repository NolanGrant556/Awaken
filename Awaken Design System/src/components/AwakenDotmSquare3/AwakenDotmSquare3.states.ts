import type { AwakenDotmSquare3Props } from "./AwakenDotmSquare3";

export type AwakenDotmSquare3State = {
  id: string;
  label: string;
  props: AwakenDotmSquare3Props;
};

export const AwakenDotmSquare3States: AwakenDotmSquare3State[] = [
  { id: "square-dots", label: "方形点", props: { ariaLabel: "加载中" } },
];
