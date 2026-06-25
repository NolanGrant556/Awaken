import { Children, isValidElement, useMemo, type ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import { AwakenMermaidDiagram } from "../AwakenMermaidDiagram";
import "./AwakenMarkdownRenderer.css";

export type AwakenMarkdownRendererProps = {
  markdown: string;
  fallback?: string;
  className?: string;
};

const closedMermaidFencePattern =
  /(?:^|\n)(`{3,}|~{3,})[^\S\n]*mermaid[^\n]*\n([\s\S]*?)(?:\n\1[^\S\n]*(?=\n|$))/gi;

function normalizeMermaidSource(source: string): string {
  return source.trim();
}

function findClosedMermaidSources(markdown: string): Set<string> {
  const sources = new Set<string>();
  for (const match of markdown.matchAll(closedMermaidFencePattern)) {
    sources.add(normalizeMermaidSource(match[2] ?? ""));
  }
  return sources;
}

function textFromNode(children: ReactNode): string {
  if (Array.isArray(children)) {
    return children.map((child) => textFromNode(child)).join("");
  }
  if (typeof children === "string" || typeof children === "number") {
    return String(children);
  }
  return "";
}

function languageFromClassName(className?: string): string | undefined {
  return className
    ?.split(/\s+/)
    .map((entry) => entry.match(/^language-(.+)$/)?.[1])
    .find(Boolean)
    ?.toLowerCase();
}

function onlyChild(children: ReactNode): ReactNode | undefined {
  const childArray = Children.toArray(children);
  return childArray.length === 1 ? childArray[0] : undefined;
}

export function AwakenMarkdownRenderer({
  markdown,
  fallback = "Streaming response...",
  className,
}: AwakenMarkdownRendererProps) {
  const source = markdown || fallback;
  const closedMermaidSources = useMemo(() => findClosedMermaidSources(source), [source]);
  const markdownComponents = useMemo<Components>(
    () => ({
      a({ node: _node, children, ...props }) {
        return (
          <a {...props} target="_blank" rel="noreferrer">
            {children}
          </a>
        );
      },
      code({ node: _node, className: codeClassName, children, ...props }) {
        const language = languageFromClassName(codeClassName);
        const codeText = textFromNode(children).replace(/\n$/, "");
        const isClosedMermaid =
          language === "mermaid" &&
          closedMermaidSources.has(normalizeMermaidSource(codeText));

        if (isClosedMermaid) {
          return <AwakenMermaidDiagram chart={codeText} />;
        }

        const nextClassName = codeClassName
          ? `assistant-message-code ${codeClassName}`
          : "assistant-message-code";
        return (
          <code {...props} className={nextClassName}>
            {children}
          </code>
        );
      },
      pre({ node: _node, children, ...props }) {
        const child = onlyChild(children);
        if (isValidElement(child) && child.type === AwakenMermaidDiagram) {
          return child;
        }
        return (
          <pre {...props} className="assistant-message-code-block">
            {children}
          </pre>
        );
      },
      table({ node: _node, children, ...props }) {
        return (
          <div className="assistant-message-table-wrap">
            <table {...props} className="assistant-message-table">
              {children}
            </table>
          </div>
        );
      },
    }),
    [closedMermaidSources],
  );

  return (
    <div className={className}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSanitize]}
        skipHtml
        components={markdownComponents}
      >
        {source}
      </ReactMarkdown>
    </div>
  );
}
