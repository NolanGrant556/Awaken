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
  type PetNotificationAction,
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
  const [dismissedKeys, setDismissedKeys] = useState<Set<string>>(new Set());
  const [locallyHiddenKeys, setLocallyHiddenKeys] = useState<Set<string>>(new Set());
  const [firstAwakeUntil, setFirstAwakeUntil] = useState<number | null>(null);
  const [notificationClockMs, setNotificationClockMs] = useState(() => Date.now());
  const dragStartRef = useRef<{ x: number; y: number; moved: boolean } | null>(null);
  const resizeStartRef = useRef<{ x: number; initialWidth: number } | null>(null);
  const hitAreaRef = useRef<HTMLDivElement | null>(null);
  const activeNotificationKeyRef = useRef<string | null>(null);

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
      const key = activeNotificationKeyRef.current;
      if (key) {
        setLocallyHiddenKeys((prev) => new Set([...prev, key]));
      }
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
    const isInteractive = hover || dragging || resizeMode || bubbleExpanded;
    bridge.setPointerInteractive(isInteractive);
  }, [hover, dragging, resizeMode, bubbleExpanded]);

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
        console.debug("[awaken-pet-debug] bounds:report", {
          borderBox: { width, height },
          contentRect: {
            width: entry.contentRect.width,
            height: entry.contentRect.height,
          },
          offset: {
            width: element.offsetWidth,
            height: element.offsetHeight,
          },
        });
        bridge.reportBounds({ width: Math.round(width), height: Math.round(height) });
      }
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [bootstrap?.petId]);

  const visibleDismissedKeys = new Set([...dismissedKeys, ...locallyHiddenKeys]);
  const notifications = buildNotifications(snapshot, visibleDismissedKeys, notificationClockMs);
  const activeNotification = notifications[0] ?? null;
  const businessState: PetAnimationState = activeNotification?.status ?? "idle";

  useEffect(() => {
    activeNotificationKeyRef.current = activeNotification?.key ?? null;
    if (!activeNotification) {
      setBubbleExpanded(false);
    }
  }, [activeNotification?.key, activeNotification]);

  useEffect(() => {
    const nextExpiry = notifications.reduce<number | null>(
      (earliest, notification) =>
        earliest === null || notification.expiredAtMs < earliest
          ? notification.expiredAtMs
          : earliest,
      null,
    );
    if (nextExpiry === null) return undefined;
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

  async function handleNotificationActivate(notification: PetNotification) {
    setBubbleExpanded(false);
    setLocallyHiddenKeys((prev) => new Set([...prev, notification.key]));
    await window.awakenPetOverlay.activateNotification(notification.id);
  }

  async function handleNotificationAction(action: PetNotificationAction) {
    await window.awakenPetOverlay.performNotificationAction(action.request);
  }

  async function handleDismissNotification(notification: PetNotification, event: React.MouseEvent) {
    event.stopPropagation();
    setDismissedKeys((prev) => new Set([...prev, notification.key]));
    setLocallyHiddenKeys((prev) => new Set([...prev, notification.key]));
    await window.awakenPetOverlay.dismissNotification({
      notificationId: notification.id,
      turnKey: notification.turnKey,
    });
  }

  const notificationBody =
    activeNotification?.subtitle ??
    (activeNotification ? (FALLBACK_NOTIFICATION_BODY[activeNotification.status] ?? "") : "");

  if (!bootstrap || !bootstrap.petId || !bootstrap.spritesheetUrl) {
    return null;
  }

  return (
    <div className="pet-overlay-root">
      <div
        ref={hitAreaRef}
        className="pet-hit-area"
        onPointerEnter={() => setHover(true)}
        onPointerLeave={() => setHover(false)}
      >
        {activeNotification ? (
          <div
            className={`pet-work-bubble${
              bubbleExpanded ? " pet-work-bubble-expanded" : ""
            } pet-work-bubble-level-${activeNotification.level}`}
            role="status"
            aria-live="polite"
          >
            <button
              type="button"
              className="pet-bubble-icon pet-bubble-close"
              aria-label="Close pet message"
              onClick={(event) => void handleDismissNotification(activeNotification, event)}
            >
              x
            </button>
            <button
              type="button"
              className="pet-bubble-icon pet-bubble-expand"
              aria-label={bubbleExpanded ? "Collapse pet message" : "Expand pet message"}
              aria-expanded={bubbleExpanded}
              onClick={(event) => {
                event.stopPropagation();
                setBubbleExpanded((value) => !value);
              }}
            >
              {bubbleExpanded ? "\u2303" : "\u2304"}
            </button>
            <div
              className="pet-work-bubble-content"
              onClick={() => void handleNotificationActivate(activeNotification)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  void handleNotificationActivate(activeNotification);
                }
              }}
              role="button"
              tabIndex={0}
            >
              <div className="pet-work-bubble-title">{activeNotification.title}</div>
              <div className="pet-work-bubble-body">{notificationBody}</div>
              {bubbleExpanded && activeNotification.actions.length > 0 ? (
                <div className="pet-work-bubble-actions">
                  {activeNotification.actions.map((action) => (
                    <button
                      key={action.id}
                      type="button"
                      className="pet-work-bubble-action"
                      onClick={(event) => {
                        event.stopPropagation();
                        void handleNotificationAction(action);
                      }}
                    >
                      {action.label}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
            <button
              type="button"
              className="pet-bubble-reply"
              onClick={(event) => {
                event.stopPropagation();
                void handleNotificationActivate(activeNotification);
              }}
            >
              {"\u56de\u590d"}
            </button>
          </div>
        ) : null}

        <div className="pet-mascot-wrapper">
          <div
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            onPointerCancel={handlePointerCancel}
          >
            <PetSprite
              spritesheetUrl={bootstrap.spritesheetUrl}
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
