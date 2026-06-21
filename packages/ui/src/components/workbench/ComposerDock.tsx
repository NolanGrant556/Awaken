import type { ReactNode } from "react";

type ComposerDockProps = {
  children: ReactNode;
};

export function ComposerDock({ children }: ComposerDockProps) {
  return <div className="aw-composer-dock">{children}</div>;
}
