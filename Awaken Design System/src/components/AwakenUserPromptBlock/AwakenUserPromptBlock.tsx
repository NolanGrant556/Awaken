import "./AwakenUserPromptBlock.css";
export type AwakenUserPromptBlockProps = {
  text: string;
};

export function AwakenUserPromptBlock({ text }: AwakenUserPromptBlockProps) {
  return (
    <article className="execution-prompt-block">
      <p>{text}</p>
    </article>
  );
}
