import "./AwakenComposerDock.css";
import { forwardRef, type ReactNode } from "react";

export type AwakenComposerDockProps = {
  children: ReactNode;
};

export const AwakenComposerDock = forwardRef<HTMLDivElement, AwakenComposerDockProps>(
  function AwakenComposerDock({ children }, ref) {
    return (
      <div className="aw-composer-dock" ref={ref}>
        {children}
      </div>
    );
  },
);
