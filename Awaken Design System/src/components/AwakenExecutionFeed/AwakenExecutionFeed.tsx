import "./AwakenExecutionFeed.css";
import { Children, type ReactNode } from "react";

export type AwakenExecutionFeedProps = {
  children?: ReactNode;
  label?: string;
};

export function AwakenExecutionFeed({
  children,
  label = "Agent execution narrative",
}: AwakenExecutionFeedProps) {
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
