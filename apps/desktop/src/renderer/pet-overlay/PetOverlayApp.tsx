import { useCallback, useEffect, useRef, useState } from "react";
import {
  PET_FIRST_AWAKE_TTL_MS,
  PET_MASCOT_DEFAULT_WIDTH_PX,
  PET_MASCOT_MAX_WIDTH_PX,
  PET_MASCOT_MIN_WIDTH_PX,
  type PetActivitySnapshot,
  type PetAnimationState,
  type PetNotificationActionRequest,
  type PetOverlayBootstrap,
  type PetOverlayBounds,
} from "../../shared/pets";
import {
  buildNotifications,
  notificationKeyForSession,
  type PetNotification,
} from "../pets/pet-notifications";
import {
  resolveOverlayAnimationState,
  type PetDragDirection,
} from "./pet-overlay-state";
import { PetSprite } from "./PetSprite";

declare global {
  interface Window {
    awakenPetOverlay: {
      getBootstrap: () => Promise<PetOverlayBootstrap>;
      onBootstrapChanged: (callback: (value: PetOverlayBootstrap) => void) => () => void;
      onActivityChanged: (callback: (value: PetActivitySnapshot) => void) => () => void;
      onMainWindowFocused: (callback: () => void) => () => void;
      beginDrag: (point: { x: number; y: number }) => void;
      moveDrag: (point: { x: number; y: number }) => void;
      endDrag: (point: { x: number; y: number }) => void;
      setPointerInteractive: (interactive: boolean) => void;
      resizeMascot: (width: number) => void;
      finishResize: (width: number) => void;
      reportBounds: (bounds: PetOverlayBounds) => void;
      activateNotification: (notificationId: string) => Promise<void>;
      performNotificationAction: (input: PetNotificationActionRequest) => Promise<void>;
      dismissNotification: (input: {
        notificationId: string;
        turnKey: string | null;
      }) => Promise<void>;
      recordPetWelcomed: (petId: string) => Promise<void>;
      openMainWindow: () => void;
      closeOverlay: () => void;
    };
  }
}

const DRAG_THRESHOLD_PX = 4;

const FALLBACK_NOTIFICATION_BODY: Partial<Record<PetAnimationState, string>> = {
  running: "\u6b63\u5728\u601d\u8003\u5e76\u751f\u6210\u56de\u7b54",
  waiting: "\u9700\u8981\u4f60\u56de\u590d\u540e\u7ee7\u7eed",
  failed: "\u8fd9\u6b21\u8fd0\u884c\u9047\u5230\u963b\u585e",
  review: "\u5df2\u6709\u65b0\u7684\u7ed3\u679c\u53ef\u67e5\u770b",
};

