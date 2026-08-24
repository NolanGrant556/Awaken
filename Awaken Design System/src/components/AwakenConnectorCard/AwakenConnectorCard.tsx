import "./AwakenConnectorCard.css";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenConnectorBrandIcon } from "../AwakenConnectorBrandIcon";
import { AwakenSwitch } from "../AwakenSwitch";
import type { AwakenConnectorCardItem } from "../AwakenConnectors/AwakenConnectors.types";

export type AwakenConnectorCardProps = {
  item: AwakenConnectorCardItem;
  onPrimaryAction?: (item: AwakenConnectorCardItem) => void;
  onEnabledChange?: (item: AwakenConnectorCardItem, enabled: boolean) => void;
  onSettings?: (item: AwakenConnectorCardItem) => void;
};

function statusTone(state: AwakenConnectorCardItem["state"]): string {
  if (state === "connected" || state === "loaded") return "success";
  if (state === "failed" || state === "invalid") return "error";
  if (state === "pending" || state === "needs-auth" || state === "shadowed" || state === "disabled_by_policy") {
    return "warning";
  }
  return "neutral";
}

function hasDistinctDescription(item: AwakenConnectorCardItem): boolean {
  const description = item.description.trim();
  return Boolean(description) && description.localeCompare(item.displayName.trim(), undefined, {
    sensitivity: "accent",
  }) !== 0;
}

export function AwakenConnectorCard({
  item,
  onPrimaryAction,
  onEnabledChange,
  onSettings,
}: AwakenConnectorCardProps) {
  const showDescription = hasDistinctDescription(item);
  const hasMcpServerControls = item.variant === "server" || Boolean(item.mcpServerName);
  if (item.variant === "cli") {
    const actionLabel = `打开 ${item.displayName} 连接设置`;
    return (
      <article className="aw-connector-card aw-connector-card--cli" data-ready={item.connected ? "true" : "false"}>
        <button
          type="button"
          className="aw-connector-card__main"
          disabled={item.busy}
          onClick={() => onPrimaryAction?.(item)}
        >
          <AwakenConnectorBrandIcon
            providerId={item.id}
            imageSrc={item.brandLogoSrc}
            label={item.displayName}
            tone={item.brandTone}
            size="sm"
          />
          <span className="aw-connector-card__copy">
            <strong className="aw-connector-card__title">{item.displayName}</strong>
            {showDescription ? (
              <span className="aw-connector-card__description">{item.description}</span>
            ) : null}
            {item.errors?.map((error) => (
              <span className="aw-connector-card__error" key={error}>{error}</span>
            ))}
          </span>
        </button>
        <button
          type="button"
          className="aw-connector-card__action"
          data-ready={item.connected ? "true" : "false"}
          aria-label={actionLabel}
          disabled={item.busy}
          onClick={() => onPrimaryAction?.(item)}
        >
          {item.connected ? <AwakenIcon concept="step-done" size={16} /> : <AwakenIcon concept="add" size={16} />}
        </button>
      </article>
    );
  }

  return (
    <article className="aw-connector-card" data-variant={item.variant}>
      <span className="aw-connector-card__icon" aria-hidden="true">
        <AwakenIcon concept="mcp-brand" size={20} />
      </span>
      <span className="aw-connector-card__copy">
        <span className="aw-connector-card__title-row">
          <strong className="aw-connector-card__title">{item.displayName}</strong>
          {item.stateLabel ? (
            <span className="aw-connector-card__status" data-tone={statusTone(item.state)}>{item.stateLabel}</span>
          ) : null}
        </span>
        {showDescription ? (
          <span className="aw-connector-card__description">{item.description}</span>
        ) : null}
        {item.tags?.length ? (
          <span className="aw-connector-card__tags">
            {item.tags.map((tag) => <span key={tag}>{tag}</span>)}
          </span>
        ) : null}
      </span>
      {hasMcpServerControls ? (
        <span className="aw-connector-card__actions">
          <AwakenSwitch
            checked={item.enabled ?? false}
            label={item.displayName}
            busy={item.busy}
            onCheckedChange={(enabled) => onEnabledChange?.(item, enabled)}
          />
          <button
            type="button"
            className="aw-connector-card__action"
            aria-label={`设置 ${item.displayName}`}
            disabled={item.busy}
            onClick={() => onSettings?.(item)}
          >
            <AwakenIcon concept="settings" size={16} />
          </button>
        </span>
      ) : (
        <button
          type="button"
          className="aw-connector-card__action"
          aria-label={item.connected ? `断开 ${item.displayName}` : `连接 ${item.displayName}`}
          disabled={item.busy}
          onClick={() => onPrimaryAction?.(item)}
        >
          {item.connected ? <AwakenIcon concept="close" size={16} /> : <AwakenIcon concept="add" size={16} />}
        </button>
      )}
    </article>
  );
}
