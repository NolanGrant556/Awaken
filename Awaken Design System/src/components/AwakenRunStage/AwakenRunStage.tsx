import "./AwakenRunStage.css";
import type {
  ReactNode,
  Ref,
  UIEventHandler,
} from "react";

export type AwakenRunStageProps = {
  children: ReactNode;
  composer?: ReactNode;
  overlay?: ReactNode;
  contentInactive?: boolean;
  scrollRef?: Ref<HTMLDivElement>;
  onScroll?: UIEventHandler<HTMLDivElement>;
};

export function AwakenRunStage({
  children,
  composer,
  overlay,
  contentInactive = false,
  scrollRef,
  onScroll,
}: AwakenRunStageProps) {
  return (
    <main className="aw-run-stage" aria-label="Run stage">
      <div
        className={`aw-run-stage-scroll${
          contentInactive ? " aw-run-stage-scroll-inactive" : ""
        }`}
        ref={scrollRef}
        tabIndex={-1}
        onScroll={onScroll}
        aria-hidden={contentInactive || undefined}
        inert={contentInactive}
      >
        {children}
      </div>
      {overlay}
      {composer}
    </main>
  );
}
