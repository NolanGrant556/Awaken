import React from "react";
import { createRoot } from "react-dom/client";
import { installAwakenFocusModality } from "../src/focusModality";
import "../src/tokens/tokens.css";
import "./styles.css";
import { AwakenGalleryApp } from "./AwakenGalleryApp";

installAwakenFocusModality(document.documentElement);

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AwakenGalleryApp />
  </React.StrictMode>,
);
