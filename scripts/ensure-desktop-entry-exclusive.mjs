#!/usr/bin/env node

import { closeCompetingPackagedAwaken } from "./desktop-lifecycle-common.mjs";

try {
  const result = await closeCompetingPackagedAwaken();
  if (result.closed.length > 0) {
    console.log(
      `[desktop-entry] 已关闭其他 Awaken 安装版实例：${result.closed
        .map((row) => row.pid)
        .join(", ")}`,
    );
  }
} catch (error) {
  console.error(
    `[desktop-entry] 无法建立 README 唯一实例：${error instanceof Error ? error.message : String(error)}`,
  );
  process.exit(1);
}
