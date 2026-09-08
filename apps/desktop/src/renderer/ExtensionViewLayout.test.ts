import { readStyleSource } from "./style-test-source";
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const extensionStyles = readStyleSource(new URL("./styles.css", import.meta.url));
const extensionsSource = readFileSync(
  new URL("./ExtensionsView.tsx", import.meta.url),
  "utf8",
);
const scheduledTasksSource = readFileSync(
  new URL("./ScheduledTasksView.tsx", import.meta.url),
  "utf8",
);

describe("Extension page layout", () => {
  it("keeps the shared 72px tail inside the Skills and scheduled-task scroll range", () => {
    expect(extensionStyles).toMatch(
      /\.extension-view\s*\{[\s\S]*?overflow-y:\s*auto;/,
    );
    expect(extensionStyles).toMatch(
      /\.extension-view-inner\s*\{[\s\S]*?align-self:\s*start;[\s\S]*?padding:\s*54px\s+0\s+72px;[\s\S]*?overflow:\s*visible;/,
    );
    expect(extensionsSource).toContain(
      '<section className="extension-view" aria-label={title}>',
    );
    expect(scheduledTasksSource).toContain(
      '<section className="extension-view scheduled-tasks-view" aria-label="定时任务">',
    );
  });
});
