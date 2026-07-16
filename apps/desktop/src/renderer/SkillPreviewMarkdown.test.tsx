import { AwakenMarkdownRenderer } from "@awaken/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

describe("Skill preview Markdown policy", () => {
  it("renders GFM while keeping active content, images, Mermaid, and unsafe links inert", () => {
    const html = renderToStaticMarkup(
      <AwakenMarkdownRenderer
        markdown={`# Heading

- [x] done
- [ ] 无公式错误
- [ ] 计算用公式而非写死数字（除非用户只要静态表）
- [ ] 列宽/表头可读，中文不乱码
- [ ] 最终文件位于任务工作区内
- [ ] 已 **\`artifact_publish\`**

| A | B |
| - | - |
| 1 | 2 |

> quote

\`\`\`mermaid
graph TD
A-->B
\`\`\`

<script>globalThis.skillPreviewExecuted = true</script>

![remote](https://example.com/tracker.png)

[safe](https://example.com) [relative](./resource.md) [unsafe](javascript:alert(1))
`}
        fallback=""
        renderMermaid={false}
        renderImages={false}
        allowedLinkProtocols={["http", "https"]}
      />,
    );

    expect(html).toContain("<h1>Heading</h1>");
    expect(html).toContain("<table");
    expect(html).toContain("<blockquote>");
    expect(html).toContain("type=\"checkbox\"");
    expect(html).toContain("disabled=\"\"");
    expect(html).toContain("assistant-message-task-content");
    expect(html).toMatch(
      /task-list-item.*checkbox.*assistant-message-task-content.*已.*artifact_publish.*<\/div><\/li>/,
    );
    expect(html).toContain("language-mermaid");
    expect(html).not.toContain("assistant-mermaid-diagram");
    expect(html).not.toContain("<script");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("tracker.png");
    expect(html).toContain("[remote]");
    expect(html).toContain("href=\"https://example.com\"");
    expect(html).not.toContain("href=\"./resource.md\"");
    expect(html).not.toContain("javascript:");
  });
});
