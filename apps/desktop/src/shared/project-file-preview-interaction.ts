export const PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL =
  "awaken:project-file-preview-interaction";

export type ProjectFilePreviewInteraction = {
  previewId: string;
  type: "primary-click";
};
