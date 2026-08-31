import "./AwakenComposerBar.css";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  ReactNode,
  Ref,
} from "react";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenComposerInput, type ComposerInputSelection } from "../AwakenComposerInput";
import { AwakenContextButton } from "../AwakenContextButton";
import { AwakenDropdown } from "../AwakenDropdown";
import { AwakenMemoryLightFlow } from "../AwakenMemoryLightFlow";
import { AwakenSendButton } from "../AwakenSendButton";
import { AwakenTooltip } from "../AwakenTooltip";
import { useAwakenAnchoredMenuLayout } from "../../hooks/useAwakenAnchoredMenuLayout";
import {
  AwakenComposerInlineInput,
  composerInlinePlainTextForRange,
  type ComposerInlineInputElement,
  type ComposerInlineStructuredValue,
} from "./AwakenComposerInlineInput";

const CONTEXT_RING_RADIUS = 7;
const CONTEXT_RING_CIRCUMFERENCE = 2 * Math.PI * CONTEXT_RING_RADIUS;

export { composerInlinePlainTextForRange };
const COMPOSER_EFFORT_TRACK_INSET_PX = 14;
const COMPOSER_EFFORT_DETENT_EDGE_PROGRESS = 0.08;
const COMPOSER_EFFORT_MAX_ZONE_MIN_PROGRESS = 0.92;
const COMPOSER_EFFORT_HYSTERESIS_PX = 8;
const COMPOSER_MODEL_MENU_MAX_HEIGHT = 327;

function estimateComposerModelMenuHeight(
  optionCount: number,
  hasFooterAction: boolean,
  showingEffortPanel: boolean,
): number {
  if (showingEffortPanel) return 188;
  return Math.min(
    COMPOSER_MODEL_MENU_MAX_HEIGHT,
    16 + optionCount * 40 + (hasFooterAction ? 49 : 0),
  );
}

export type ComposerBarMetaMenuItem = {
  id: string;
  label: string;
  icon?: ReactNode;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
  effort?: {
    levels: Array<"low" | "high" | "max">;
    selectedLevel: "low" | "high" | "max";
    displayLabels?: Partial<Record<"low" | "high" | "max", string>>;
    note?: string;
    onChange: (level: "low" | "high" | "max") => void;
  };
};

export type ComposerBarMetaAction = {
  label: string;
  icon?: ReactNode;
  onClick: () => void;
};

export type ComposerBarMetaItem = {
  id: string;
  label: string;
  value?: string;
  progress?: number;
  icon?: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  buttonRef?: Ref<HTMLButtonElement>;
  ariaHaspopup?: "dialog" | "menu" | "listbox" | "tree" | "grid";
  ariaExpanded?: boolean;
  ariaControls?: string;
  menuItems?: ComposerBarMetaMenuItem[];
  menuFooterAction?: ComposerBarMetaAction;
};

export type ComposerBarProjectOption = {
  id: string;
  label: string;
  /** `undefined` uses the default folder icon; `null` renders no icon. */
  icon?: ReactNode;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
};

export type ComposerBarProjectAction = {
  label: string;
  disabled?: boolean;
  onClick: () => void;
};

export type ComposerBarProjectPicker = {
  label?: string;
  placeholder?: string;
  options: ComposerBarProjectOption[];
  emptyLabel?: string;
  clearAction?: ComposerBarProjectAction;
  createAction?: ComposerBarProjectAction;
  disabled?: boolean;
};

export type ComposerBarAttachment = {
  id: string;
  kind: "image" | "file";
  src?: string;
  alt: string;
  mimeType: string;
  label?: string;
  detail?: string;
  tooltip?: string;
  extension?: string;
};

export type ComposerBarToken = {
  id: string;
  type: string;
  label: string;
  title?: string;
  leadingText?: string;
  placement?: "before" | "after";
};

export type ComposerBarStructuredInputValue = ComposerInlineStructuredValue;
export type ComposerBarInputSelection = ComposerInputSelection;

export type ComposerBarSlashAction = {
  id: string;
  type: string;
  label: string;
  sectionLabel?: string;
  description?: string;
  icon?: ReactNode;
  disabled?: boolean;
  disabledReason?: string;
  preview?: {
    description?: string;
    path?: string;
  };
};

export type ComposerBarSlashPalette = {
  open: boolean;
  actions: ComposerBarSlashAction[];
  activeActionId?: string;
  loading?: boolean;
  error?: string | null;
  emptyLabel?: string;
  onSelectAction?: (actionId: string) => void;
  onActiveActionChange?: (actionId: string) => void;
};

export type ComposerBarPluginOption = {
  id: string;
  label: string;
  selected?: boolean;
  disabled?: boolean;
  disabledReason?: string;
};

export type ComposerBarPluginPicker = {
  label?: string;
  emptyLabel?: string;
  options: ComposerBarPluginOption[];
  loading?: boolean;
  error?: string | null;
  disabled?: boolean;
  onSelectOption?: (optionId: string) => void;
};

type SlashPreviewState = {
  actionId: string;
  style: CSSProperties;
};

type SlashPaletteStyle = CSSProperties & {
  "--composer-bar-slash-max-height"?: string;
};

const HIDDEN_SLASH_PALETTE_STYLE: SlashPaletteStyle = { visibility: "hidden" };

type ComposerBarSlashActionSection = {
  label?: string;
  actions: ComposerBarSlashAction[];
};

function slashActionSections(
  actions: ComposerBarSlashAction[],
): ComposerBarSlashActionSection[] {
  return actions.reduce<ComposerBarSlashActionSection[]>((sections, action) => {
    const label = action.sectionLabel?.trim() || undefined;
    const current = sections[sections.length - 1];
    if (current && current.label === label) {
      current.actions.push(action);
      return sections;
    }
    sections.push({ label, actions: [action] });
    return sections;
  }, []);
}

export type SlashPreviewAnchorRect = {
  left: number;
  right: number;
  top: number;
};

export type SlashPreviewViewport = {
  width: number;
  height: number;
};

export type SlashPreviewContainerRect = {
  left: number;
  top: number;
};

export function slashPreviewStyleForAnchor(
  rect: SlashPreviewAnchorRect,
  viewport: SlashPreviewViewport,
  containerRect?: SlashPreviewContainerRect,
): CSSProperties {
  const margin = 8;
  const gap = 6;
  const availableWidth = Math.max(0, viewport.width - margin * 2);
  const width = Math.min(220, availableWidth);
  const heightEstimate = Math.min(150, Math.max(0, viewport.height - margin * 2));

  let left = rect.right + gap;
  if (left + width > viewport.width - margin) {
    left = rect.left - width - gap;
  }
  left = Math.min(Math.max(margin, left), Math.max(margin, viewport.width - width - margin));

  let top = rect.top - 2;
  if (top + heightEstimate > viewport.height - margin) {
    top = viewport.height - heightEstimate - margin;
  }
  top = Math.max(margin, top);

  return {
    left: containerRect ? left - containerRect.left : left,
    top: containerRect ? top - containerRect.top : top,
    width,
  };
}

