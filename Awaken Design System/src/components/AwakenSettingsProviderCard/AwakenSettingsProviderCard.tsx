import "./AwakenSettingsProviderCard.css";
import { AwakenIcon } from "../AwakenIcon";
import { useEffect, useRef, type ReactNode } from "react";
import {
  AwakenProviderLogo,
  type AwakenProviderBrand,
} from "../AwakenProviderLogo";

export type AwakenProviderConnectionStatus =
  | "connected"
  | "attention"
  | "unconfigured";

const CONNECTION_STATUS_LABELS: Record<AwakenProviderConnectionStatus, string> = {
  connected: "已连接",
  attention: "需要处理",
  unconfigured: "尚未配置",
};

export type AwakenSettingsProviderCardProps = {
  name: string;
  brand?: AwakenProviderBrand;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
};

export function AwakenSettingsProviderCard({
  name,
  brand,
  selected = false,
  disabled = false,
  onSelect,
}: AwakenSettingsProviderCardProps) {
  return (
    <button
      type="button"
      className="model-provider-card"
      data-provider-brand={brand}
      aria-pressed={selected}
      disabled={disabled}
      onClick={onSelect}
    >
      <span className="model-provider-card__logo">
        {brand ? (
          <AwakenProviderLogo brand={brand} name={name} />
        ) : (
          <span className="model-provider-logo-fallback">{name.trim().slice(0, 1) || "M"}</span>
        )}
      </span>
      <span className="model-provider-card__copy">
        <strong>{name}</strong>
      </span>
      <span className="model-provider-card__action">
        配置
        <AwakenIcon concept="provider-link" size={16} />
      </span>
    </button>
  );
}

export type AwakenSettingsProviderModalProps = {
  open: boolean;
  name: string;
  brand?: AwakenProviderBrand;
  status?: AwakenProviderConnectionStatus;
  caption?: ReactNode;
  children: ReactNode;
  onClose: () => void;
};

export function AwakenSettingsProviderModal({
  open,
  name,
  brand,
  status = "unconfigured",
  caption,
  children,
  onClose,
}: AwakenSettingsProviderModalProps) {
  const modalRef = useRef<HTMLElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    restoreFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    const frame = window.requestAnimationFrame(() => modalRef.current?.focus());
    return () => {
      window.cancelAnimationFrame(frame);
      restoreFocusRef.current?.focus();
      restoreFocusRef.current = null;
    };
  }, [open]);

  if (!open) return null;

  return (
    <div
      className="model-provider-modal-layer"
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }}
    >
      <button
        type="button"
        className="model-provider-modal-backdrop"
        aria-label="关闭供应商配置"
        onClick={onClose}
      />
      <section
        ref={modalRef}
        className="model-provider-modal"
        data-provider-brand={brand}
        role="dialog"
        aria-modal="true"
        aria-label={`配置 ${name}`}
        tabIndex={-1}
      >
        <header className="model-provider-modal__header">
          <span className="model-provider-modal__wash" aria-hidden="true" />
          <span className="model-provider-modal__logo">
            {brand ? (
              <AwakenProviderLogo brand={brand} name={name} />
            ) : (
              <span className="model-provider-logo-fallback">{name.trim().slice(0, 1) || "M"}</span>
            )}
          </span>
          <span className="model-provider-modal__heading">
            <strong>{name}</strong>
            <span className={`model-provider-modal__status model-provider-modal__status--${status}`}>
              <span aria-hidden="true" />
              {CONNECTION_STATUS_LABELS[status]}
            </span>
          </span>
          <button
            type="button"
            className="model-provider-modal__close"
            aria-label="关闭配置面板"
            onClick={onClose}
          >
            <AwakenIcon concept="close" size={20} />
          </button>
          {caption ? <div className="model-provider-modal__caption">{caption}</div> : null}
        </header>
        <div className="model-provider-modal__body model-provider-form">
          {children}
        </div>
      </section>
    </div>
  );
}
