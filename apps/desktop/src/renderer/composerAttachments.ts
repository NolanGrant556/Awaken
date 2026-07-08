import type { PickedAttachmentFile, RunInputAttachment } from "../shared/runtime";

export const MAX_COMPOSER_IMAGE_BYTES = 8 * 1024 * 1024;
export const SUPPORTED_COMPOSER_IMAGE_MIME_TYPES = [
  "image/png",
  "image/jpeg",
  "image/webp",
] as const;

export type ComposerDraftAttachment = RunInputAttachment & {
  previewURL?: string;
};

export type AttachmentView = {
  id: string;
  kind: "image" | "file";
  src?: string;
  alt: string;
  label: string;
  detail: string;
  mimeType: string;
  byteSize?: number;
  path?: string;
  tooltip: string;
  extension: string;
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

export function composerDraftAttachmentFromPickedFile(
  file: PickedAttachmentFile,
): ComposerDraftAttachment {
  return {
    id: file.id || crypto.randomUUID(),
    kind: file.kind,
    path: file.path,
    mimeType: file.mimeType,
    dataBase64: file.dataBase64,
    displayName: file.displayName,
    width: file.width,
    height: file.height,
    byteSize: file.byteSize,
    previewURL: file.kind === "image" && file.dataBase64
      ? previewURLForRunInputAttachment(file)
      : undefined,
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
): AttachmentView[] {
  if (!attachments || attachments.length === 0) {
    return [];
  }
  return attachments.map((attachment) =>
    attachmentViewFromRunInputAttachment(attachment),
  );
}

export function composerAttachmentViews(
  attachments: ComposerDraftAttachment[],
): AttachmentView[] {
  return attachments.map((attachment) =>
    attachmentViewFromRunInputAttachment(attachment, attachment.previewURL),
  );
}

function attachmentViewFromRunInputAttachment(
  attachment: RunInputAttachment,
  previewURL?: string,
): AttachmentView {
  const label = attachment.displayName?.trim() ||
    lastPathSegment(attachment.path) ||
    (attachment.kind === "image" ? "Image attachment" : "File attachment");
  const byteSize = formatAttachmentByteSize(attachment.byteSize);
  const typeLabel = attachmentTypeLabel(attachment.mimeType, attachment.path);
  const pathTail = attachmentPathTail(attachment.path);
  const detail = [typeLabel, byteSize].filter(Boolean).join(" · ");
  const tooltip = [
    label,
    typeLabel ? `类型: ${typeLabel}` : "",
    byteSize ? `大小: ${byteSize}` : "",
    pathTail ? `路径: ${pathTail}` : "",
  ].filter(Boolean).join("\n");
  return {
    id: attachment.id,
    kind: attachment.kind,
    src: attachment.kind === "image"
      ? previewURL || previewURLForRunInputAttachment(attachment)
      : undefined,
    alt: label,
    label,
    detail,
    mimeType: attachment.mimeType,
    byteSize: attachment.byteSize,
    path: attachment.path,
    tooltip,
    extension: attachmentExtension(label, attachment.mimeType),
  };
}

function attachmentTypeLabel(mimeType?: string, filePath?: string): string {
  const lowerMime = mimeType?.toLowerCase().trim() ?? "";
  if (lowerMime === "application/pdf") return "PDF";
  if (lowerMime.includes("spreadsheet") || lowerMime.includes("excel")) {
    return "Excel";
  }
  if (lowerMime.includes("wordprocessing") || lowerMime.includes("msword")) {
    return "Word";
  }
  if (lowerMime === "text/markdown") return "Markdown";
  if (lowerMime.startsWith("image/")) {
    return lowerMime.replace("image/", "").toUpperCase();
  }
  if (lowerMime.startsWith("text/")) {
    return lowerMime.replace("text/", "").toUpperCase();
  }
  const ext = attachmentExtension(filePath ?? "", lowerMime);
  return ext ? ext.toUpperCase() : lowerMime || "File";
}

function attachmentExtension(name: string, mimeType?: string): string {
  const basename = lastPathSegment(name) || name;
  const match = /\.([^.]+)$/.exec(basename);
  if (match) {
    return match[1].slice(0, 5).toUpperCase();
  }
  const lowerMime = mimeType?.toLowerCase().trim() ?? "";
  if (lowerMime === "application/pdf") return "PDF";
  if (lowerMime.includes("spreadsheet") || lowerMime.includes("excel")) return "XLS";
  if (lowerMime.includes("wordprocessing") || lowerMime.includes("msword")) return "DOC";
  if (lowerMime.startsWith("image/")) {
    return lowerMime.replace("image/", "").slice(0, 5).toUpperCase();
  }
  return "";
}

function formatAttachmentByteSize(byteSize?: number): string {
  if (!byteSize || !Number.isFinite(byteSize) || byteSize <= 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB"];
  let size = byteSize;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const digits = size >= 10 || unitIndex === 0 ? 0 : 1;
  return `${size.toFixed(digits)} ${units[unitIndex]}`;
}

function attachmentPathTail(filePath?: string): string {
  if (!filePath) {
    return "";
  }
  const parts = filePath.split(/[\\/]/).filter(Boolean);
  if (parts.length <= 3) {
    return filePath;
  }
  return `.../${parts.slice(-3).join("/")}`;
}

function lastPathSegment(filePath?: string): string {
  if (!filePath) {
    return "";
  }
  const parts = filePath.split(/[\\/]/).filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1] : "";
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