export function PetOverlayApp() {
  const [bootstrap, setBootstrap] = useState<PetOverlayBootstrap | null>(null);
  const [snapshot, setSnapshot] = useState<PetActivitySnapshot>({
    sessions: [],
    emittedAtMs: 0,
  });
  const [hover, setHover] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [resizeMode, setResizeMode] = useState(false);
  const [mascotWidth, setMascotWidth] = useState(PET_MASCOT_DEFAULT_WIDTH_PX);
  const [dragDirection, setDragDirection] = useState<PetDragDirection>("right");
  const [bubbleExpanded, setBubbleExpanded] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [queued, setQueued] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [sending, setSending] = useState<Set<string>>(new Set());
  const inFlight = useRef(new Set<string>());
  const [dismissedKeys, setDismissedKeys] = useState<Set<string>>(new Set());
  const [locallyHiddenKeys, setLocallyHiddenKeys] = useState<Set<string>>(new Set());
  const [firstAwakeUntil, setFirstAwakeUntil] = useState<number | null>(null);
  const [notificationClockMs, setNotificationClockMs] = useState(() => Date.now());
  const dragStartRef = useRef<{ x: number; y: number; moved: boolean } | null>(null);
  const resizeStartRef = useRef<{ x: number; initialWidth: number } | null>(null);
  const hitAreaRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    const bridge = window.awakenPetOverlay;
    const applyBootstrap = (value: PetOverlayBootstrap): void => {
      if (cancelled) return;
      setBootstrap(value);
      if (value.mascotWidthPx) {
        setMascotWidth(value.mascotWidthPx);
      }
      if (value.dismissedNotificationKeys) {
        setDismissedKeys(new Set(value.dismissedNotificationKeys));
      }
      const petId = value.petId;
      const alreadyWelcomed = petId ? value.welcomedPetIds?.includes(petId) : true;
      setFirstAwakeUntil(null);
      if (petId && !alreadyWelcomed) {
        setFirstAwakeUntil(Date.now() + PET_FIRST_AWAKE_TTL_MS);
        void bridge.recordPetWelcomed(petId);
      }
    };

    void bridge.getBootstrap().then(applyBootstrap);
    const unsubscribeBootstrap = bridge.onBootstrapChanged(applyBootstrap);
    const unsubscribeActivity = bridge.onActivityChanged((nextSnapshot) => {
      setSnapshot(nextSnapshot);
      setNotificationClockMs(Date.now());
      setLocallyHiddenKeys((prev) => {
        const liveKeys = new Set(nextSnapshot.sessions.map(notificationKeyForSession));
        const next = new Set([...prev].filter((key) => liveKeys.has(key)));
        return next.size === prev.size ? prev : next;
      });
    });
    const unsubscribeMainFocus = bridge.onMainWindowFocused(() => {
      setBubbleExpanded(false);
      setSelected(null);
      setEditing(null);
    });

    return () => {
      cancelled = true;
      unsubscribeBootstrap();
      unsubscribeActivity();
      unsubscribeMainFocus();
    };
  }, []);

  useEffect(() => {
    if (firstAwakeUntil === null) return undefined;
    const delay = Math.max(0, firstAwakeUntil - Date.now());
    const timer = window.setTimeout(() => {
      setFirstAwakeUntil((current) => (current === firstAwakeUntil ? null : current));
    }, delay);
    return () => window.clearTimeout(timer);
  }, [firstAwakeUntil]);

  useEffect(() => {
    const bridge = window.awakenPetOverlay;
    const isInteractive = hover || dragging || resizeMode || bubbleExpanded || editing !== null;
    bridge.setPointerInteractive(isInteractive);
  }, [hover, dragging, resizeMode, bubbleExpanded, editing]);

  useEffect(() => {
    const element = hitAreaRef.current;
    if (!element) return undefined;

    const bridge = window.awakenPetOverlay;
    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      const rect = element.getBoundingClientRect();
      const width = rect.width;
      const height = rect.height;
      if (width > 0 && height > 0) {
        bridge.reportBounds({ width: Math.round(width), height: Math.round(height) });
      }
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [bootstrap?.petId]);

  const visibleDismissedKeys = new Set([...dismissedKeys, ...locallyHiddenKeys]);
  const retainedThreadIds = new Set([
    ...(editing ? [editing] : []),
    ...Object.keys(queued),
  ]);
  const notifications = buildNotifications(snapshot, visibleDismissedKeys, notificationClockMs, retainedThreadIds);
  const activeNotification = notifications[0] ?? null;
  const businessState: PetAnimationState = activeNotification?.status ?? "idle";

  useEffect(() => {
    if (!activeNotification) {
      setBubbleExpanded(false);
    }
  }, [activeNotification]);

  useEffect(() => {
    const nextExpiry = notifications.reduce<number | null>(
      (earliest, notification) =>
        earliest === null || notification.expiredAtMs < earliest
          ? notification.expiredAtMs
          : earliest,
      null,
    );
    if (nextExpiry === null || !Number.isFinite(nextExpiry)) return undefined;
    const timer = window.setTimeout(
      () => setNotificationClockMs(Date.now()),
      Math.max(0, nextExpiry - Date.now() + 1),
    );
    return () => window.clearTimeout(timer);
  }, [notifications]);

  const effectiveBusinessState: PetAnimationState =
    firstAwakeUntil && Date.now() < firstAwakeUntil && businessState === "idle"
      ? "waving"
      : businessState;

  const animationState = resolveOverlayAnimationState({
    resizeMode,
    dragging,
    dragDirection,
    hover,
    effectiveBusinessState,
  });

  const handlePointerDown = useCallback(
    (event: React.PointerEvent) => {
      if (resizeMode || event.button !== 0 || event.ctrlKey) return;
      dragStartRef.current = { x: event.screenX, y: event.screenY, moved: false };
      window.awakenPetOverlay.beginDrag({ x: event.screenX, y: event.screenY });
      (event.target as HTMLElement).setPointerCapture(event.pointerId);
    },
    [resizeMode],
  );

  const handlePointerMove = useCallback(
    (event: React.PointerEvent) => {
      if (!dragStartRef.current || resizeMode) return;
      const dx = event.screenX - dragStartRef.current.x;
      const dy = event.screenY - dragStartRef.current.y;
      if (dx > 0) {
        setDragDirection("right");
      } else if (dx < 0) {
        setDragDirection("left");
      }
      if (Math.abs(dx) > DRAG_THRESHOLD_PX || Math.abs(dy) > DRAG_THRESHOLD_PX) {
        dragStartRef.current.moved = true;
        setDragging(true);
      }
      if (dragStartRef.current.moved) {
        window.awakenPetOverlay.moveDrag({ x: event.screenX, y: event.screenY });
      }
    },
    [resizeMode],
  );

  const handlePointerUp = useCallback(
    (event: React.PointerEvent) => {
      if (resizeMode) return;
      const didMove = dragStartRef.current?.moved ?? false;
      dragStartRef.current = null;
      setDragging(false);
      window.awakenPetOverlay.endDrag({ x: event.screenX, y: event.screenY });
      if (!didMove) {
        window.awakenPetOverlay.openMainWindow();
      }
    },
    [resizeMode],
  );

  const handlePointerCancel = useCallback((event: React.PointerEvent) => {
    dragStartRef.current = null;
    setDragging(false);
    window.awakenPetOverlay.endDrag({ x: event.screenX, y: event.screenY });
  }, []);

  const handleResizePointerDown = useCallback(
    (event: React.PointerEvent) => {
      event.stopPropagation();
      resizeStartRef.current = { x: event.screenX, initialWidth: mascotWidth };
      setResizeMode(true);
      (event.target as HTMLElement).setPointerCapture(event.pointerId);
    },
    [mascotWidth],
  );

  const handleResizePointerMove = useCallback((event: React.PointerEvent) => {
    if (!resizeStartRef.current) return;
    const dx = event.screenX - resizeStartRef.current.x;
    const nextWidth = Math.min(
      PET_MASCOT_MAX_WIDTH_PX,
      Math.max(PET_MASCOT_MIN_WIDTH_PX, resizeStartRef.current.initialWidth + dx),
    );
    setMascotWidth(nextWidth);
    window.awakenPetOverlay.resizeMascot(nextWidth);
  }, []);

  const handleResizePointerUp = useCallback((event: React.PointerEvent) => {
    if (!resizeStartRef.current) return;
    const dx = event.screenX - resizeStartRef.current.x;
    const nextWidth = Math.min(
      PET_MASCOT_MAX_WIDTH_PX,
      Math.max(PET_MASCOT_MIN_WIDTH_PX, resizeStartRef.current.initialWidth + dx),
    );
    resizeStartRef.current = null;
    setResizeMode(false);
    setMascotWidth(nextWidth);
    window.awakenPetOverlay.finishResize(nextWidth);
  }, []);

  async function handleDismissNotification(notification: PetNotification, event: React.MouseEvent) {
    event.stopPropagation();
    setDismissedKeys((prev) => new Set([...prev, notification.key]));
    setLocallyHiddenKeys((prev) => new Set([...prev, notification.key]));
    await window.awakenPetOverlay.dismissNotification({
      notificationId: notification.id,
      turnKey: notification.turnKey,
    });
  }

  useEffect(() => {
    for (const [threadId, prompt] of Object.entries(queued)) {
      const session = snapshot.sessions.find((item) => item.threadId === threadId);
      if (!session || session.status === "running" || session.status === "waiting" ||
          inFlight.current.has(threadId)) continue;
      inFlight.current.add(threadId);
      setSending((prev) => new Set([...prev, threadId]));
      void window.awakenPetOverlay.performNotificationAction({ type: "follow-up", threadId, prompt })
        .then(() => setDrafts((prev) => ({ ...prev, [threadId]: "" })))
        .catch(() => setErrors((prev) => ({ ...prev, [threadId]: "发送失败，请按回车重试" })))
        .finally(() => {
          setQueued((prev) => {
            const next = { ...prev };
            delete next[threadId];
            return next;
          });
          setSending((prev) => new Set([...prev].filter((id) => id !== threadId)));
          inFlight.current.delete(threadId);
        });
    }
  }, [queued, snapshot]);

  if (!bootstrap || !bootstrap.petId || !bootstrap.spritesheetUrl) {
    return null;
  }

  return (
    <div className="pet-overlay-root">
      <div
        ref={hitAreaRef}
        className={`pet-hit-area${activeNotification ? " pet-hit-area-with-bubble" : ""}`}
        onPointerEnter={() => setHover(true)}
        onPointerLeave={() => setHover(false)}
      >
        {activeNotification && (
          <div
            className={`pet-bubble-list${!bubbleExpanded && notifications.length > 1 ? " pet-bubble-stack" : ""}`}
            style={{ maxHeight: Math.min(248, 363 - Math.ceil(mascotWidth * 208 / 192)) }}
          >
            {(bubbleExpanded ? notifications : [activeNotification]).map((item) => {
              const id = item.threadId ?? item.id;
              const showActions = selected === id;
              const stop = item.actions[0];
              return (
                <div key={item.id} className="pet-work-bubble">
                  <div className="pet-bubble-row">
                    <button
                      type="button"
                      className="pet-work-bubble-content"
                      aria-expanded={bubbleExpanded || showActions}
                      onClick={() => {
                        if (!bubbleExpanded && notifications.length > 1) {
                          setBubbleExpanded(true);
                          return;
                        }
                        setSelected(showActions ? null : id);
                        if (editing !== id || showActions) setEditing(null);
                      }}
                    >
                      <span className="pet-work-bubble-title">
                        {item.status === "review" && <svg className="pet-bubble-done" viewBox="0 0 16 16" role="img" aria-label="已完成">
                          <circle cx="8" cy="8" r="8" fill="#079b46" />
                          <path d="m4.75 8 2.15 2.15 4.35-4.3" fill="none" stroke="white"
                            strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>}
                        {item.title}
                      </span>
                      <span className="pet-work-bubble-body">
                        {queued[id] ? (sending.has(id) ? "正在发送…" : "跟进消息将在本轮结束后发送") :
                          item.subtitle ?? FALLBACK_NOTIFICATION_BODY[item.status]}
                      </span>
                    </button>
                    {showActions && (
                      <>
                        {item.threadId && (
                          <button type="button" className="pet-round-action" aria-label="输入跟进消息"
                            aria-pressed={editing === id}
                            onClick={() => setEditing(editing === id ? null : id)}>
                            <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor"
                              strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                              <path d="M20 5v5a4 4 0 0 1-4 4H4m5-5-5 5 5 5" />
                            </svg>
                          </button>
                        )}
                        {stop && (
                          <button type="button" className="pet-round-action" aria-label="终止当前运行"
                            onClick={() => {
                              setErrors((prev) => ({ ...prev, [id]: "" }));
                              void window.awakenPetOverlay.performNotificationAction(stop.request)
                                .catch(() => setErrors((prev) => ({ ...prev, [id]: "终止失败，请重试" })));
                            }}>
                            <svg viewBox="0 0 24 24" aria-hidden="true">
                              <rect x="5" y="5" width="14" height="14" rx="2.5" fill="currentColor" />
                            </svg>
                          </button>
                        )}
                        <button type="button" className="pet-bubble-dismiss" aria-label="关闭气泡"
                          onClick={(event) => void handleDismissNotification(item, event)}>×</button>
                      </>
                    )}
                  </div>
                  {editing === id && (
                    <input
                      autoFocus
                      aria-label="继续跟进"
                      placeholder="继续跟进"
                      maxLength={16000}
                      className="pet-bubble-input"
                      value={drafts[id] ?? ""}
                      readOnly={Boolean(queued[id])}
                      onChange={(event) => setDrafts({ ...drafts, [id]: event.target.value })}
                      onKeyDown={(event) => {
                        if (event.key !== "Enter" || event.nativeEvent.isComposing || event.keyCode === 229) return;
                        event.preventDefault();
                        const prompt = drafts[id]?.trim();
                        if (!item.threadId || !prompt || queued[id]) return;
                        setErrors((prev) => ({ ...prev, [id]: "" }));
                        setQueued((prev) => ({ ...prev, [id]: prompt }));
                      }}
                    />
                  )}
                  {showActions && item.status === "waiting" && (
                    <button type="button" className="pet-bubble-collapse" onClick={() => {
                      setLocallyHiddenKeys((prev) => new Set([...prev, item.key]));
                      void window.awakenPetOverlay.activateNotification(item.id);
                    }}>在会话中回答</button>
                  )}
                  {errors[id] && <div className="pet-bubble-error" role="alert">{errors[id]}</div>}
                </div>
              );
            })}
            {bubbleExpanded && notifications.length > 1 && (
              <button type="button" className="pet-bubble-collapse" onClick={() => {
                setBubbleExpanded(false);
                setSelected(null);
                setEditing(null);
              }}>收起</button>
            )}
          </div>
        )}

        <div className="pet-mascot-wrapper">
          <div
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            onPointerCancel={handlePointerCancel}
          >
            <PetSprite
              spritesheetUrl={bootstrap.spritesheetUrl}
              spriteVersionNumber={bootstrap.spriteVersionNumber}
              state={animationState}
              widthPx={mascotWidth}
            />
          </div>
          <button
            type="button"
            className="pet-resize-handle"
            aria-label="Resize pet"
            onPointerDown={handleResizePointerDown}
            onPointerMove={handleResizePointerMove}
            onPointerUp={handleResizePointerUp}
            onPointerCancel={handleResizePointerUp}
          />
        </div>
      </div>
    </div>
  );
}