export function slashPaletteStyleForAnchor(
  anchorTop: number,
  viewportHeight: number,
): SlashPaletteStyle {
  const margin = 8;
  const gap = 6;
  const maxHeight = Math.max(88, Math.min(210, Math.floor(anchorTop - margin - gap)));

  return { "--composer-bar-slash-max-height": `${maxHeight}px` };
}

export function slashPaletteStyleForCaret(
  caretRect: { left: number; top: number },
  inputRect: { left: number; right: number; bottom: number },
  viewport: { width: number; height: number },
): SlashPaletteStyle {
  const margin = 8;
  const gap = 5;
  const paletteWidth = Math.min(260, Math.max(0, viewport.width - 24));
  const viewportLeft = Math.min(
    Math.max(margin, caretRect.left),
    Math.max(margin, viewport.width - paletteWidth - margin),
  );
  return {
    ...slashPaletteStyleForAnchor(caretRect.top, viewport.height),
    left: `${viewportLeft - inputRect.left}px`,
    bottom: `${Math.max(gap, inputRect.bottom - caretRect.top + gap)}px`,
  };
}

export type AwakenComposerBarProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  submitOnEnter?: boolean;
  onInputKeyDown?: (
    event: ReactKeyboardEvent<HTMLTextAreaElement | ComposerInlineInputElement>,
  ) => void;
  onStructuredInputChange?: (value: ComposerBarStructuredInputValue) => void;
  inputAriaLabel?: string;
  onAddContext?: () => void;
  placeholder?: string;
  disabled?: boolean;
  isReady?: boolean;
  submitMode?: "send" | "stop";
  variant?: "center" | "dock" | "embedded";
  metaItems?: ComposerBarMetaItem[];
  projectPicker?: ComposerBarProjectPicker;
  focusRequest?: number;
  focusOffset?: number;
  onInputSelectionChange?: (selection: ComposerBarInputSelection) => void;
  submitDisabled?: boolean;
  attachments?: ComposerBarAttachment[];
  attachmentWarning?: ReactNode;
  tokens?: ComposerBarToken[];
  fileComments?: ComposerBarFileComment[];
  slashPalette?: ComposerBarSlashPalette;
  pluginPicker?: ComposerBarPluginPicker;
  onRemoveAttachment?: (attachmentId: string) => void;
  onRemoveToken?: (tokenId: string) => void;
  onRemoveFileComment?: (commentId: string) => void;
  onClearFileComments?: () => void;
  onPasteImages?: (files: File[]) => void;
};

export type ComposerBarFileComment = {
  id: string;
  displayName: string;
  relativePath: string;
  startLine: number;
  endLine: number;
  selectedText: string;
  comment: string;
  validationState?: "ready" | "stale";
};

export type AwakenFileCommentSummaryProps = {
  comments: ComposerBarFileComment[];
  onRemoveComment?: (commentId: string) => void;
  onClear?: () => void;
  className?: string;
};

function fileCommentRangeLabel(comment: ComposerBarFileComment): string {
  return comment.startLine === comment.endLine
    ? `第 ${comment.startLine} 行`
    : `第 ${comment.startLine}–${comment.endLine} 行`;
}

export function AwakenFileCommentSummary({
  comments,
  onRemoveComment,
  onClear,
  className,
}: AwakenFileCommentSummaryProps) {
  const [open, setOpen] = useState(false);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => {
    if (closeTimerRef.current) clearTimeout(closeTimerRef.current);
  }, []);

  if (comments.length === 0) return null;

  const cancelClose = () => {
    if (closeTimerRef.current) clearTimeout(closeTimerRef.current);
  };
  const scheduleClose = () => {
    cancelClose();
    closeTimerRef.current = setTimeout(() => setOpen(false), 120);
  };

  return (
    <span
      className={`composer-file-comments${className ? ` ${className}` : ""}`}
      onPointerEnter={() => {
        cancelClose();
        setOpen(true);
      }}
      onPointerLeave={scheduleClose}
      onFocusCapture={() => {
        cancelClose();
        setOpen(true);
      }}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) scheduleClose();
      }}
    >
      <button
        type="button"
        className="composer-file-comments-trigger"
        aria-label={`${comments.length} 个文件评论`}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setOpen(false);
        }}
      >
        <AwakenIcon concept="file-comment" size={16} />
        <span>{comments.length} 个评论</span>
      </button>
      {open ? (
        <span className="composer-file-comments-popover" role="dialog" aria-label="文件评论详情">
          <span className="composer-file-comments-header">
            <strong>文件评论</strong>
            {onClear ? (
              <button type="button" onClick={onClear}>全部移除</button>
            ) : null}
          </span>
          <span className="composer-file-comments-list">
            {comments.map((comment) => (
              <span key={comment.id} className="composer-file-comment-card">
                <span className="composer-file-comment-location">
                  <strong>{comment.relativePath || comment.displayName}</strong>
                  <span>{fileCommentRangeLabel(comment)}</span>
                </span>
                <code>{comment.selectedText}</code>
                <span className="composer-file-comment-body">{comment.comment}</span>
                {comment.validationState === "stale" ? (
                  <span className="composer-file-comment-stale">文件已变化，需要重新选择</span>
                ) : null}
                {onRemoveComment ? (
                  <button
                    type="button"
                    className="composer-file-comment-remove"
                    aria-label={`移除 ${comment.relativePath} ${fileCommentRangeLabel(comment)} 评论`}
                    onClick={() => onRemoveComment(comment.id)}
                  >
                    移除
                  </button>
                ) : null}
              </span>
            ))}
          </span>
        </span>
      ) : null}
    </span>
  );
}

export function shouldActivateComposerMetaItemKey(key: string): boolean {
  return key === " " || key === "Spacebar";
}

