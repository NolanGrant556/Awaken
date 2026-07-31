import { describe, expect, it } from "vitest";
import { projectFileIconKind, projectFileTreeRows } from "./projectFiles";

const listings = {
  "": [
    { name: "src", relativePath: "src", kind: "directory" as const },
    { name: "README.md", relativePath: "README.md", kind: "file" as const },
  ],
  src: [
    {
      name: "components",
      relativePath: "src/components",
      kind: "directory" as const,
    },
    {
      name: "index.ts",
      relativePath: "src/index.ts",
      kind: "file" as const,
    },
  ],
  "src/components": [
    {
      name: "Button.tsx",
      relativePath: "src/components/Button.tsx",
      kind: "file" as const,
    },
  ],
};

describe("project file tree rows", () => {
  it("maps common document, media, archive, and code formats to distinct icons", () => {
    const kindFor = (name: string) =>
      projectFileIconKind({ name, kind: "file" });

    expect(kindFor("report.docx")).toBe("word");
    expect(kindFor("analysis.XLSX")).toBe("excel");
    expect(kindFor("brief.pptx")).toBe("powerpoint");
    expect(kindFor("manual.pdf")).toBe("pdf");
    expect(kindFor("README.md")).toBe("markdown");
    expect(kindFor("App.tsx")).toBe("code");
    expect(kindFor("package.json")).toBe("data");
    expect(kindFor("hero.png")).toBe("image");
    expect(kindFor("source.zip")).toBe("archive");
    expect(kindFor("voice.mp3")).toBe("audio");
    expect(kindFor("demo.mp4")).toBe("video");
    expect(kindFor("notes.txt")).toBe("text");
    expect(kindFor("Dockerfile")).toBe("code");
    expect(kindFor("go.mod")).toBe("code");
    expect(kindFor("go.sum")).toBe("code");
    expect(kindFor(".gitignore")).toBe("code");
    expect(kindFor("unknown.bin")).toBe("file");
  });

  it("shows only descendants of expanded directories", () => {
    const rows = projectFileTreeRows({
      listings,
      expandedPaths: new Set(["src"]),
      selectedPath: "src/index.ts",
      filter: "",
    });

    expect(rows.map((row) => [row.relativePath, row.depth, row.selected])).toEqual([
      ["src", 0, false],
      ["src/components", 1, false],
      ["src/index.ts", 1, true],
      ["README.md", 0, false],
    ]);
  });

  it("keeps matching ancestors when filtering loaded descendants", () => {
    const rows = projectFileTreeRows({
      listings,
      expandedPaths: new Set(),
      filter: "button",
    });

    expect(rows.map((row) => [row.relativePath, row.depth])).toEqual([
      ["src", 0],
      ["src/components", 1],
      ["src/components/Button.tsx", 2],
    ]);
  });
});
