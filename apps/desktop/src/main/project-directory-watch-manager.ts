import { randomUUID } from "node:crypto";
import { watch } from "node:fs";
import path from "node:path";
import type {
  ProjectDirectoryChangedEvent,
  ProjectDirectoryWatchRequest,
  ProjectDirectoryWatchResult,
  RuntimeProject,
} from "../shared/runtime.js";
import { listProjectDirectory } from "./project-files.js";

const DEFAULT_CHANGE_DEBOUNCE_MS = 80;

type ProjectDirectoryWatchHandle = {
  close: () => void;
};

export type ProjectDirectoryWatchFactory = (
  rootPath: string,
  onChange: () => void,
  onError: () => void,
) => ProjectDirectoryWatchHandle;

type ProjectDirectoryWatchNotification = {
  webContentsId: number;
  event: ProjectDirectoryChangedEvent;
};

type ProjectDirectoryWatchSubscription = {
  webContentsId: number;
  watcher: ProjectDirectoryWatchHandle;
  changeTimer?: ReturnType<typeof setTimeout>;
};

function watchDirectory(
  rootPath: string,
  onChange: () => void,
  onError: () => void,
): ProjectDirectoryWatchHandle {
  const watcher = watch(rootPath, { recursive: true }, onChange);
  watcher.on("error", onError);
  return watcher;
}

/**
 * Owns project-folder filesystem subscriptions in Electron Main. Subscription
 * setup reuses the directory-listing authority check, while every Renderer
 * refresh still goes through listProjectDirectory and its current-project
 * revalidation.
 */
export class ProjectDirectoryWatchManager {
  private readonly subscriptions = new Map<string, ProjectDirectoryWatchSubscription>();

  constructor(
    private readonly notify: (notification: ProjectDirectoryWatchNotification) => void,
    private readonly createWatcher: ProjectDirectoryWatchFactory = watchDirectory,
    private readonly changeDebounceMs = DEFAULT_CHANGE_DEBOUNCE_MS,
    private readonly createSubscriptionId: () => string = randomUUID,
  ) {}

  async subscribe(
    projects: RuntimeProject[],
    request: ProjectDirectoryWatchRequest,
    webContentsId: number,
  ): Promise<ProjectDirectoryWatchResult> {
    const validation = await listProjectDirectory(projects, request);
    if (!validation.ok) return validation;

    const subscriptionId = this.createSubscriptionId();
    try {
      const watcher = this.createWatcher(
        path.resolve(request.folderPath),
        () => this.scheduleChange(subscriptionId),
        () => this.stopAfterError(subscriptionId),
      );
      this.subscriptions.set(subscriptionId, { webContentsId, watcher });
      return { ok: true, subscriptionId };
    } catch {
      return {
        ok: false,
        code: "WATCH_UNAVAILABLE",
        message: "文件变化暂时无法自动同步。",
      };
    }
  }

  unsubscribe(subscriptionId: string, webContentsId: number): void {
    const subscription = this.subscriptions.get(subscriptionId);
    if (!subscription || subscription.webContentsId !== webContentsId) return;
    this.closeSubscription(subscriptionId, subscription);
  }

  releaseForWebContents(webContentsId: number): void {
    for (const [subscriptionId, subscription] of this.subscriptions) {
      if (subscription.webContentsId === webContentsId) {
        this.closeSubscription(subscriptionId, subscription);
      }
    }
  }

  dispose(): void {
    for (const [subscriptionId, subscription] of this.subscriptions) {
      this.closeSubscription(subscriptionId, subscription);
    }
  }

  private scheduleChange(subscriptionId: string): void {
    const subscription = this.subscriptions.get(subscriptionId);
    if (!subscription) return;
    if (subscription.changeTimer) clearTimeout(subscription.changeTimer);
    subscription.changeTimer = setTimeout(() => {
      const current = this.subscriptions.get(subscriptionId);
      if (!current) return;
      current.changeTimer = undefined;
      this.notify({
        webContentsId: current.webContentsId,
        event: { subscriptionId },
      });
    }, this.changeDebounceMs);
  }

  private stopAfterError(subscriptionId: string): void {
    const subscription = this.subscriptions.get(subscriptionId);
    if (!subscription) return;
    this.notify({
      webContentsId: subscription.webContentsId,
      event: { subscriptionId },
    });
    this.closeSubscription(subscriptionId, subscription);
  }

  private closeSubscription(
    subscriptionId: string,
    subscription: ProjectDirectoryWatchSubscription,
  ): void {
    if (subscription.changeTimer) clearTimeout(subscription.changeTimer);
    subscription.watcher.close();
    this.subscriptions.delete(subscriptionId);
  }
}