function ComposerContextUsageMeter({
  item,
  progressPercent,
  ring,
}: {
  item: ComposerBarMetaItem;
  progressPercent: number;
  ring: ReactNode;
}) {
  const [suppressTooltip, setSuppressTooltip] = useState(false);
  const panelWasOpenRef = useRef(false);

  useEffect(() => {
    if (item.ariaExpanded) {
      panelWasOpenRef.current = true;
      setSuppressTooltip(true);
      return;
    }
    if (panelWasOpenRef.current) {
      setSuppressTooltip(true);
    }
  }, [item.ariaExpanded]);

  const accessibleName = item.value ? `${item.label}，${item.value}` : item.label;
  const tooltipLabel = (
    <span className="composer-bar-context-tooltip">
      <span className="composer-bar-context-tooltip-summary">{progressPercent}% 当前使用</span>
      {item.value ? (
        <span className="composer-bar-context-tooltip-detail">{item.value}</span>
      ) : null}
    </span>
  );

  return (
    <span
      className="composer-bar-meta-meter-anchor"
      onPointerLeave={() => {
        if (!item.ariaExpanded) {
          panelWasOpenRef.current = false;
          setSuppressTooltip(false);
        }
      }}
    >
      <AwakenTooltip
        label={tooltipLabel}
        openOnFocus={false}
        disabled={Boolean(item.disabled || item.ariaExpanded || suppressTooltip)}
      >
        <button
          ref={item.buttonRef}
          type="button"
          className="composer-bar-meta-item composer-bar-meta-item-meter composer-bar-meta-item-button"
          aria-label={accessibleName}
          aria-haspopup={item.ariaHaspopup}
          aria-expanded={item.ariaExpanded}
          aria-controls={item.ariaControls}
          disabled={item.disabled}
          onClick={() => {
            setSuppressTooltip(true);
            item.onClick?.();
          }}
          onKeyDown={(event) => {
            if (shouldActivateComposerMetaItemKey(event.key)) {
              event.preventDefault();
              setSuppressTooltip(true);
              item.onClick?.();
            }
          }}
        >
          <span
            className="composer-bar-context-meter"
            role="meter"
            aria-label={item.label}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={progressPercent}
            aria-valuetext={item.value ?? `${progressPercent}%`}
          >
            {ring}
          </span>
        </button>
      </AwakenTooltip>
    </span>
  );
}

