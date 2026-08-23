import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import ProjectFileIcon from "./ProjectFileIcon";

describe("ProjectFileIcon", () => {
  // 断言的是图标层的类名（aw-icon--<图形名>），而不是第三方库的类名。
  // 文件类型族共用同一个文件外形，因此这里真正校验的是「类型映射到了正确的内标」。
  it.each([
    ["word", "aw-icon--file-list"],
    ["text", "aw-icon--file-list"],
    ["excel", "aw-icon--table"],
    ["powerpoint", "aw-icon--display-play"],
    ["pdf", "aw-icon--file-pdf"],
    ["markdown", "aw-icon--file-markdown"],
    ["code", "aw-icon--file-code"],
    ["data", "aw-icon--file-data"],
    ["image", "aw-icon--file-image"],
    ["archive", "aw-icon--file-archive"],
    ["audio", "aw-icon--file-audio"],
    ["video", "aw-icon--file-video"],
    ["file", "aw-icon--file"],
    ["symlink", "aw-icon--symlink"],
    ["folder", "aw-icon--folder"],
  ] as const)("renders the %s file-type icon", (kind, expectedClass) => {
    expect(renderToStaticMarkup(<ProjectFileIcon kind={kind} />)).toContain(
      expectedClass,
    );
  });
});
