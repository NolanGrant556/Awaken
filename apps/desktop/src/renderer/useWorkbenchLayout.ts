import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import { useEffect, useRef, useState } from "react";
import {
  DEFAULT_SIDEBAR_WIDTH,
  INSPECTOR_RESPONSIVE_COLLAPSE_QUERY,
  LAYOUT_STEP,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
} from "./desktopLayoutConstants";
import {
  clampNumber,
  readStoredBoolean,
  readStoredNumber,
  writeStoredValue,
} from "./desktopStorage";
import { workbenchInspectorPresentation } from "./desktopSurfacePresentation";
import {
  DEFAULT_INSPECTOR_WIDTH,
  FILE_WORKSPACE_INSPECTOR_CAP,
  MAX_INSPECTOR_WIDTH,
  MIN_INSPECTOR_WIDTH,
  computeEffectiveInspectorWidth,
  inspectorWidthCap,
  readWindowWidth,
  sanitizeInspectorPreference,
} from "./workbenchLayout";
import type { WorkbenchNavSection } from "./desktopSurfacePresentation";

export function useWorkbenchLayout(
  fileWorkspaceOpen: boolean,
  activeNavSection: WorkbenchNavSection | undefined,
) {
  const inspectorCollapsedValueRef = useRef(false);

  const inspectorCollapsedBeforeResponsiveRef = useRef<boolean | null>(null);

  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  const [sidebarWidth, setSidebarWidth] = useState(() =>
    readStoredNumber(
      "awaken:layout:sidebarWidth",
      DEFAULT_SIDEBAR_WIDTH,
      MIN_SIDEBAR_WIDTH,
      MAX_SIDEBAR_WIDTH,
    ),
  );

  const [inspectorWidth, setInspectorWidth] = useState(() =>
    sanitizeInspectorPreference(
      readStoredNumber(
        "awaken:layout:inspectorWidth",
        DEFAULT_INSPECTOR_WIDTH,
        MIN_INSPECTOR_WIDTH,
        MAX_INSPECTOR_WIDTH,
      ),
    ),
  );

  const [fileWorkspaceInspectorWidth, setFileWorkspaceInspectorWidth] =
    useState<number | undefined>();

  const [windowWidth, setWindowWidth] = useState(readWindowWidth);

  const [inspectorCollapsed, setInspectorCollapsed] = useState(() =>
    readStoredBoolean("awaken:layout:inspectorCollapsed", false),
  );

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const handleResize = () => {
      setWindowWidth(readWindowWidth());
    };
    window.addEventListener("resize", handleResize);
    return () => {
      window.removeEventListener("resize", handleResize);
    };
  }, []);

  useEffect(() => {
    writeStoredValue("awaken:layout:sidebarWidth", String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorWidth", String(inspectorWidth));
  }, [inspectorWidth]);

  useEffect(() => {
    writeStoredValue(
      "awaken:layout:inspectorCollapsed",
      String(inspectorCollapsed),
    );
    inspectorCollapsedValueRef.current = inspectorCollapsed;
  }, [inspectorCollapsed]);

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return;
    }

    const media = window.matchMedia(INSPECTOR_RESPONSIVE_COLLAPSE_QUERY);
    const handleResponsiveInspector = (matches: boolean) => {
      if (matches) {
        if (inspectorCollapsedBeforeResponsiveRef.current === null) {
          inspectorCollapsedBeforeResponsiveRef.current =
            inspectorCollapsedValueRef.current;
          setInspectorCollapsed(true);
        }
        return;
      }

      const previousCollapsed = inspectorCollapsedBeforeResponsiveRef.current;
      inspectorCollapsedBeforeResponsiveRef.current = null;
      if (previousCollapsed !== null) {
        setInspectorCollapsed(previousCollapsed);
      }
    };
    const handleChange = (event: MediaQueryListEvent) => {
      handleResponsiveInspector(event.matches);
    };

    handleResponsiveInspector(media.matches);
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent): void {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "b") {
        event.preventDefault();
        setSidebarCollapsed((collapsed) => !collapsed);
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  const inspectorPresentation = workbenchInspectorPresentation(
    activeNavSection,
    inspectorCollapsed,
  );

  const inspectorCap = inspectorWidthCap({
    fileWorkspaceOpen,
    preferenceWidth: inspectorWidth,
    overrideWidth: fileWorkspaceInspectorWidth,
  });

  const effectiveInspectorWidth = computeEffectiveInspectorWidth({
    windowWidth,
    sidebarWidth,
    sidebarCollapsed,
    inspectorAvailable: inspectorPresentation.available,
    inspectorCollapsed: inspectorPresentation.effectiveCollapsed,
    cap: inspectorCap,
  });

  function handleSidebarResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = sidebarWidth;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth + moveEvent.clientX - startX,
        MIN_SIDEBAR_WIDTH,
        MAX_SIDEBAR_WIDTH,
      );
      setSidebarWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove(
        "is-layout-resizing",
        "is-sidebar-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-sidebar-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleInspectorResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = effectiveInspectorWidth || inspectorWidth;
    const resizeMax = fileWorkspaceOpen
      ? FILE_WORKSPACE_INSPECTOR_CAP
      : MAX_INSPECTOR_WIDTH;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth - (moveEvent.clientX - startX),
        MIN_INSPECTOR_WIDTH,
        resizeMax,
      );
      if (fileWorkspaceOpen) {
        setFileWorkspaceInspectorWidth(nextWidth);
        return;
      }
      setInspectorWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove(
        "is-layout-resizing",
        "is-inspector-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-inspector-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleSidebarResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? LAYOUT_STEP : -LAYOUT_STEP;
    setSidebarWidth((width) =>
      clampNumber(width + delta, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH),
    );
  }

  function handleInspectorResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowLeft" ? LAYOUT_STEP : -LAYOUT_STEP;
    const resizeMax = fileWorkspaceOpen
      ? FILE_WORKSPACE_INSPECTOR_CAP
      : MAX_INSPECTOR_WIDTH;
    if (fileWorkspaceOpen) {
      setFileWorkspaceInspectorWidth((width) =>
        clampNumber(
          (width ?? FILE_WORKSPACE_INSPECTOR_CAP) + delta,
          MIN_INSPECTOR_WIDTH,
          resizeMax,
        ),
      );
      return;
    }
    setInspectorWidth((width) =>
      clampNumber(width + delta, MIN_INSPECTOR_WIDTH, resizeMax),
    );
  }

  const layoutStyle = {
    "--aw-sidebar-width": `${sidebarWidth}px`,
    "--aw-inspector-width": `${effectiveInspectorWidth}px`,
  } as CSSProperties;
  return {
    sidebarCollapsed,
    setSidebarCollapsed,
    sidebarWidth,
    inspectorWidth,
    fileWorkspaceInspectorWidth,
    setFileWorkspaceInspectorWidth,
    windowWidth,
    inspectorCollapsed,
    setInspectorCollapsed,
    inspectorCollapsedBeforeResponsiveRef,
    inspectorPresentation,
    inspectorCap,
    effectiveInspectorWidth,
    layoutStyle,
    handleSidebarResizeStart,
    handleInspectorResizeStart,
    handleSidebarResizeKeyDown,
    handleInspectorResizeKeyDown,
  };
}
