#!/usr/bin/env node
import { checkPrimaryRuntime } from "./primary-runtime-common.mjs";

const args = new Set(process.argv.slice(2));
const json = args.has("--json");
const quiet = args.has("--quiet");

const report = checkPrimaryRuntime();

if (json) {
  console.log(JSON.stringify(report, null, 2));
} else if (!quiet) {
  console.log(`primary-runtime ${report.ready ? "ready" : "missing"}`);
  console.log(`- manifest: ${report.manifestPath}`);
  console.log(`- runtime: ${report.runtimeDir}`);
  for (const component of report.components) {
    console.log(
      `- ${component.name}: ${component.ok ? "ok" : "missing"} ${component.path}${
        component.detail ? ` (${component.detail})` : ""
      }`,
    );
  }
}

if (!report.ready) {
  process.exit(1);
}
