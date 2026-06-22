import type { ComponentProps } from "react";
import type { AwakenAvatar } from "./AwakenAvatar";

export type AwakenAvatarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenAvatar>>;
};

export const AwakenAvatarStates: AwakenAvatarState[] = [
  {
    id: "fallback-initials",
    label: "姓名缩写",
    props: {
      fallback: "Nolan Grant",
      size: "md",
    } as unknown as Partial<ComponentProps<typeof AwakenAvatar>>,
  },
  {
    id: "small-fallback",
    label: "小尺寸头像",
    props: {
      fallback: "AI",
      size: "sm",
    } as unknown as Partial<ComponentProps<typeof AwakenAvatar>>,
  },
  {
    id: "image",
    label: "图片头像",
    props: {
      src: "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='64' height='64' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='32' fill='%231f2937'/%3E%3Ctext x='32' y='39' text-anchor='middle' font-size='22' font-family='Arial' fill='white'%3ENG%3C/text%3E%3C/svg%3E",
      fallback: "Nolan Grant",
      size: "md",
    } as unknown as Partial<ComponentProps<typeof AwakenAvatar>>,
  },
];
