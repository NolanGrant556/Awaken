#!/usr/bin/env node
import {
  checkPrimaryRuntime,
  migrateOldAwakenCacheIfNeeded,
  resolvePrimaryRuntimeRoot,
} from "./primary-runtime-common.mjs";

const args = new Set(process.argv.slice(2));
const json = args.has("--json");
const quiet = args.has("--quiet");
const migrate = args.has("--migrate");

const root = resolvePrimaryRuntimeRoot();
let migration = null;
if (migrate) {
  migration = migrateOldAwakenCacheIfNeeded({ root });
}

const report = checkPrimaryRuntime({ root });
if (migration) {
  report.migration = migration;
}

if (json) {
  console.log(JSON.stringify(report, null, 2));
} else if (!quiet) {
  console.log(`primary-runtime ${report.ready ? "ready" : "missing"}`);
  console.log(`- manifest: ${report.manifestPath}`);
  console.log(`- runtimeRoot: ${report.runtimeRoot}`);
  console.log(`- active: ${report.active || "(none)"}`);
  console.log(`- previous: ${report.previous || "(none)"}`);
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
