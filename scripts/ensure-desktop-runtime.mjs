import { execFileSync } from "node:child_process";
import { accessSync, constants, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");
const target = `${process.platform}-${process.arch}`;
const binName = process.platform === "win32" ? "awaken-core.exe" : "awaken-core";
const binaryPath = join(root, "apps", "desktop", "resources", "bin", target, binName);
const companionBinaryPaths = process.platform === "win32"
  ? [join(root, "apps", "desktop", "resources", "bin", target, "awaken-network-watch.exe")]
  : [];
const sourceRoots = [join(root, "go.mod"), join(root, "cmd"), join(root, "src")];

function hasExecutableBinary(filePath = binaryPath) {
  try {
    accessSync(filePath, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

function isRuntimeSource(filePath) {
  return filePath === join(root, "go.mod") || (filePath.endsWith(".go") && !filePath.endsWith("_test.go"));
}

function newestRuntimeSource() {
  let newest = { path: "", mtimeMs: 0 };

  function visit(filePath) {
    let stat;
    try {
      stat = statSync(filePath);
    } catch {
      return;
    }

    if (stat.isDirectory()) {
      for (const entry of readdirSync(filePath, { withFileTypes: true })) {
        visit(join(filePath, entry.name));
      }
      return;
    }

    if (stat.isFile() && isRuntimeSource(filePath) && stat.mtimeMs > newest.mtimeMs) {
      newest = { path: filePath, mtimeMs: stat.mtimeMs };
    }
  }

  for (const sourceRoot of sourceRoots) {
    visit(sourceRoot);
  }

  return newest;
}

function buildReason() {
  if (!hasExecutableBinary()) {
    return `missing ${binaryPath}`;
  }
  for (const companionPath of companionBinaryPaths) {
    if (!hasExecutableBinary(companionPath)) {
      return `missing ${companionPath}`;
    }
  }

  const binaryStat = statSync(binaryPath);
  const newestSource = newestRuntimeSource();
  if (newestSource.path && newestSource.mtimeMs > binaryStat.mtimeMs + 1) {
    return `${relative(root, newestSource.path)} is newer than ${relative(root, binaryPath)}`;
  }

  return "";
}

const reason = buildReason();

if (!reason) {
  console.log(`[ensure-desktop-runtime] found current ${binaryPath}`);
} else {
  console.log(`[ensure-desktop-runtime] ${reason}; building runtime binaries for ${target}`);

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
      `[ensure-desktop-runtime] failed to build runtime binaries. Run npm run build:binary for full output.`,
    );
    process.exit(1);
  }

  if (!hasExecutableBinary()) {
    console.error(`[ensure-desktop-runtime] build finished but ${binaryPath} is still missing.`);
    process.exit(1);
  }
  for (const companionPath of companionBinaryPaths) {
    if (!hasExecutableBinary(companionPath)) {
      console.error(`[ensure-desktop-runtime] build finished but ${companionPath} is still missing.`);
      process.exit(1);
    }
  }
}

try {
  execFileSync("node", ["scripts/prepare-officecli.mjs", `--targets=${target}`], {
    cwd: root,
    env: {
      ...process.env,
      AWAKEN_BINARY_TARGETS: target,
    },
    stdio: "inherit",
  });
} catch {
  console.error(
    `[ensure-desktop-runtime] failed to prepare OfficeCLI. Run npm run prepare:officecli for full output.`,
  );
  process.exit(1);
}

try {
  execFileSync("node", ["scripts/prepare-node-runtime.mjs"], {
    cwd: root,
    env: { ...process.env },
    stdio: "inherit",
  });
} catch {
  console.error(
    `[ensure-desktop-runtime] failed to prepare Node runtime. Run node scripts/prepare-node-runtime.mjs for full output.`,
  );
  process.exit(1);
}

try {
  execFileSync("node", ["scripts/prepare-cua-driver.mjs"], {
    cwd: root,
    env: { ...process.env },
    stdio: "inherit",
  });
} catch {
  console.error(
    `[ensure-desktop-runtime] failed to prepare CUA driver. Run node scripts/prepare-cua-driver.mjs for full output.`,
  );
  process.exit(1);
}
