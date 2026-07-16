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
  renderMermaid?: boolean;
  renderImages?: boolean;
  allowedLinkProtocols?: readonly string[];
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
  renderMermaid = true,
  renderImages = true,
  allowedLinkProtocols,
}: AwakenMarkdownRendererProps) {
  const source = markdown || fallback;
  const closedMermaidSources = useMemo(
    () => renderMermaid ? findClosedMermaidSources(source) : new Set<string>(),
    [renderMermaid, source],
  );
  const markdownComponents = useMemo<Components>(
    () => ({
      a({ node: _node, children, href, ...props }) {
        if (allowedLinkProtocols) {
          let protocol = "";
          try {
            protocol = new URL(href ?? "").protocol.replace(/:$/, "").toLowerCase();
          } catch {
            return <span>{children}</span>;
          }
          if (!allowedLinkProtocols.includes(protocol)) {
            return <span>{children}</span>;
          }
        }
        return (
          <a {...props} href={href} target="_blank" rel="noreferrer">
            {children}
          </a>
        );
      },
      img({ node: _node, alt, ...props }) {
        if (!renderImages) {
          return alt ? <span className="aw-markdown-image-alt">[{alt}]</span> : null;
        }
        return <img {...props} alt={alt ?? ""} />;
      },
      input({ node: _node, type, ...props }) {
        return <input {...props} type={type} disabled={type === "checkbox" || props.disabled} />;
      },
      li({ node: _node, className: listItemClassName, children, ...props }) {
        const childArray = Children.toArray(children);
        const checkboxIndex = childArray.findIndex(
          (child) =>
            isValidElement<{ type?: string }>(child) && child.props.type === "checkbox",
        );
        const isTaskListItem = listItemClassName
          ?.split(/\s+/)
          .includes("task-list-item");

        if (!isTaskListItem || checkboxIndex < 0) {
          return (
            <li {...props} className={listItemClassName}>
              {children}
            </li>
          );
        }

        const checkbox = childArray[checkboxIndex];
        const taskContent = childArray.filter((_, index) => index !== checkboxIndex);
        return (
          <li {...props} className={listItemClassName}>
            {checkbox}
            <div className="assistant-message-task-content">{taskContent}</div>
          </li>
        );
      },
      code({ node: _node, className: codeClassName, children, ...props }) {
        const language = languageFromClassName(codeClassName);
        const codeText = textFromNode(children).replace(/\n$/, "");
        const isClosedMermaid =
          renderMermaid &&
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
    [allowedLinkProtocols, closedMermaidSources, renderImages, renderMermaid],
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
