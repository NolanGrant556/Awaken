import "./AwakenAssistantMessage.css";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";

type AssistantMessageStatus = "active" | "done" | "error";

export type AwakenAssistantMessageProps = {
  markdown: string;
  status?: AssistantMessageStatus;
};

export function AwakenAssistantMessage({
  markdown,
  status = "active",
}: AwakenAssistantMessageProps) {
  return (
    <article className={`assistant-message assistant-message-${status}`}>
      <AwakenMarkdownRenderer className="assistant-message-markdown" markdown={markdown} />
    </article>
  );
}
