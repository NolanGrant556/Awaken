import "./AwakenComposerDock.css";
import { forwardRef, type ReactNode } from "react";

export type AwakenComposerDockProps = {
  children: ReactNode;
  interactionInactive?: boolean;
};

export const AwakenComposerDock = forwardRef<HTMLDivElement, AwakenComposerDockProps>(
  function AwakenComposerDock({ children, interactionInactive = false }, ref) {
    return (
      <div
        className="aw-composer-dock"
        ref={ref}
        aria-hidden={interactionInactive || undefined}
        inert={interactionInactive || undefined}
      >
        {children}
      </div>
    );
  },
);
