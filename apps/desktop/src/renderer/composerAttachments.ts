import type { RunInputAttachment, RuntimeModelProfileSummary } from "../shared/runtime";

export const MAX_COMPOSER_IMAGE_ATTACHMENTS = 4;
export const MAX_COMPOSER_IMAGE_BYTES = 8 * 1024 * 1024;
export const SUPPORTED_COMPOSER_IMAGE_MIME_TYPES = [
  "image/png",
  "image/jpeg",
  "image/webp",
] as const;

export type ComposerDraftAttachment = RunInputAttachment & {
  previewURL: string;
};

export async function readComposerDraftImageAttachment(
  file: File,
  index: number,
): Promise<ComposerDraftAttachment> {
  const dataURL = await readFileAsDataURL(file);
  const dataBase64 = dataURL.split(",", 2)[1] ?? "";
  const dimensions = await imageDimensionsFromDataURL(dataURL);
  const extension = file.type === "image/png"
    ? "png"
    : file.type === "image/webp"
      ? "webp"
      : "jpg";
  return {
    id: crypto.randomUUID(),
    kind: "image",
    mimeType: file.type,
    dataBase64,
    displayName: file.name?.trim() || `pasted-image-${index + 1}.${extension}`,
    width: dimensions.width,
    height: dimensions.height,
    byteSize: file.size,
    previewURL: dataURL,
  };
}

export function toRunInputAttachment(
  attachment: ComposerDraftAttachment,
): RunInputAttachment {
  return {
    id: attachment.id,
    kind: attachment.kind,
    mimeType: attachment.mimeType,
    path: attachment.path,
    dataBase64: attachment.dataBase64,
    displayName: attachment.displayName,
    width: attachment.width,
    height: attachment.height,
    byteSize: attachment.byteSize,
  };
}

export function previewURLForRunInputAttachment(
  attachment: Pick<RunInputAttachment, "mimeType" | "dataBase64">,
): string {
  return attachment.dataBase64
    ? `data:${attachment.mimeType};base64,${attachment.dataBase64}`
    : "";
}

export function promptAttachmentViews(
  attachments: RunInputAttachment[] | undefined,
): Array<{ id: string; src: string; alt: string }> {
  if (!attachments || attachments.length === 0) {
    return [];
  }
  return attachments
    .filter((attachment) => (attachment.dataBase64 ?? "").trim().length > 0)
    .map((attachment) => ({
      id: attachment.id,
      src: previewURLForRunInputAttachment(attachment),
      alt: attachment.displayName || "Prompt image",
    }));
}

export function supportsVisionForProfile(
  profile: RuntimeModelProfileSummary | undefined,
): boolean {
  return profile?.advanced?.supportsVision === true;
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("Failed to read image"));
    reader.onload = () => {
      const result = reader.result;
      if (typeof result !== "string") {
        reject(new Error("Failed to read image"));
        return;
      }
      resolve(result);
    };
    reader.readAsDataURL(file);
  });
}

function imageDimensionsFromDataURL(dataURL: string): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      resolve({
        width: image.naturalWidth,
        height: image.naturalHeight,
      });
    };
    image.onerror = () => reject(new Error("Failed to decode image"));
    image.src = dataURL;
  });
}
