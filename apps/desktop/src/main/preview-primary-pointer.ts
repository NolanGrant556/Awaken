import type { MouseInputEvent } from "electron";

const PREVIEW_PRIMARY_CLICK_DISTANCE_PX = 6;

export type PreviewPrimaryPointerGesture = {
  startX: number;
  startY: number;
  moved: boolean;
};

export function updatePreviewPrimaryPointerGesture(
  gesture: PreviewPrimaryPointerGesture | undefined,
  mouse: MouseInputEvent,
): {
  gesture: PreviewPrimaryPointerGesture | undefined;
  primaryClick: boolean;
} {
  if (mouse.type === "mouseDown" && mouse.button === "left") {
    return {
      gesture: { startX: mouse.x, startY: mouse.y, moved: false },
      primaryClick: false,
    };
  }
  if (!gesture) return { gesture: undefined, primaryClick: false };
  if (mouse.type === "mouseMove") {
    const distanceX = mouse.x - gesture.startX;
    const distanceY = mouse.y - gesture.startY;
    return {
      gesture: {
        ...gesture,
        moved:
          gesture.moved ||
          distanceX * distanceX + distanceY * distanceY >
            PREVIEW_PRIMARY_CLICK_DISTANCE_PX *
              PREVIEW_PRIMARY_CLICK_DISTANCE_PX,
      },
      primaryClick: false,
    };
  }
  if (mouse.type === "mouseUp") {
    return {
      gesture: undefined,
      primaryClick:
        mouse.button !== "right" &&
        mouse.button !== "middle" &&
        !gesture.moved,
    };
  }
  if (mouse.type === "mouseLeave" || mouse.type === "contextMenu") {
    return { gesture: undefined, primaryClick: false };
  }
  return { gesture, primaryClick: false };
}
