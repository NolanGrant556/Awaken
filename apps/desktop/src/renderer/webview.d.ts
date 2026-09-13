import type { DetailedHTMLProps, HTMLAttributes } from "react";
import type { WebviewTag } from "electron";

declare module "react" {
  namespace JSX {
    interface IntrinsicElements {
      webview: DetailedHTMLProps<HTMLAttributes<WebviewTag>, WebviewTag> & {
        src: string;
        partition?: string;
        plugins?: true;
        allowpopups?: string | boolean;
        webpreferences?: string;
      };
    }
  }
}
