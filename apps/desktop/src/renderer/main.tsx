import React from "react";
import ReactDOM from "react-dom/client";
import { installAwakenFocusModality } from "@awaken/design-system";
import "@awaken/design-system/tokens/tokens.css";
import "@awaken/design-system/tokens/components.css";
import "@awaken/design-system/index.css";
import "./styles.css";
import "./styles/workbench.css";
import { App } from "./App";

installAwakenFocusModality(document.documentElement);

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
