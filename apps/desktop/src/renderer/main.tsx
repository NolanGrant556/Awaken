import React from "react";
import ReactDOM from "react-dom/client";
import { installAwakenFocusModality } from "@awaken/design-system";
import "@awaken/design-system/tokens/tokens.css";
import "@awaken/design-system/index.css";
import "./styles.css";
import "./styles/workbench.css";
import { App } from "./App";
import { DesktopErrorBoundary } from "./DesktopErrorBoundary";
import { installDesktopKeyboardPolicy } from "./desktop-keyboard-policy";

installDesktopKeyboardPolicy(document);
installAwakenFocusModality(document.documentElement);

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <DesktopErrorBoundary>
      <App />
    </DesktopErrorBoundary>
  </React.StrictMode>,
);
