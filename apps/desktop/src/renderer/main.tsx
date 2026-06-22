import React from "react";
import ReactDOM from "react-dom/client";
import "@awaken/design-system/tokens/tokens.css";
import "@awaken/design-system/tokens/components.css";
import "./styles.css";
import { App } from "./App";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
