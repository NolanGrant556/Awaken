import "./AwakenNarrativeFeed.css";
import { Children, type ReactNode } from "react";

export type AwakenNarrativeFeedProps = {
  children?: ReactNode;
  label?: string;
  emptyTitle?: string;
  emptyDetail?: string;
};

export function AwakenNarrativeFeed({
  children,
  label = "Run narrative",
  emptyTitle: _emptyTitle = "",
  emptyDetail: _emptyDetail = "",
}: AwakenNarrativeFeedProps) {
  const renderedChildren = Children.toArray(children);
  const hasChildren = renderedChildren.length > 0;

  return (
    <section className="narrative-list" aria-label={label}>
      {hasChildren ? renderedChildren : null}
    </section>
  );
}
