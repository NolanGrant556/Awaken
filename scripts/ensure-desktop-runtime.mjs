import { execFileSync } from "node:child_process";
import { accessSync, constants } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");
const target = `${process.platform}-${process.arch}`;
const binName = process.platform === "win32" ? "awaken-core.exe" : "awaken-core";
const binaryPath = join(root, "apps", "desktop", "resources", "bin", target, binName);

function hasExecutableBinary() {
  try {
    accessSync(binaryPath, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

if (hasExecutableBinary()) {
  console.log(`[ensure-desktop-runtime] found ${binaryPath}`);
  process.exit(0);
}

console.log(`[ensure-desktop-runtime] missing ${binaryPath}; building awaken-core for ${target}`);

try {
  execFileSync("node", ["scripts/build-binary.mjs"], {
    cwd: root,
    env: {
      ...process.env,
      AWAKEN_BINARY_TARGETS: target,
    },
    stdio: "inherit",
  });
} catch {
  console.error(
    `[ensure-desktop-runtime] failed to build awaken-core. Run npm run build:binary for full output.`,
  );
  process.exit(1);
}

if (!hasExecutableBinary()) {
  console.error(`[ensure-desktop-runtime] build finished but ${binaryPath} is still missing.`);
  process.exit(1);
}
