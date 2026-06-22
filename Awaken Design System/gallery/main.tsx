import React from "react";
import { createRoot } from "react-dom/client";
import "../src/tokens/tokens.css";
import "./styles.css";
import { AwakenGalleryApp } from "./AwakenGalleryApp";

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AwakenGalleryApp />
  </React.StrictMode>,
);
