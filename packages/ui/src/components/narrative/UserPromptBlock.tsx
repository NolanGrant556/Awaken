type UserPromptBlockProps = {
  text: string;
};

export function UserPromptBlock({ text }: UserPromptBlockProps) {
  return (
    <article className="execution-prompt-block">
      <p>{text}</p>
    </article>
  );
}
