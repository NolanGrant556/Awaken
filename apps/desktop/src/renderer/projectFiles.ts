import type { ProjectDirectoryEntry } from "../shared/runtime";

export type ProjectFileIconKind =
  | "folder"
  | "symlink"
  | "word"
  | "excel"
  | "powerpoint"
  | "pdf"
  | "markdown"
  | "code"
  | "data"
  | "image"
  | "archive"
  | "audio"
  | "video"
  | "text"
  | "file";

const fileIconExtensions: ReadonlyArray<
  readonly [ProjectFileIconKind, ReadonlySet<string>]
> = [
  ["word", new Set(["doc", "docx", "dot", "dotx", "odt", "rtf"])],
  [
    "excel",
    new Set(["csv", "ods", "tsv", "xls", "xlsb", "xlsm", "xlsx"]),
  ],
  ["powerpoint", new Set(["key", "odp", "ppt", "pptm", "pptx"])],
  ["pdf", new Set(["pdf"])],
  ["markdown", new Set(["markdown", "md", "mdx"])],
  [
    "code",
    new Set([
      "astro",
      "bash",
      "c",
      "cc",
      "cpp",
      "cs",
      "css",
      "cxx",
      "fish",
      "go",
      "h",
      "hpp",
      "htm",
      "html",
      "java",
      "js",
      "jsx",
      "kt",
      "kts",
      "less",
      "lua",
      "mjs",
      "mod",
      "php",
      "py",
      "rb",
      "rs",
      "sass",
      "scss",
      "sh",
      "sql",
      "svelte",
      "sum",
      "swift",
      "toml",
      "ts",
      "tsx",
      "vue",
      "xml",
      "yaml",
      "yml",
      "zsh",
    ]),
  ],
  ["data", new Set(["json", "json5", "jsonc", "ndjson"])],
  [
    "image",
    new Set(["avif", "bmp", "gif", "heic", "ico", "jpeg", "jpg", "png", "svg", "tif", "tiff", "webp"]),
  ],
  [
    "archive",
    new Set(["7z", "bz2", "gz", "rar", "tar", "tgz", "xz", "zip"]),
  ],
  ["audio", new Set(["aac", "flac", "m4a", "mp3", "ogg", "wav"])],
  ["video", new Set(["avi", "m4v", "mkv", "mov", "mp4", "webm"])],
  ["text", new Set(["log", "text", "txt"])],
];

const extensionlessCodeFiles = new Set([
  ".editorconfig",
  ".gitattributes",
  ".gitignore",
  ".npmrc",
  "cmakelists.txt",
  "dockerfile",
  "gemfile",
  "justfile",
  "makefile",
  "procfile",
  "rakefile",
]);

export function projectFileIconKind(
  entry: Pick<ProjectDirectoryEntry, "kind" | "name">,
): ProjectFileIconKind {
  if (entry.kind === "directory") return "folder";
  if (entry.kind === "symlink") return "symlink";

  const normalizedName = entry.name.toLocaleLowerCase();
  if (extensionlessCodeFiles.has(normalizedName)) return "code";
  const dot = normalizedName.lastIndexOf(".");
  const extension =
    dot > 0 && dot < normalizedName.length - 1
      ? normalizedName.slice(dot + 1)
      : "";
  for (const [kind, extensions] of fileIconExtensions) {
    if (extensions.has(extension)) return kind;
  }
  return "file";
}

export type ProjectFileTreeRow = ProjectDirectoryEntry & {
  depth: number;
  expanded: boolean;
  selected: boolean;
};

type ProjectDirectoryListings = Record<string, ProjectDirectoryEntry[]>;

function normalizedSearchValue(value: string): string {
  return value.trim().toLocaleLowerCase();
}

function filterRows(
  entries: ProjectDirectoryEntry[],
  listings: ProjectDirectoryListings,
  selectedPath: string | undefined,
  query: string,
  depth: number,
): ProjectFileTreeRow[] {
  const rows: ProjectFileTreeRow[] = [];
  for (const entry of entries) {
    const descendantRows =
      entry.kind === "directory" && listings[entry.relativePath]
        ? filterRows(
            listings[entry.relativePath],
            listings,
            selectedPath,
            query,
            depth + 1,
          )
        : [];
    const matches = normalizedSearchValue(
      `${entry.name} ${entry.relativePath}`,
    ).includes(query);
    if (!matches && descendantRows.length === 0) {
      continue;
    }
    rows.push({
      ...entry,
      depth,
      expanded: descendantRows.length > 0,
      selected: entry.relativePath === selectedPath,
    });
    rows.push(...descendantRows);
  }
  return rows;
}

function visibleRows(
  entries: ProjectDirectoryEntry[],
  listings: ProjectDirectoryListings,
  expandedPaths: ReadonlySet<string>,
  selectedPath: string | undefined,
  depth: number,
): ProjectFileTreeRow[] {
  const rows: ProjectFileTreeRow[] = [];
  for (const entry of entries) {
    const expanded =
      entry.kind === "directory" && expandedPaths.has(entry.relativePath);
    rows.push({
      ...entry,
      depth,
      expanded,
      selected: entry.relativePath === selectedPath,
    });
    if (expanded && listings[entry.relativePath]) {
      rows.push(
        ...visibleRows(
          listings[entry.relativePath],
          listings,
          expandedPaths,
          selectedPath,
          depth + 1,
        ),
      );
    }
  }
  return rows;
}

/** Creates the flat, accessible row model rendered by the project file tree. */
export function projectFileTreeRows(args: {
  listings: ProjectDirectoryListings;
  expandedPaths: ReadonlySet<string>;
  selectedPath?: string;
  filter: string;
}): ProjectFileTreeRow[] {
  const rootEntries = args.listings[""] ?? [];
  const query = normalizedSearchValue(args.filter);
  if (query) {
    return filterRows(
      rootEntries,
      args.listings,
      args.selectedPath,
      query,
      0,
    );
  }
  return visibleRows(
    rootEntries,
    args.listings,
    args.expandedPaths,
    args.selectedPath,
    0,
  );
}
