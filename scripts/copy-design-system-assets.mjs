import { copyFile, mkdir, readdir } from "node:fs/promises";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..", "Awaken Design System");

async function copyDirectoryContents(sourceDir, targetDir) {
  await mkdir(targetDir, { recursive: true });

  let entries;
  try {
    entries = await readdir(sourceDir, { withFileTypes: true });
  } catch (error) {
    if (error?.code === "ENOENT") {
      return;
    }
    throw error;
  }

  await Promise.all(
    entries
      .filter((entry) => entry.isFile())
      .map((entry) => copyFile(join(sourceDir, entry.name), join(targetDir, entry.name))),
  );
}

await Promise.all([
  copyDirectoryContents(join(root, "src", "tokens"), join(root, "dist", "tokens")),
  copyDirectoryContents(join(root, "src", "assets"), join(root, "dist", "assets")),
]);
