import { Children, type ReactNode } from "react";

type NarrativeFeedProps = {
  children?: ReactNode;
  label?: string;
  emptyTitle?: string;
  emptyDetail?: string;
};

export function NarrativeFeed({
  children,
  label = "Run narrative",
  emptyTitle: _emptyTitle = "",
  emptyDetail: _emptyDetail = "",
}: NarrativeFeedProps) {
  const renderedChildren = Children.toArray(children);
  const hasChildren = renderedChildren.length > 0;

  return (
    <section className="narrative-list" aria-label={label}>
      {hasChildren ? renderedChildren : null}
    </section>
  );
}
