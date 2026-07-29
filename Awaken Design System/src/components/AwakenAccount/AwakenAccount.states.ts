import { AwakenProfileBalanceSamples } from "../AwakenProfileBalancePanel";
import type { AwakenAccountProps } from "./AwakenAccount";

const noop = () => {};

export type AwakenAccountState = {
  id: string;
  label: string;
  props: AwakenAccountProps;
};

export const AwakenAccountStates: AwakenAccountState[] = [
  {
    id: "available",
    label: "额度可用",
    props: {
      name: "Nolan",
      avatarFallback: "AW",
      balances: AwakenProfileBalanceSamples,
      themePreference: "light",
      defaultOpen: true,
      onOpenSettings: noop,
    },
  },
  {
    id: "loading",
    label: "查询额度",
    props: {
      name: "Nolan",
      avatarFallback: "AW",
      loading: true,
      themePreference: "light",
      defaultOpen: true,
      onOpenSettings: noop,
    },
  },
  {
    id: "closed",
    label: "收起",
    props: {
      name: "Nolan",
      avatarFallback: "AW",
      balances: AwakenProfileBalanceSamples,
      themePreference: "light",
      onOpenSettings: noop,
    },
  },
];
