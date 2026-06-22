import React from "react";
import { renderToString } from "react-dom/server";
import * as AwakenDesignSystem from "../dist/index.js";

const demoEntries = Object.entries(AwakenDesignSystem)
  .filter(([name, value]) => name.startsWith("Awaken") && name.endsWith("Demo") && typeof value === "function")
  .sort(([a], [b]) => a.localeCompare(b));

if (demoEntries.length === 0) {
  throw new Error("No Awaken component demos were exported");
}

for (const [name, Demo] of demoEntries) {
  const componentName = name.replace(/Demo$/, "");
  let html = "";
  try {
    html = renderToString(React.createElement(Demo));
  } catch (error) {
    throw new Error(`Gallery smoke failed for ${componentName}: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (!html.includes(componentName)) {
    throw new Error(`Gallery smoke missing component name for ${componentName}`);
  }
}

console.log(`awaken design system render smoke passed: ${demoEntries.length} components`);
