import "./AwakenRunStage.css";
import type {
  ReactNode,
  Ref,
  UIEventHandler,
} from "react";

export type AwakenRunStageProps = {
  children: ReactNode;
  composer?: ReactNode;
  scrollRef?: Ref<HTMLDivElement>;
  onScroll?: UIEventHandler<HTMLDivElement>;
};

export function AwakenRunStage({
  children,
  composer,
  scrollRef,
  onScroll,
}: AwakenRunStageProps) {
  return (
    <main className="aw-run-stage" aria-label="Run stage">
      <div
        className="aw-run-stage-scroll"
        ref={scrollRef}
        tabIndex={-1}
        onScroll={onScroll}
      >
        {children}
      </div>
      {composer}
    </main>
  );
}
