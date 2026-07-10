import { usePetAnimation } from "./pet-animation";
import type { PetAnimationState } from "../../shared/pets";

export type PetSpriteProps = {
  spritesheetUrl: string;
  state: PetAnimationState;
  widthPx: number;
};

export function PetSprite({ spritesheetUrl, state, widthPx }: PetSpriteProps) {
  const { backgroundPosition } = usePetAnimation(state);

  return (
    <div
      className="pet-sprite"
      role="img"
      aria-label={`宠物状态 ${state}`}
      style={{
        ["--pet-width" as string]: `${widthPx}px`,
        backgroundImage: `url(${spritesheetUrl})`,
        backgroundPosition,
      }}
    />
  );
}
