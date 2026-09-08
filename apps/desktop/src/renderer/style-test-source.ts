import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

/** Resolve local stylesheet imports so contract checks do not depend on file placement. */
export function readStyleSource(path: URL | string): string {
  const url = typeof path === "string" ? pathToFileURL(path) : path;
  return readFileSync(url, "utf8").replace(/@import\s+["']([^"']+)["'];/g,
    (_match, relative: string) => readStyleSource(new URL(relative, url)));
}
