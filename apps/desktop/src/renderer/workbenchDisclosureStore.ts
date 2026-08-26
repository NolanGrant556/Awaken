import { useCallback, useLayoutEffect, useSyncExternalStore } from "react";

type WorkbenchDisclosureValue = {
  open: boolean;
  source: "automatic" | "user";
};

export type WorkbenchDisclosureStore = {
  getSnapshot: (key: string) => boolean | undefined;
  set: (key: string, open: boolean) => void;
  setAutomatic: (key: string, open: boolean) => void;
  subscribe: (key: string, listener: () => void) => () => void;
};

type WorkbenchDisclosureStorage = Pick<Storage, "getItem" | "setItem">;

const SESSION_STORAGE_PREFIX = "awaken:workbench-disclosure:";

function rendererSessionStorage(): WorkbenchDisclosureStorage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

/**
 * Keeps disclosure choices outside App render state. A single tool, batch, or
 * execution-process toggle notifies only the leaf subscribed to that stable
 * identity, while the values still survive temporary Workbench unmounts in the
 * current Desktop lifecycle.
 */
export function createWorkbenchDisclosureStore(
  storage?: WorkbenchDisclosureStorage,
): WorkbenchDisclosureStore {
  const values = new Map<string, WorkbenchDisclosureValue>();
  const hydratedKeys = new Set<string>();
  const listeners = new Map<string, Set<() => void>>();

  function persist(key: string, value: WorkbenchDisclosureValue): void {
    try {
      storage?.setItem(
        `${SESSION_STORAGE_PREFIX}${key}`,
        value.source === "automatic"
          ? `automatic:${String(value.open)}`
          : String(value.open),
      );
    } catch {
      // Quota or privacy restrictions must not block a local disclosure toggle.
    }
  }

  function notify(key: string): void {
    for (const listener of listeners.get(key) ?? []) {
      listener();
    }
  }

  function hydrate(key: string): void {
    if (hydratedKeys.has(key)) return;
    hydratedKeys.add(key);
    try {
      const stored = storage?.getItem(`${SESSION_STORAGE_PREFIX}${key}`);
      if (stored === "true" || stored === "false") {
        values.set(key, { open: stored === "true", source: "user" });
      }
      if (stored === "automatic:true" || stored === "automatic:false") {
        values.set(key, {
          open: stored === "automatic:true",
          source: "automatic",
        });
      }
    } catch {
      // Storage is a resilience layer only; in-memory disclosure state remains usable.
    }
  }

  return {
    getSnapshot(key) {
      hydrate(key);
      return values.get(key)?.open;
    },
    set(key, open) {
      hydrate(key);
      const previous = values.get(key);
      if (previous?.open === open && previous.source === "user") return;
      const next = { open, source: "user" as const };
      values.set(key, next);
      persist(key, next);
      if (previous?.open !== open) notify(key);
    },
    setAutomatic(key, open) {
      hydrate(key);
      const previous = values.get(key);
      if (previous?.source === "user" || previous?.open === open) return;
      const next = { open, source: "automatic" as const };
      values.set(key, next);
      persist(key, next);
      notify(key);
    },
    subscribe(key, listener) {
      const keyListeners = listeners.get(key) ?? new Set<() => void>();
      keyListeners.add(listener);
      listeners.set(key, keyListeners);
      return () => {
        keyListeners.delete(listener);
        if (keyListeners.size === 0) listeners.delete(key);
      };
    },
  };
}

/**
 * One store per renderer lifecycle. Keeping this at module scope preserves
 * disclosure choices even if the Workbench or App tree is temporarily
 * remounted while navigating to another Desktop surface. A renderer restart
 * reloads the module and therefore resets every disclosure to its default.
 */
export const rendererWorkbenchDisclosureStore = createWorkbenchDisclosureStore(
  rendererSessionStorage(),
);

export function useWorkbenchDisclosure(
  store: WorkbenchDisclosureStore,
  key: string,
): readonly [boolean | undefined, (open: boolean) => void] {
  const subscribe = useCallback(
    (listener: () => void) => store.subscribe(key, listener),
    [key, store],
  );
  const getSnapshot = useCallback(() => store.getSnapshot(key), [key, store]);
  const open = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const setOpen = useCallback((nextOpen: boolean) => store.set(key, nextOpen), [key, store]);
  return [open, setOpen] as const;
}

/**
 * Keeps a live Activity Batch's automatic open state stable when its
 * Workbench subtree is temporarily unmounted. Replayed history may reuse the
 * last automatic value, while new live status transitions still open/close
 * automatically. An explicit user choice always wins.
 */
export function useAutomaticWorkbenchDisclosure(
  store: WorkbenchDisclosureStore,
  key: string,
  options: { active: boolean; replayed: boolean },
): readonly [boolean | undefined, (open: boolean) => void] {
  const [open, setOpen] = useWorkbenchDisclosure(store, key);

  useLayoutEffect(() => {
    if (options.active) {
      store.setAutomatic(key, true);
      return;
    }
    if (!options.replayed) {
      store.setAutomatic(key, false);
    }
  }, [key, options.active, options.replayed, store]);

  return [open, setOpen] as const;
}