export function AwakenComposerBar({
  value,
  onChange,
  onSubmit,
  submitOnEnter = true,
  onInputKeyDown,
  onStructuredInputChange,
  inputAriaLabel = "输入任务",
  onAddContext,
  placeholder,
  disabled,
  isReady = false,
  submitMode = "send",
  variant = "dock",
  metaItems = [],
  projectPicker,
  focusRequest = 0,
  focusOffset,
  onInputSelectionChange,
  submitDisabled = false,
  attachments = [],
  attachmentWarning,
  tokens = [],
  fileComments = [],
  slashPalette,
  pluginPicker,
  onRemoveAttachment,
  onRemoveToken,
  onRemoveFileComment,
  onClearFileComments,
  onPasteImages,
}: AwakenComposerBarProps) {
  const canSubmit = submitMode === "stop" || (!disabled && !submitDisabled);
  const [openMetaMenuId, setOpenMetaMenuId] = useState<string | null>(null);
  const [activeModelOptionId, setActiveModelOptionId] = useState<string | null>(null);
  const [pendingModelEffort, setPendingModelEffort] = useState<"low" | "high" | "max" | null>(null);
  const modelEffortDraggingRef = useRef(false);
  const modelEffortValueRef = useRef<"low" | "high" | "max" | null>(null);
  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [pluginPickerOpen, setPluginPickerOpen] = useState(false);
  const [slashPreviewState, setSlashPreviewState] = useState<SlashPreviewState | null>(null);
  const [slashPaletteStyle, setSlashPaletteStyle] = useState<SlashPaletteStyle>(
    HIDDEN_SLASH_PALETTE_STYLE,
  );
  const [inputSelection, setInputSelection] = useState<ComposerBarInputSelection>();
  const inputWrapRef = useRef<HTMLDivElement | null>(null);
  const slashPaletteRef = useRef<HTMLDivElement | null>(null);
  const modelMetaItem = metaItems.find((item) => item.id === "model");
  const {
    anchorRef: modelMetaRef,
    menuRef: modelMenuRef,
    placement: modelMenuPlacement,
    maxHeight: modelMenuMaxHeight,
    updateLayout: updateModelMenuLayout,
  } = useAwakenAnchoredMenuLayout<HTMLDivElement, HTMLDivElement>({
    open: Boolean(openMetaMenuId),
    estimatedHeight: estimateComposerModelMenuHeight(
      modelMetaItem?.menuItems?.length ?? 0,
      Boolean(modelMetaItem?.menuFooterAction),
      Boolean(activeModelOptionId),
    ),
    maxHeight: COMPOSER_MODEL_MENU_MAX_HEIGHT,
    preferredPlacement: variant === "dock" ? "top" : "bottom",
    gap: 8,
  });
  const selectedProjectOption = projectPicker?.options.find((option) => option.selected);
  const showProjectPicker = variant === "center" && Boolean(projectPicker);
  const pluginOptions = pluginPicker?.options ?? [];
  const showPluginPicker = variant !== "embedded" && Boolean(pluginPicker);
  const topMetaItems: ComposerBarMetaItem[] = [];
  const bottomMetaItems = variant === "dock" ? metaItems.filter((item) => item.id !== "model") : [];
  const slashActions = slashPalette?.actions ?? [];
  const slashSections = slashActionSections(slashActions);
  const slashPreviewAction = slashActions.find(
    (action) => action.id === slashPreviewState?.actionId,
  );
  const slashPreviewDescription =
    slashPreviewAction?.preview?.description ??
    slashPreviewAction?.description ??
    slashPreviewAction?.disabledReason;
  const slashPreviewPath = slashPreviewAction?.preview?.path;
  const handleSubmit = useCallback(() => {
    if (canSubmit) {
      onSubmit?.();
    }
  }, [canSubmit, onSubmit]);
  const handleInputSelectionChange = useCallback(
    (selection: ComposerBarInputSelection): void => {
      setInputSelection(selection);
      onInputSelectionChange?.(selection);
    },
    [onInputSelectionChange],
  );

  useEffect(() => {
    if (!openMetaMenuId) return;

    function handlePointerDown(event: MouseEvent): void {
      if (!modelMetaRef.current?.contains(event.target as Node)) {
        setOpenMetaMenuId(null);
        setActiveModelOptionId(null);
      }
    }

    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        setOpenMetaMenuId(null);
        setActiveModelOptionId(null);
      }
    }

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [openMetaMenuId]);

  useEffect(() => {
    if (!showProjectPicker) {
      setProjectMenuOpen(false);
    }
  }, [showProjectPicker]);

  useEffect(() => {
    if (!showPluginPicker) {
      setPluginPickerOpen(false);
    }
  }, [showPluginPicker]);

  useEffect(() => {
    if (openMetaMenuId && activeModelOptionId) return;
    modelEffortDraggingRef.current = false;
  }, [activeModelOptionId, openMetaMenuId]);

  useEffect(() => {
    if (!slashPalette?.open) {
      setSlashPreviewState(null);
    }
  }, [slashPalette?.open]);

  useLayoutEffect(() => {
    if (!slashPalette?.open) {
      setSlashPaletteStyle(HIDDEN_SLASH_PALETTE_STYLE);
      return;
    }

    function updateSlashPaletteStyle(): void {
      const rect = inputWrapRef.current?.getBoundingClientRect();
      if (!rect) {
        setSlashPaletteStyle(HIDDEN_SLASH_PALETTE_STYLE);
        return;
      }
      const positionedStyle = inputSelection?.caretRect
        ? slashPaletteStyleForCaret(inputSelection.caretRect, rect, {
            width: window.innerWidth,
            height: window.innerHeight,
          })
        : slashPaletteStyleForAnchor(rect.top, window.innerHeight);
      setSlashPaletteStyle({ ...positionedStyle, visibility: "visible" });
    }

    updateSlashPaletteStyle();
    window.addEventListener("resize", updateSlashPaletteStyle);
    return () => window.removeEventListener("resize", updateSlashPaletteStyle);
  }, [inputSelection, slashPalette?.open, slashActions.length, value, tokens.length]);

  useEffect(() => {
    if (!slashPalette?.open || !slashPalette.activeActionId) {
      return;
    }
    const activeOption = slashPaletteRef.current?.querySelector<HTMLElement>(
      '.composer-bar-slash-option[aria-selected="true"]',
    );
    activeOption?.scrollIntoView?.({ block: "nearest" });
  }, [slashActions.length, slashPalette?.activeActionId, slashPalette?.open]);

  function renderContextUsageRing(progressPercent: number) {
    const dashOffset =
      CONTEXT_RING_CIRCUMFERENCE -
      (CONTEXT_RING_CIRCUMFERENCE * progressPercent) / 100;
    return (
      <svg
        className="composer-bar-context-ring"
        viewBox="0 0 20 20"
        width={18}
        height={18}
        aria-hidden="true"
        focusable="false"
      >
        <circle
          className="composer-bar-context-ring-track"
          cx="10"
          cy="10"
          r={CONTEXT_RING_RADIUS}
          fill="none"
          strokeWidth="2.25"
        />
        <circle
          className="composer-bar-context-ring-fill"
          cx="10"
          cy="10"
          r={CONTEXT_RING_RADIUS}
          fill="none"
          strokeWidth="2.25"
          strokeDasharray={CONTEXT_RING_CIRCUMFERENCE}
          strokeDashoffset={dashOffset}
          strokeLinecap="round"
          transform="rotate(-90 10 10)"
        />
      </svg>
    );
  }

  function renderMetaItem(item: ComposerBarMetaItem) {
    const hasProgress = typeof item.progress === "number" && Number.isFinite(item.progress);
    const progressPercent = hasProgress
      ? Math.max(0, Math.min(100, Math.round((item.progress ?? 0) * 100)))
      : undefined;
    const isContextMeter = hasProgress && progressPercent !== undefined;

    if (isContextMeter) {
      return (
        <ComposerContextUsageMeter
          key={item.id}
          item={item}
          progressPercent={progressPercent}
          ring={renderContextUsageRing(progressPercent)}
        />
      );
    }

    const content = (
      <>
        {item.icon ? <span className="composer-bar-meta-icon">{item.icon}</span> : null}
        <span className="composer-bar-meta-label">{item.label}</span>
        {item.value ? <span className="composer-bar-meta-value">{item.value}</span> : null}
      </>
    );
    const className = `composer-bar-meta-item${
      item.onClick ? " composer-bar-meta-item-button" : ""
    }`;

    if (item.onClick) {
      return (
        <button
          key={item.id}
          ref={item.buttonRef}
          type="button"
          className={className}
          aria-haspopup={item.ariaHaspopup}
          aria-expanded={item.ariaExpanded}
          aria-controls={item.ariaControls}
          disabled={item.disabled}
          onClick={item.onClick}
          onKeyDown={(event) => {
            if (shouldActivateComposerMetaItemKey(event.key)) {
              event.preventDefault();
              item.onClick?.();
            }
          }}
        >
          {content}
        </button>
      );
    }

    return (
      <span key={item.id} className={className}>
        {content}
      </span>
    );
  }

  function renderModelMetaItem(item: ComposerBarMetaItem) {
    const hasMenu = Boolean(item.menuItems?.length);
    const isMenuOpen = openMetaMenuId === item.id;
    const isInteractive = !item.disabled && (hasMenu || item.onClick);
    const activeModelOption = item.menuItems?.find((option) => option.id === activeModelOptionId);
    const activeEffortLevels = activeModelOption?.effort?.levels ?? [];
    const pendingEffortIndex = pendingModelEffort
      ? activeEffortLevels.indexOf(pendingModelEffort)
      : -1;
    const effortProgressAtIndex = (index: number): number => {
      if (activeEffortLevels.length <= 1) return 0.5;
      return index / (activeEffortLevels.length - 1);
    };
    const effortDetentProgressAtIndex = (index: number): number => {
      if (index === activeEffortLevels.length - 1 && activeEffortLevels.length > 1) {
        return 1 - COMPOSER_EFFORT_DETENT_EDGE_PROGRESS;
      }
      return effortProgressAtIndex(index);
    };
    const pendingEffortProgress = pendingEffortIndex >= 0
      ? effortProgressAtIndex(pendingEffortIndex)
      : 0;
    const effortLabel = (level: "low" | "high" | "max") =>
      activeModelOption?.effort?.displayLabels?.[level] ?? level;

    function effortPointerInput(event: ReactPointerEvent<HTMLDivElement>): {
      progress: number;
      width: number;
    } {
      const rect = event.currentTarget.getBoundingClientRect();
      const activeWidth = Math.max(1, rect.width - COMPOSER_EFFORT_TRACK_INSET_PX * 2);
      const pointerX = Math.max(
        0,
        Math.min(
          activeWidth,
          event.clientX - rect.left - COMPOSER_EFFORT_TRACK_INSET_PX,
        ),
      );
      return {
        progress: pointerX / activeWidth,
        width: activeWidth,
      };
    }

    function setEffortDragProgress(element: HTMLDivElement, progress: number): void {
      element.style.setProperty(
        "--composer-effort-drag-progress",
        `${Math.max(0, Math.min(1, progress)) * 100}%`,
      );
    }

    function changeEffortAtIndex(nextIndex: number): void {
      const nextLevel = activeEffortLevels[nextIndex];
      if (!nextLevel || modelEffortValueRef.current === nextLevel) return;
      modelEffortValueRef.current = nextLevel;
      setPendingModelEffort(nextLevel);
      activeModelOption?.effort?.onChange(nextLevel);
    }

    function selectEffortAtProgress(
      pointerProgress: number,
      activeWidth: number,
      options: { useHysteresis: boolean },
    ): number | null {
      if (activeEffortLevels.length === 0) return null;
      const detentBoundary = (leftIndex: number) => {
        const midpoint = (
          effortProgressAtIndex(leftIndex) + effortProgressAtIndex(leftIndex + 1)
        ) / 2;
        if (activeEffortLevels[leftIndex + 1] === "max") {
          return Math.max(COMPOSER_EFFORT_MAX_ZONE_MIN_PROGRESS, midpoint);
        }
        return midpoint;
      };
      const effortDetentHysteresis = Math.min(
        0.06,
        COMPOSER_EFFORT_HYSTERESIS_PX / Math.max(1, activeWidth),
      );
      let directIndex = 0;
      while (
        directIndex < activeEffortLevels.length - 1 &&
        pointerProgress >= detentBoundary(directIndex)
      ) {
        directIndex += 1;
      }
      const currentIndex = modelEffortValueRef.current
        ? activeEffortLevels.indexOf(modelEffortValueRef.current)
        : -1;
      let nextIndex = directIndex;
      if (options.useHysteresis && currentIndex >= 0) {
        nextIndex = currentIndex;
        while (
          nextIndex < activeEffortLevels.length - 1 &&
          pointerProgress >=
            detentBoundary(nextIndex) + effortDetentHysteresis
        ) {
          nextIndex += 1;
        }
        while (
          nextIndex > 0 &&
          pointerProgress <
            detentBoundary(nextIndex - 1) - effortDetentHysteresis
        ) {
          nextIndex -= 1;
        }
      }
      changeEffortAtIndex(nextIndex);
      return nextIndex;
    }

    function stopEffortDragging(
      event: ReactPointerEvent<HTMLDivElement>,
      options: { commit: boolean },
    ): void {
      if (!modelEffortDraggingRef.current) return;
      if (options.commit) {
        const input = effortPointerInput(event);
        selectEffortAtProgress(input.progress, input.width, { useHysteresis: true });
      }
      modelEffortDraggingRef.current = false;
      delete event.currentTarget.dataset.dragging;
      event.currentTarget.style.removeProperty("--composer-effort-drag-progress");
      if (
        Number.isFinite(event.pointerId) &&
        event.currentTarget.hasPointerCapture?.(event.pointerId)
      ) {
        event.currentTarget.releasePointerCapture?.(event.pointerId);
      }
    }

    function handleEffortSliderKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
      delete event.currentTarget.dataset.pointerFocus;
      if (activeEffortLevels.length === 0) return;
      const currentIndex = pendingEffortIndex >= 0 ? pendingEffortIndex : 0;
      let nextIndex: number | undefined;
      if (event.key === "ArrowLeft" || event.key === "ArrowDown") {
        nextIndex = Math.max(0, currentIndex - 1);
      } else if (event.key === "ArrowRight" || event.key === "ArrowUp") {
        nextIndex = Math.min(activeEffortLevels.length - 1, currentIndex + 1);
      } else if (event.key === "Home") {
        nextIndex = 0;
      } else if (event.key === "End") {
        nextIndex = activeEffortLevels.length - 1;
      }
      if (nextIndex === undefined) return;
      event.preventDefault();
      changeEffortAtIndex(nextIndex);
    }
    const triggerContent = (
      <>
        {item.icon ? <span className="composer-bar-model-trigger-icon">{item.icon}</span> : null}
        <span className="composer-bar-model-trigger-label">{item.label}</span>
        {hasMenu ? (
          <span className="composer-bar-model-trigger-chevron" aria-hidden="true">
            <AwakenIcon concept="dropdown" size={14} />
          </span>
        ) : null}
      </>
    );

    return (
      <div
        className="composer-bar-toolbar-model"
        ref={modelMetaRef}
        data-menu-placement={modelMenuPlacement}
      >
        {isInteractive ? (
          <button
            type="button"
            className={`composer-bar-model-trigger${
              isMenuOpen ? " composer-bar-model-trigger-open" : ""
            }`}
            aria-label={item.label}
            title={item.label}
            aria-haspopup={hasMenu ? "listbox" : undefined}
            aria-expanded={hasMenu ? isMenuOpen : undefined}
            onClick={(event) => {
              if (hasMenu) {
                if (!isMenuOpen) {
                  updateModelMenuLayout();
                  event.currentTarget.style.setProperty(
                    "--composer-model-trigger-collapsed-width",
                    `${event.currentTarget.getBoundingClientRect().width}px`,
                  );
                }
                setOpenMetaMenuId((current) => {
                  const next = current === item.id ? null : item.id;
                  if (!next) setActiveModelOptionId(null);
                  return next;
                });
                return;
              }
              item.onClick?.();
            }}
          >
            {triggerContent}
          </button>
        ) : (
          <span
            className="composer-bar-model-trigger composer-bar-model-trigger-disabled"
            aria-label={item.label}
            title={item.label}
          >
            {triggerContent}
          </span>
        )}
        {hasMenu && isMenuOpen ? (
          <div
            ref={modelMenuRef}
            className="composer-bar-model-menu"
            style={{ maxHeight: modelMenuMaxHeight }}
            role="listbox"
            aria-label="选择模型"
          >
            {activeModelOption?.effort ? (
              <div className="composer-bar-model-effort-panel" role="dialog" aria-label={`${activeModelOption.label} 推理强度`}>
                <button
                  type="button"
                  className="composer-bar-model-effort-back"
                  onClick={() => setActiveModelOptionId(null)}
                  aria-label="返回模型列表"
                >
                  <AwakenIcon concept="back" size={16} />
                  {activeModelOption.icon ? (
                    <span className="composer-bar-model-option-icon composer-bar-model-effort-icon">
                      {activeModelOption.icon}
                    </span>
                  ) : null}
                  <span>{activeModelOption.label}</span>
                </button>
                <div
                  className="composer-bar-model-effort-slider"
                  role="slider"
                  tabIndex={0}
                  aria-label="推理强度"
                  aria-orientation="horizontal"
                  aria-valuemin={0}
                  aria-valuemax={Math.max(0, activeEffortLevels.length - 1)}
                  aria-valuenow={Math.max(0, pendingEffortIndex)}
                  aria-valuetext={pendingModelEffort ? effortLabel(pendingModelEffort) : undefined}
                  data-effort={pendingModelEffort ?? undefined}
                  style={
                    {
                      "--composer-effort-track-inset": `${COMPOSER_EFFORT_TRACK_INSET_PX}px`,
                      "--composer-effort-selected-progress": `${pendingEffortProgress * 100}%`,
                    } as CSSProperties
                  }
                  onKeyDown={handleEffortSliderKeyDown}
                  onBlur={(event) => {
                    delete event.currentTarget.dataset.pointerFocus;
                  }}
                  onPointerMove={(event) => {
                    if (modelEffortDraggingRef.current) {
                      const input = effortPointerInput(event);
                      setEffortDragProgress(event.currentTarget, input.progress);
                      selectEffortAtProgress(input.progress, input.width, {
                        useHysteresis: true,
                      });
                    }
                  }}
                  onPointerDown={(event) => {
                    if (event.button !== 0 || event.isPrimary === false) return;
                    event.preventDefault();
                    modelEffortDraggingRef.current = true;
                    event.currentTarget.dataset.dragging = "true";
                    event.currentTarget.dataset.pointerFocus = "true";
                    if (Number.isFinite(event.pointerId)) {
                      event.currentTarget.setPointerCapture?.(event.pointerId);
                    }
                    const input = effortPointerInput(event);
                    setEffortDragProgress(event.currentTarget, input.progress);
                    selectEffortAtProgress(input.progress, input.width, {
                      useHysteresis: false,
                    });
                    event.currentTarget.focus();
                  }}
                  onPointerUp={(event) => stopEffortDragging(event, { commit: true })}
                  onPointerCancel={(event) => stopEffortDragging(event, { commit: false })}
                  onLostPointerCapture={(event) =>
                    stopEffortDragging(event, { commit: false })
                  }
                >
                  {pendingModelEffort ? (
                    <span className="composer-bar-model-effort-track" aria-hidden="true">
                      <AwakenMemoryLightFlow
                        className="composer-bar-model-effort-flow"
                        theme="dark"
                        tone={pendingModelEffort === "max" ? "danger" : "effort"}
                        variant="compact"
                      />
                      <span className="composer-bar-model-effort-detents">
                        {activeEffortLevels.slice(1).map((level, offset) => {
                          const index = offset + 1;
                          return (
                            <span
                              key={level}
                              className="composer-bar-model-effort-detent"
                              data-level={level}
                              data-reached={index <= pendingEffortIndex ? "true" : "false"}
                              style={
                                {
                                  "--composer-effort-detent-progress": `${
                                    effortDetentProgressAtIndex(index) * 100
                                  }%`,
                                } as CSSProperties
                              }
                            />
                          );
                        })}
                      </span>
                    </span>
                  ) : null}
                  <span className="composer-bar-model-effort-thumb-rail" aria-hidden="true">
                    <span className="composer-bar-model-effort-thumb" />
                  </span>
                </div>
                <div className="composer-bar-model-effort-labels" aria-hidden="true">
                  {activeEffortLevels.map((level) => (
                    <span
                      key={level}
                      data-selected={level === pendingModelEffort ? "true" : "false"}
                    >
                      {effortLabel(level)}
                    </span>
                  ))}
                </div>
                {activeModelOption.effort.note ? (
                  <p className="composer-bar-model-effort-note">{activeModelOption.effort.note}</p>
                ) : null}
              </div>
            ) : (
            <div className="composer-bar-model-menu-scroll">
              {item.menuItems?.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={`composer-bar-model-option${
                    option.selected ? " composer-bar-model-option-selected" : ""
                  }`}
                  role="option"
                  aria-selected={Boolean(option.selected)}
                  disabled={option.disabled}
                  onClick={() => {
                    if (option.effort) {
                      option.onSelect?.();
                      setActiveModelOptionId(option.id);
                      setPendingModelEffort(option.effort.selectedLevel);
                      modelEffortValueRef.current = option.effort.selectedLevel;
                      return;
                    }
                    option.onSelect?.();
                    setOpenMetaMenuId(null);
                  }}
                >
                  {option.icon ? (
                    <span className="composer-bar-model-option-icon">{option.icon}</span>
                  ) : null}
                  <span className="composer-bar-model-option-label">{option.label}</span>
                  {option.selected ? (
                    <AwakenIcon concept="step-done" size={16} />
                  ) : null}
                </button>
              ))}
            </div>
            )}
            {!activeModelOption && item.menuFooterAction ? (
              <button
                type="button"
                className="composer-bar-model-menu-action"
                onClick={() => {
                  item.menuFooterAction?.onClick();
                  setOpenMetaMenuId(null);
                }}
              >
                {item.menuFooterAction.icon ? (
                  <span className="composer-bar-model-option-icon">
                    {item.menuFooterAction.icon}
                  </span>
                ) : null}
                <span>{item.menuFooterAction.label}</span>
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
    );
  }

  function renderProjectPicker() {
    if (!showProjectPicker || !projectPicker) {
      return null;
    }

    const triggerLabel = selectedProjectOption?.label ?? projectPicker.placeholder ?? "选择项目";
    const pickerLabel = projectPicker.label ?? "选择项目";
    const selectedProjectIcon =
      selectedProjectOption?.icon === null
        ? null
        : (selectedProjectOption?.icon ?? (
            <AwakenIcon concept="project" size={16} />
          ));
    const showClearAction = Boolean(
      projectPicker.clearAction && selectedProjectOption && selectedProjectIcon !== null,
    );

    return (
      <div className="composer-bar-project-shelf" aria-label={pickerLabel}>
        <AwakenDropdown
          open={projectMenuOpen}
          onOpenChange={setProjectMenuOpen}
          placement="bottom-start"
          offset={8}
          trigger={
            <div
              className={`composer-bar-project-trigger-group${
                showClearAction ? " composer-bar-project-trigger-group-clearable" : ""
              }`}
            >
              {showClearAction ? (
                <button
                  type="button"
                  className="composer-bar-project-clear"
                  aria-label={`${projectPicker.clearAction?.label}：${triggerLabel}`}
                  title={projectPicker.clearAction?.label}
                  disabled={projectPicker.disabled || projectPicker.clearAction?.disabled}
                  onClick={(event) => {
                    event.stopPropagation();
                    setProjectMenuOpen(false);
                    projectPicker.clearAction?.onClick();
                  }}
                >
                  <AwakenIcon concept="close" size={12} />
                </button>
              ) : null}
              <button
                type="button"
                className={`composer-bar-project-trigger${
                  selectedProjectIcon === null ? " composer-bar-project-trigger-no-icon" : ""
                }`}
                aria-label={`${pickerLabel}，当前：${triggerLabel}`}
                aria-haspopup="menu"
                aria-expanded={projectMenuOpen}
                disabled={projectPicker.disabled}
              >
                {selectedProjectIcon !== null ? (
                  <span className="composer-bar-project-trigger-icon">{selectedProjectIcon}</span>
                ) : null}
                <span className="composer-bar-project-trigger-label">{triggerLabel}</span>
                <AwakenIcon concept="dropdown" size={16} />
              </button>
            </div>
          }
        >
          <div className="composer-bar-project-menu" role="menu" aria-label={pickerLabel}>
            <div className="composer-bar-project-menu-label">{pickerLabel}</div>
            {projectPicker.options.length > 0 ? (
              <div className="composer-bar-project-options">
                {projectPicker.options.map((option) => {
                  const optionIcon =
                    option.icon === null
                      ? null
                      : (option.icon ?? <AwakenIcon concept="project" size={16} />);

                  return (
                    <button
                      key={option.id}
                      type="button"
                      className={`composer-bar-project-option${
                        option.selected ? " composer-bar-project-option-selected" : ""
                      }${optionIcon === null ? " composer-bar-project-option-no-icon" : ""}`}
                      role="menuitemradio"
                      aria-checked={option.selected ?? false}
                      disabled={option.disabled}
                      onClick={() => {
                        setProjectMenuOpen(false);
                        option.onSelect?.();
                      }}
                    >
                      {optionIcon}
                      <span className="composer-bar-project-option-label">{option.label}</span>
                      {option.selected ? (
                        <AwakenIcon concept="step-done" size={16} />
                      ) : (
                        <span aria-hidden="true" />
                      )}
                    </button>
                  );
                })}
              </div>
            ) : (
              <div className="composer-bar-project-empty">
                {projectPicker.emptyLabel ?? "暂无项目"}
              </div>
            )}
            {projectPicker.createAction ? (
              <button
                type="button"
                className="composer-bar-project-create"
                role="menuitem"
                disabled={projectPicker.createAction.disabled}
                onClick={() => {
                  setProjectMenuOpen(false);
                  projectPicker.createAction?.onClick();
                }}
              >
                <span className="composer-bar-project-create-icon">
                  <AwakenIcon concept="project-add" size={16} />
                </span>
                <span className="composer-bar-project-create-copy">
                  <span className="composer-bar-project-create-label">
                    {projectPicker.createAction.label}
                  </span>
                </span>
              </button>
            ) : null}
          </div>
        </AwakenDropdown>
      </div>
    );
  }

  function renderAttachment(attachment: ComposerBarAttachment) {
    const label = attachment.label ?? attachment.alt;
    const title =
      attachment.tooltip ??
      [label, attachment.mimeType, attachment.detail].filter(Boolean).join("\n");

    return (
      <div
        key={attachment.id}
        className={`composer-bar-attachment composer-bar-attachment-${attachment.kind}`}
        title={title || undefined}
      >
        {attachment.kind === "image" && attachment.src ? (
          <img src={attachment.src} alt={attachment.alt} />
        ) : (
          <div className="composer-bar-file-card" aria-label={label}>
            <AwakenIcon concept="context-file" size={16} />
            <span className="composer-bar-file-card-name">{label}</span>
            {attachment.extension ? (
              <span className="composer-bar-file-card-extension">{attachment.extension}</span>
            ) : null}
          </div>
        )}
        {onRemoveAttachment ? (
          <button
            type="button"
            className="composer-bar-attachment-remove"
            aria-label={`移除 ${label}`}
            onClick={() => onRemoveAttachment(attachment.id)}
          >
            <AwakenIcon concept="close" size={12} />
          </button>
        ) : null}
      </div>
    );
  }

  function renderPluginPicker() {
    if (!showPluginPicker || !pluginPicker) {
      return null;
    }
    const triggerLabel = pluginPicker.label ?? "插件";
    const previewOptions = pluginOptions.slice(0, 3);

    return (
      <AwakenDropdown
        open={pluginPickerOpen}
        onOpenChange={setPluginPickerOpen}
        placement="top-start"
        offset={8}
        trigger={
          <button
            type="button"
            className="composer-bar-plugin-trigger"
            aria-label={
              pluginOptions.length > 0
                ? `${triggerLabel}，${pluginOptions.length} 个可用`
                : `${triggerLabel}，暂无可用插件`
            }
            aria-haspopup="dialog"
            aria-expanded={pluginPickerOpen}
            disabled={disabled || pluginPicker.disabled}
          >
            {previewOptions.length > 0 ? (
              <span className="composer-bar-plugin-preview" aria-hidden="true">
                {previewOptions.map((option) => (
                  <span
                    key={option.id}
                    className="composer-bar-plugin-badge"
                    title={option.label}
                  >
                    <AwakenIcon concept="plugin-default" size={12} />
                  </span>
                ))}
              </span>
            ) : (
              <span className="composer-bar-plugin-preview" aria-hidden="true">
                <span className="composer-bar-plugin-badge">
                  <AwakenIcon concept="plugin-default" size={12} />
                </span>
              </span>
            )}
            <span className="composer-bar-plugin-trigger-label">{triggerLabel}</span>
          </button>
        }
      >
        <div className="composer-bar-plugin-menu" aria-label="选择插件">
          <div className="composer-bar-plugin-options" role="listbox" aria-label="可用插件">
            {pluginPicker.loading && pluginOptions.length === 0 ? (
              <div className="composer-bar-plugin-empty" role="status">
                加载中…
              </div>
            ) : pluginPicker.error ? (
              <div className="composer-bar-plugin-empty composer-bar-plugin-empty-error" role="status">
                {pluginPicker.error}
              </div>
            ) : pluginOptions.length === 0 ? (
              <div className="composer-bar-plugin-empty" role="status">
                {pluginPicker.emptyLabel ?? "还没有安装插件"}
              </div>
            ) : (
              pluginOptions.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={`composer-bar-plugin-option${
                    option.selected ? " composer-bar-plugin-option-selected" : ""
                  }`}
                  role="option"
                  aria-selected={option.selected ?? false}
                  aria-disabled={option.disabled || undefined}
                  disabled={option.disabled}
                  title={option.disabledReason}
                  onClick={() => {
                    setPluginPickerOpen(false);
                    pluginPicker.onSelectOption?.(option.id);
                  }}
                >
                  <span className="composer-bar-plugin-option-badge" aria-hidden="true">
                    <AwakenIcon concept="plugin-default" size={12} />
                  </span>
                  <span className="composer-bar-plugin-option-label">{option.label}</span>
                  {option.selected ? (
                    <AwakenIcon concept="step-done" size={12} />
                  ) : null}
                </button>
              ))
            )}
          </div>
        </div>
      </AwakenDropdown>
    );
  }

  function showSlashPreview(
    action: ComposerBarSlashAction,
    event: ReactMouseEvent<HTMLButtonElement>,
  ): void {
    const hasPreview =
      action.preview?.description ||
      action.description ||
      action.disabledReason ||
      action.preview?.path;
    if (!hasPreview) {
      setSlashPreviewState(null);
      return;
    }

    const rect = event.currentTarget.getBoundingClientRect();
    const paletteRect = slashPaletteRef.current?.getBoundingClientRect();
    setSlashPreviewState({
      actionId: action.id,
      style: slashPreviewStyleForAnchor(
        { left: rect.left, right: rect.right, top: rect.top },
        { width: window.innerWidth, height: window.innerHeight },
        paletteRect ? { left: paletteRect.left, top: paletteRect.top } : undefined,
      ),
    });
  }

  function renderSlashPalette() {
    if (!slashPalette?.open) {
      return null;
    }
    return (
      <div className="composer-bar-slash-palette" ref={slashPaletteRef} style={slashPaletteStyle}>
        <div
          className="composer-bar-slash-list"
          role="listbox"
          aria-label="Slash actions"
          onMouseLeave={() => setSlashPreviewState(null)}
        >
          {slashPalette.loading ? (
            <div className="composer-bar-slash-empty" role="status">
              加载中...
            </div>
          ) : slashPalette.error ? (
            <div className="composer-bar-slash-empty composer-bar-slash-empty-error" role="status">
              {slashPalette.error}
            </div>
          ) : slashActions.length === 0 ? (
            <div className="composer-bar-slash-empty" role="status">
              {slashPalette.emptyLabel ?? "没有可用项"}
            </div>
          ) : (
            slashSections.map((section, sectionIndex) => (
              <div
                key={`${section.label ?? "actions"}:${sectionIndex}`}
                className="composer-bar-slash-section"
                role="group"
                aria-label={section.label}
              >
                {section.label ? (
                  <div className="composer-bar-slash-section-label" aria-hidden="true">
                    {section.label}
                  </div>
                ) : null}
                {section.actions.map((action) => {
                  const active = action.id === slashPalette.activeActionId;
                  return (
                    <button
                      key={action.id}
                      type="button"
                      className={`composer-bar-slash-option${
                        active ? " composer-bar-slash-option-active" : ""
                      }${action.disabled ? " composer-bar-slash-option-disabled" : ""}`}
                      role="option"
                      aria-selected={active}
                      aria-disabled={action.disabled || undefined}
                      onMouseEnter={(event) => {
                        slashPalette.onActiveActionChange?.(action.id);
                        showSlashPreview(action, event);
                      }}
                      onFocus={() => slashPalette.onActiveActionChange?.(action.id)}
                      onClick={() => {
                        if (!action.disabled) {
                          slashPalette.onSelectAction?.(action.id);
                        }
                      }}
                    >
                      <span className="composer-bar-slash-option-main">
                        <span className="composer-bar-slash-option-icon" aria-hidden="true">
                          {action.icon}
                        </span>
                        <span className="composer-bar-slash-option-label">{action.label}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>
        {slashPreviewState && (slashPreviewDescription || slashPreviewPath) ? (
          <div
            className="composer-bar-slash-preview"
            style={slashPreviewState.style}
            aria-hidden="true"
          >
            {slashPreviewDescription ? (
              <span className="composer-bar-slash-preview-description">
                {slashPreviewDescription}
              </span>
            ) : null}
            {slashPreviewPath ? (
              <span className="composer-bar-slash-preview-path">{slashPreviewPath}</span>
            ) : null}
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div
      className={`composer-bar composer-bar-${variant}${
        modelMetaItem ? " composer-bar-has-model" : ""
      }${showProjectPicker ? " composer-bar-has-project-picker" : ""}`}
    >
      {topMetaItems.length > 0 ? (
        <div className="composer-bar-meta" aria-label="Awaken composer context">
          {topMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
      <div className="composer-bar-surface">
        <div className="composer-bar-input-row">
          {attachments.length > 0 ? (
            <div className="composer-bar-attachments" aria-label="附件">
              {attachments.map((attachment) => renderAttachment(attachment))}
            </div>
          ) : null}
          <AwakenFileCommentSummary
            comments={fileComments}
            onRemoveComment={onRemoveFileComment}
            onClear={onClearFileComments}
          />
          <div className="composer-bar-input-wrap" ref={inputWrapRef}>
            {renderSlashPalette()}
            {tokens.length > 0 ? (
              <AwakenComposerInlineInput
                value={value}
                tokens={tokens}
                onChange={onChange}
                onStructuredChange={onStructuredInputChange}
                onSubmit={handleSubmit}
                submitOnEnter={submitOnEnter}
                onKeyDown={onInputKeyDown}
                onRemoveToken={slashPalette?.open ? undefined : onRemoveToken}
                ariaLabel={inputAriaLabel}
                placeholder={placeholder}
                disabled={disabled}
                focusRequest={focusRequest}
                focusOffset={focusOffset}
                onSelectionChange={handleInputSelectionChange}
                minRows={variant === "dock" ? 1 : variant === "embedded" ? 6 : 3}
                maxRows={variant === "dock" ? 8 : variant === "embedded" ? 12 : 6}
                onPasteImages={onPasteImages}
              />
            ) : (
              <AwakenComposerInput
                value={value}
                onChange={onChange}
                onSubmit={handleSubmit}
                submitOnEnter={submitOnEnter}
                onKeyDown={onInputKeyDown}
                ariaLabel={inputAriaLabel}
                placeholder={placeholder}
                disabled={disabled}
                focusRequest={focusRequest}
                focusOffset={focusOffset}
                onSelectionChange={handleInputSelectionChange}
                minRows={variant === "dock" ? 1 : variant === "embedded" ? 6 : 3}
                maxRows={variant === "dock" ? 8 : variant === "embedded" ? 12 : 6}
                onPasteImages={onPasteImages}
              />
            )}
          </div>
          {attachmentWarning ? (
            <div className="composer-bar-attachment-warning" role="status">
              {attachmentWarning}
            </div>
          ) : null}
        </div>
        {variant !== "embedded" ? (
          <div className="composer-bar-toolbar">
            <div className="composer-bar-context">
              <AwakenContextButton onClick={onAddContext} disabled={disabled} />
              {renderPluginPicker()}
            </div>
            {modelMetaItem ? renderModelMetaItem(modelMetaItem) : null}
            <div className="composer-bar-submit">
              <AwakenSendButton
                ready={isReady || submitMode === "stop"}
                onClick={handleSubmit}
                disabled={submitMode === "stop" ? false : submitDisabled}
                mode={submitMode}
              />
            </div>
          </div>
        ) : null}
      </div>
      {renderProjectPicker()}
      {bottomMetaItems.length > 0 ? (
        <div
          className="composer-bar-meta composer-bar-meta-bottom"
          aria-label="Awaken composer context"
        >
          {bottomMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
    </div>
  );
}
