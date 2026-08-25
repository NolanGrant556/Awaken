import { useCallback, useSyncExternalStore } from "react";

export type WorkbenchDisclosureStore = {
  getSnapshot: (key: string) => boolean | undefined;
  set: (key: string, open: boolean) => void;
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
  const values = new Map<string, boolean>();
  const hydratedKeys = new Set<string>();
  const listeners = new Map<string, Set<() => void>>();

  function hydrate(key: string): void {
    if (hydratedKeys.has(key)) return;
    hydratedKeys.add(key);
    try {
      const stored = storage?.getItem(`${SESSION_STORAGE_PREFIX}${key}`);
      if (stored === "true") values.set(key, true);
      if (stored === "false") values.set(key, false);
    } catch {
      // Storage is a resilience layer only; in-memory disclosure state remains usable.
    }
  }

  return {
    getSnapshot(key) {
      hydrate(key);
      return values.get(key);
    },
    set(key, open) {
      hydrate(key);
      if (values.get(key) === open) return;
      values.set(key, open);
      try {
        storage?.setItem(`${SESSION_STORAGE_PREFIX}${key}`, String(open));
      } catch {
        // Quota or privacy restrictions must not block a local disclosure toggle.
      }
      for (const listener of listeners.get(key) ?? []) {
        listener();
      }
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
