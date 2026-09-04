export function shouldRenderWorkbenchNarrative({
  showCenterComposer,
  fileWorkspaceSurfaceActive,
}: {
  showCenterComposer: boolean;
  fileWorkspaceSurfaceActive: boolean;
}): boolean {
  return !showCenterComposer && !fileWorkspaceSurfaceActive;
}

export function shouldShowRunPlanSurface({
  fileWorkspaceSurfaceActive,
  mediaPreviewActive,
}: {
  fileWorkspaceSurfaceActive: boolean;
  mediaPreviewActive: boolean;
}): boolean {
  return !fileWorkspaceSurfaceActive && !mediaPreviewActive;
}
