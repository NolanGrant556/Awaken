const GLOB_FILE_TYPE_LABELS: Record<string, string> = {
  csv: "CSV",
  css: "CSS",
  doc: "Word",
  docx: "Word",
  gif: "图片",
  go: "Go",
  htm: "HTML",
  html: "HTML",
  jpeg: "图片",
  jpg: "图片",
  js: "JavaScript",
  json: "JSON",
  jsx: "JSX",
  markdown: "Markdown",
  md: "Markdown",
  pdf: "PDF",
  png: "图片",
  ppt: "PowerPoint",
  pptx: "PowerPoint",
  py: "Python",
  svg: "SVG",
  ts: "TypeScript",
  tsx: "TSX",
  txt: "文本",
  webp: "图片",
  xls: "Excel",
  xlsx: "Excel",
  yaml: "YAML",
  yml: "YAML",
};

function compactPathLabel(value: string): string {
  const normalized = value.trim().replace(/[\\/]+$/, "");
  const segments = normalized.split(/[\\/]/).filter(Boolean);
  const label = segments.at(-1) || normalized;
  return label.length > 72 ? `${label.slice(0, 71).trimEnd()}…` : label;
}

export function describeGlobPattern(value: string): string {
  const normalized = value.trim().replace(/\\/g, "/");
  if (!normalized) return "文件";
  if (["*", "**", "**/*"].includes(normalized)) return "所有文件";

  const segments = normalized.split("/").filter(Boolean);
  const filePattern = segments.at(-1) || normalized;
  const literalScope = segments
    .slice(0, -1)
    .filter((segment) => !/[*?[\]{}]/.test(segment))
    .at(-1);
  const braceExtensions = filePattern.match(/^\*\.\{([^{}]+)\}$/)?.[1]
    ?.split(",")
    .map((extension) => extension.trim().toLowerCase())
    .filter(Boolean);
  const singleExtension = filePattern.match(/^\*\.([a-zA-Z0-9]+)$/)?.[1]?.toLowerCase();
  const extensions = braceExtensions?.length
    ? braceExtensions
    : singleExtension
      ? [singleExtension]
      : [];

  let description: string;
  if (extensions.length > 0) {
    const labels = [
      ...new Set(
        extensions.map(
          (extension) => GLOB_FILE_TYPE_LABELS[extension] ?? extension.toUpperCase(),
        ),
      ),
    ];
    description = `${labels.join("、")} 文件`;
  } else if (!/[*?[\]{}]/.test(filePattern)) {
    description = compactPathLabel(filePattern);
  } else {
    description = "匹配文件";
  }

  return literalScope
    ? `${compactPathLabel(literalScope)} 中的 ${description}`
    : description;
}
