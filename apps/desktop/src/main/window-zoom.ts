export const MAIN_WINDOW_ZOOM_LEVEL = 0;
export const MAIN_WINDOW_ZOOM_FACTOR = 1;

export type MainWindowZoomTarget = {
  setZoomLevel: (level: number) => void;
  setZoomFactor: (factor: number) => void;
  setVisualZoomLevelLimits: (minimumLevel: number, maximumLevel: number) => Promise<void>;
};

export function enforceMainWindowZoom(target: MainWindowZoomTarget): Promise<void> {
  target.setZoomLevel(MAIN_WINDOW_ZOOM_LEVEL);
  target.setZoomFactor(MAIN_WINDOW_ZOOM_FACTOR);
  return target.setVisualZoomLevelLimits(
    MAIN_WINDOW_ZOOM_FACTOR,
    MAIN_WINDOW_ZOOM_FACTOR,
  );
}
