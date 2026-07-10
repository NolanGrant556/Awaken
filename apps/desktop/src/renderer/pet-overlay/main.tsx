import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { PetOverlayApp } from "./PetOverlayApp";
import "./pet-overlay.css";

const container = document.getElementById("root");
if (!container) {
  throw new Error("Pet overlay root element not found");
}

const root = createRoot(container);
root.render(
  <StrictMode>
    <PetOverlayApp />
  </StrictMode>,
);
