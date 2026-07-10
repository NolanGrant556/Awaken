import { usePetAnimation } from "./pet-animation";
import {
  petSheetRowsForVersion,
  type PetAnimationState,
  type PetSpriteVersion,
} from "../../shared/pets";

export type PetSpriteProps = {
  spritesheetUrl: string;
  state: PetAnimationState;
  widthPx: number;
  spriteVersionNumber?: PetSpriteVersion;
};

export function PetSprite({
  spritesheetUrl,
  state,
  widthPx,
  spriteVersionNumber = 1,
}: PetSpriteProps) {
  const { backgroundPosition } = usePetAnimation(state, spriteVersionNumber);
  const sheetRows = petSheetRowsForVersion(spriteVersionNumber);

  return (
    <div
      className="pet-sprite"
      role="img"
      aria-label={`宠物状态 ${state}`}
      style={{
        ["--pet-width" as string]: `${widthPx}px`,
        ["--pet-sheet-rows" as string]: sheetRows,
        backgroundImage: `url(${spritesheetUrl})`,
        backgroundPosition,
      }}
    />
  );
}
