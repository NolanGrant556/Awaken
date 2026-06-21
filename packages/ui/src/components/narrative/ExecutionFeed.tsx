import { Children, type ReactNode } from "react";

type ExecutionFeedProps = {
  children?: ReactNode;
  label?: string;
};

export function ExecutionFeed({
  children,
  label = "Agent execution narrative",
}: ExecutionFeedProps) {
  const renderedChildren = Children.toArray(children).filter(Boolean);

  return (
    <section
      className={`execution-feed${
        renderedChildren.length === 0 ? " execution-feed-empty" : ""
      }`}
      aria-label={label}
    >
      {renderedChildren}
    </section>
  );
}
