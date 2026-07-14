import "./AwakenComposerBar.css";
import { useCallback, useEffect, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  ReactNode,
} from "react";
import { Check, ChevronDown, FileText, X } from "lucide-react";
import { AwakenComposerInput } from "../AwakenComposerInput";
import { AwakenContextButton } from "../AwakenContextButton";
import { AwakenSendButton } from "../AwakenSendButton";
import {
  AwakenComposerInlineInput,
  type ComposerInlineInputElement,
  type ComposerInlineStructuredValue,
} from "./AwakenComposerInlineInput";

export type ComposerBarMetaMenuItem = {
  id: string;
  label: string;
  icon?: ReactNode;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
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
  menuItems?: ComposerBarMetaMenuItem[];
  menuFooterAction?: ComposerBarMetaAction;
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

export type ComposerBarSlashAction = {
  id: string;
  type: string;
  label: string;
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

type SlashPreviewState = {
  actionId: string;
  style: CSSProperties;
};

type SlashPaletteStyle = CSSProperties & {
  "--composer-bar-slash-max-height"?: string;
};

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
  left = Math.min(
    Math.max(margin, left),
    Math.max(margin, viewport.width - width - margin),
  );

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
  const maxHeight = Math.max(
    88,
    Math.min(210, Math.floor(anchorTop - margin - gap)),
  );

  return { "--composer-bar-slash-max-height": `${maxHeight}px` };
}

export type AwakenComposerBarProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onInputKeyDown?: (
    event: ReactKeyboardEvent<HTMLTextAreaElement | ComposerInlineInputElement>,
  ) => void;
  onStructuredInputChange?: (value: ComposerBarStructuredInputValue) => void;
  onAddContext?: () => void;
  placeholder?: string;
  disabled?: boolean;
  isReady?: boolean;
  submitMode?: "send" | "stop";
  variant?: "center" | "dock";
  metaItems?: ComposerBarMetaItem[];
  focusRequest?: number;
  submitDisabled?: boolean;
  attachments?: ComposerBarAttachment[];
  attachmentWarning?: ReactNode;
  tokens?: ComposerBarToken[];
  slashPalette?: ComposerBarSlashPalette;
  onRemoveAttachment?: (attachmentId: string) => void;
  onRemoveToken?: (tokenId: string) => void;
  onPasteImages?: (files: File[]) => void;
};

