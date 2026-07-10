import {
  PET_NOTIFICATION_TTL_MS,
  type PetActivitySnapshot,
  type PetNotificationActionRequest,
  type PetSessionActivity,
  type PetSessionStatus,
} from "../../shared/pets";

export type PetNotification = {
  id: string;
  key: string;
  status: PetSessionStatus;
  title: string;
  subtitle?: string;
  turnKey: string | null;
  runId?: string;
  threadId?: string;
  updatedAtMs: number;
  expiredAtMs: number;
  level: "loading" | "warning" | "danger" | "success";
  actions: PetNotificationAction[];
};

export type PetNotificationAction = {
  id: string;
  label: string;
  request: PetNotificationActionRequest;
};

function levelForStatus(status: PetSessionStatus): PetNotification["level"] {
  switch (status) {
    case "running":
      return "loading";
    case "waiting":
      return "warning";
    case "failed":
      return "danger";
    case "review":
      return "success";
    default:
      return "loading";
  }
}

function defaultLabelForStatus(status: PetSessionStatus): string {
  switch (status) {
    case "running":
      return "Running";
    case "waiting":
      return "Needs input";
    case "failed":
      return "Blocked";
    case "review":
      return "Ready";
    default:
      return "Thinking";
  }
}

function ttlForStatus(status: PetSessionStatus): number {
  return PET_NOTIFICATION_TTL_MS[status as Exclude<PetSessionStatus, "idle">] ?? 0;
}

function actionsForSession(session: PetSessionActivity): PetNotificationAction[] {
  const actions: PetNotificationAction[] = [];

  if (session.status === "running" && session.runId) {
    actions.push({
      id: "stop",
      label: "Stop",
      request: { type: "stop-run", runId: session.runId },
    });
  }

  return actions;
}

export type DismissedNotificationKey = string;

export function notificationKeyForSession(session: PetSessionActivity): string {
  return `${session.key}:${session.turnKey ?? session.runId ?? "current"}`;
}

export function buildNotifications(
  snapshot: PetActivitySnapshot,
  dismissedKeys: Set<DismissedNotificationKey>,
  nowMs: number = Date.now(),
): PetNotification[] {
  const notifications: PetNotification[] = [];

  for (const session of snapshot.sessions) {
    if (session.status === "idle") continue;
    const key = notificationKeyForSession(session);
    if (dismissedKeys.has(key)) continue;

    const ttl = ttlForStatus(session.status);
    const expiredAtMs = session.updatedAtMs + ttl;
    if (nowMs > expiredAtMs) continue;

    notifications.push({
      id: session.key,
      key,
      status: session.status,
      title: session.title || defaultLabelForStatus(session.status),
      subtitle: session.subtitle,
      turnKey: session.turnKey ?? null,
      runId: session.runId,
      threadId: session.threadId,
      updatedAtMs: session.updatedAtMs,
      expiredAtMs,
      level: levelForStatus(session.status),
      actions: actionsForSession(session),
    });
  }

  const priority: PetSessionStatus[] = ["waiting", "failed", "review", "running"];
  notifications.sort((a, b) => {
    const pa = priority.indexOf(a.status);
    const pb = priority.indexOf(b.status);
    if (pa !== pb) return pa - pb;
    return b.updatedAtMs - a.updatedAtMs;
  });

  return notifications;
}
