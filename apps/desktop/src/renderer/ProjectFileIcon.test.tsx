import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import ProjectFileIcon from "./ProjectFileIcon";

describe("ProjectFileIcon", () => {
  it.each([
    ["word", "lucide-file-text"],
    ["excel", "lucide-file-spreadsheet"],
    ["powerpoint", "lucide-presentation"],
    ["pdf", "lucide-file-type2"],
    ["markdown", "lucide-file-type"],
    ["code", "lucide-file-code2"],
    ["image", "lucide-file-image"],
  ] as const)("renders the %s file-type icon", (kind, expectedClass) => {
    expect(renderToStaticMarkup(<ProjectFileIcon kind={kind} />)).toContain(
      expectedClass,
    );
  });
});
