import "./AwakenComposerDock.css";
import type { ReactNode } from "react";

export type AwakenComposerDockProps = {
  children: ReactNode;
};

export function AwakenComposerDock({ children }: AwakenComposerDockProps) {
  return <div className="aw-composer-dock">{children}</div>;
}