export function AwakenComposerBar({
  value,
  onChange,
  onSubmit,
  onInputKeyDown,
  onStructuredInputChange,
  onAddContext,
  placeholder,
  disabled,
  isReady = false,
  submitMode = "send",
  variant = "dock",
  metaItems = [],
  focusRequest = 0,
  submitDisabled = false,
  attachments = [],
  attachmentWarning,
  tokens = [],
  slashPalette,
  onRemoveAttachment,
  onRemoveToken,
  onPasteImages,
}: AwakenComposerBarProps) {
  const canSubmit = submitMode === "stop" || (!disabled && !submitDisabled);
  const [openMetaMenuId, setOpenMetaMenuId] = useState<string | null>(null);
  const [slashPreviewState, setSlashPreviewState] =
    useState<SlashPreviewState | null>(null);
  const [slashPaletteStyle, setSlashPaletteStyle] =
    useState<SlashPaletteStyle>({});
  const modelMetaRef = useRef<HTMLDivElement | null>(null);
  const inputWrapRef = useRef<HTMLDivElement | null>(null);
  const slashPaletteRef = useRef<HTMLDivElement | null>(null);
  const modelMetaItem = metaItems.find((item) => item.id === "model");
  const topMetaItems: ComposerBarMetaItem[] = [];
  const bottomMetaItems =
    variant === "dock" ? metaItems.filter((item) => item.id !== "model") : [];
  const slashActions = slashPalette?.actions ?? [];
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

  useEffect(() => {
    if (!openMetaMenuId) return;

    function handlePointerDown(event: MouseEvent): void {
      if (!modelMetaRef.current?.contains(event.target as Node)) {
        setOpenMetaMenuId(null);
      }
    }

    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        setOpenMetaMenuId(null);
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
    if (!slashPalette?.open) {
      setSlashPreviewState(null);
      setSlashPaletteStyle({});
    }
  }, [slashPalette?.open]);

  useEffect(() => {
    if (!slashPalette?.open) {
      return;
    }

    function updateSlashPaletteStyle(): void {
      const rect = inputWrapRef.current?.getBoundingClientRect();
      if (!rect) {
        return;
      }
      setSlashPaletteStyle(slashPaletteStyleForAnchor(rect.top, window.innerHeight));
    }

    updateSlashPaletteStyle();
    window.addEventListener("resize", updateSlashPaletteStyle);
    return () => window.removeEventListener("resize", updateSlashPaletteStyle);
  }, [slashPalette?.open, slashActions.length, value, tokens.length]);

  function renderMetaItem(item: ComposerBarMetaItem) {
    const hasProgress =
      typeof item.progress === "number" && Number.isFinite(item.progress);
    const progressPercent = hasProgress
      ? Math.max(0, Math.min(100, Math.round((item.progress ?? 0) * 100)))
      : undefined;

    const content = (
      <>
        {item.icon ? (
          <span className="composer-bar-meta-icon">{item.icon}</span>
        ) : null}
        <span className="composer-bar-meta-label">{item.label}</span>
        {hasProgress ? (
          <span
            className="composer-bar-context-meter"
            role="meter"
            aria-label={item.label}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={progressPercent}
            aria-valuetext={item.value}
          >
            <span
              className="composer-bar-context-meter-fill"
              style={
                {
                  "--composer-bar-context-progress": `${progressPercent}%`,
                } as CSSProperties
              }
            />
          </span>
        ) : null}
        {item.value ? (
          <span className="composer-bar-meta-value">{item.value}</span>
        ) : null}
      </>
    );
    const className = `composer-bar-meta-item${
      hasProgress ? " composer-bar-meta-item-meter" : ""
    }${item.onClick ? " composer-bar-meta-item-button" : ""}`;

    if (item.onClick) {
      return (
        <button
          key={item.id}
          type="button"
          className={className}
          onClick={item.onClick}
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
    const triggerContent = (
      <>
        {item.icon ? (
          <span className="composer-bar-model-trigger-icon">{item.icon}</span>
        ) : null}
        <span className="composer-bar-model-trigger-label">{item.label}</span>
        {hasMenu ? (
          <ChevronDown
            className="composer-bar-model-trigger-chevron"
            size={14}
            strokeWidth={1.85}
            aria-hidden="true"
          />
        ) : null}
      </>
    );

    return (
      <div className="composer-bar-toolbar-model" ref={modelMetaRef}>
        {isInteractive ? (
          <button
            type="button"
            className={`composer-bar-model-trigger${
              isMenuOpen ? " composer-bar-model-trigger-open" : ""
            }`}
            aria-haspopup={hasMenu ? "listbox" : undefined}
            aria-expanded={hasMenu ? isMenuOpen : undefined}
            onClick={() => {
              if (hasMenu) {
                setOpenMetaMenuId((current) =>
                  current === item.id ? null : item.id,
                );
                return;
              }
              item.onClick?.();
            }}
          >
            {triggerContent}
          </button>
        ) : (
          <span className="composer-bar-model-trigger composer-bar-model-trigger-disabled">
            {triggerContent}
          </span>
        )}
        {hasMenu && isMenuOpen ? (
          <div
            className="composer-bar-model-menu"
            role="listbox"
            aria-label="选择模型"
          >
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
                    option.onSelect?.();
                    setOpenMetaMenuId(null);
                  }}
                >
                  {option.icon ? (
                    <span className="composer-bar-model-option-icon">
                      {option.icon}
                    </span>
                  ) : null}
                  <span className="composer-bar-model-option-label">
                    {option.label}
                  </span>
                  {option.selected ? (
                    <Check
                      className="composer-bar-model-option-check"
                      size={15}
                      strokeWidth={2}
                      aria-hidden="true"
                    />
                  ) : null}
                </button>
              ))}
            </div>
            {item.menuFooterAction ? (
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

  function renderAttachment(attachment: ComposerBarAttachment) {
    const label = attachment.label ?? attachment.alt;
    const title = attachment.tooltip ?? [label, attachment.mimeType, attachment.detail]
      .filter(Boolean)
      .join("\n");

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
            <FileText
              className="composer-bar-file-card-icon"
              size={17}
              strokeWidth={1.85}
              aria-hidden="true"
            />
            <span className="composer-bar-file-card-name">{label}</span>
            {attachment.extension ? (
              <span className="composer-bar-file-card-extension">
                {attachment.extension}
              </span>
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
            <X size={12} strokeWidth={2.2} />
          </button>
        ) : null}
      </div>
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
        paletteRect
          ? { left: paletteRect.left, top: paletteRect.top }
          : undefined,
      ),
    });
  }

  function renderSlashPalette() {
    if (!slashPalette?.open) {
      return null;
    }
    return (
      <div
        className="composer-bar-slash-palette"
        ref={slashPaletteRef}
        style={slashPaletteStyle}
      >
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
            <div
              className="composer-bar-slash-empty composer-bar-slash-empty-error"
              role="status"
            >
              {slashPalette.error}
            </div>
          ) : slashActions.length === 0 ? (
            <div className="composer-bar-slash-empty" role="status">
              {slashPalette.emptyLabel ?? "没有可用项"}
            </div>
          ) : (
            slashActions.map((action) => {
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
                    <span
                      className="composer-bar-slash-option-icon"
                      aria-hidden="true"
                    >
                      {action.icon}
                    </span>
                    <span className="composer-bar-slash-option-label">
                      {action.label}
                    </span>
                  </span>
                </button>
              );
            })
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
              <span className="composer-bar-slash-preview-path">
                {slashPreviewPath}
              </span>
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
      }`}
    >
      {topMetaItems.length > 0 ? (
        <div className="composer-bar-meta" aria-label="AwakenComposer context">
          {topMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
      <div className="composer-bar-surface">
        <div className="composer-bar-input-row">
          {attachments.length > 0 ? (
            <div
              className="composer-bar-attachments"
              aria-label="附件"
            >
              {attachments.map((attachment) => renderAttachment(attachment))}
            </div>
          ) : null}
          <div
            className="composer-bar-input-wrap"
            ref={inputWrapRef}
          >
            {renderSlashPalette()}
            {tokens.length > 0 ? (
              <AwakenComposerInlineInput
                value={value}
                tokens={tokens}
                onChange={onChange}
                onStructuredChange={onStructuredInputChange}
                onSubmit={handleSubmit}
                onKeyDown={onInputKeyDown}
                onRemoveToken={slashPalette?.open ? undefined : onRemoveToken}
                placeholder={placeholder}
                disabled={disabled}
                focusRequest={focusRequest}
                minRows={variant === "dock" ? 1 : 3}
                maxRows={variant === "dock" ? 8 : 6}
                onPasteImages={onPasteImages}
              />
            ) : (
              <AwakenComposerInput
                value={value}
                onChange={onChange}
                onSubmit={handleSubmit}
                onKeyDown={onInputKeyDown}
                placeholder={placeholder}
                disabled={disabled}
                focusRequest={focusRequest}
                minRows={variant === "dock" ? 1 : 3}
                maxRows={variant === "dock" ? 8 : 6}
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
        <div className="composer-bar-toolbar">
          <div className="composer-bar-context">
            <AwakenContextButton onClick={onAddContext} disabled={disabled} />
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
      </div>
      {bottomMetaItems.length > 0 ? (
        <div
          className="composer-bar-meta composer-bar-meta-bottom"
          aria-label="AwakenComposer context"
        >
          {bottomMetaItems.map((item) => renderMetaItem(item))}
        </div>
      ) : null}
    </div>
  );
}
