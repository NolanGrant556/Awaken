import type { PetAnimationState } from "../../shared/pets";

export type PetDragDirection = "left" | "right";

export function resolveOverlayAnimationState(input: {
  resizeMode: boolean;
  dragging: boolean;
  dragDirection: PetDragDirection;
  hover: boolean;
  effectiveBusinessState: PetAnimationState;
}): PetAnimationState {
  if (input.resizeMode) return input.effectiveBusinessState;
  if (input.dragging) {
    return input.dragDirection === "left" ? "running-left" : "running-right";
  }
  if (input.hover) return "jumping";
  return input.effectiveBusinessState;
}
