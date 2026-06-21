import type { ReactNode, Ref, UIEventHandler } from "react";

type RunStageProps = {
  children: ReactNode;
  composer?: ReactNode;
  scrollRef?: Ref<HTMLDivElement>;
  onScroll?: UIEventHandler<HTMLDivElement>;
};

export function RunStage({
  children,
  composer,
  scrollRef,
  onScroll,
}: RunStageProps) {
  return (
    <main className="aw-run-stage" aria-label="Run stage">
      <div className="aw-run-stage-scroll" ref={scrollRef} onScroll={onScroll}>
        {children}
      </div>
      {composer}
    </main>
  );
}
