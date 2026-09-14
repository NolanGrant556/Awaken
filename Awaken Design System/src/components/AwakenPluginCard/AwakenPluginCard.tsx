import "./AwakenPluginCard.css";
import { AwakenIcon } from "../AwakenIcon";
import {
  AwakenPluginBrandIcon,
  isAwakenPluginBrand,
} from "../AwakenPluginBrandIcon";
import type { HTMLAttributes } from "react";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenPluginCardItem = {
  name: string;
  displayName?: string;
  description?: string;
  dialect?: string;
  builtin?: boolean;
  brandIcon?: string;
  brandLogoSrc?: string;
  /** Host-managed integrations expose connection settings instead of a package switch. */
  connectionStatus?: { label: string; connected: boolean };
  builtinOrder?: number;
  enabled: boolean;
  busy?: boolean;
  diagnostic?: string;
};

export type AwakenPluginCardProps = Omit<
  HTMLAttributes<HTMLElement>,
  "children" | "onClick"
> & {
  plugin: AwakenPluginCardItem;
  onOpen?: () => void;
  onEnabledChange?: (enabled: boolean) => void;
};

export function AwakenPluginCard({
  plugin,
  onOpen,
  onEnabledChange,
  className,
  ...props
}: AwakenPluginCardProps) {
  const visibleName = plugin.displayName?.trim() || plugin.name;
  const dialectLabels: Record<string, string> = {
    agent_plugins: "Agent Plugins",
    codex: "Codex",
    claude_code: "Claude Code",
    cursor: "Cursor",
  };
  const body = (
    <>
      <span className="aw-plugin-card__icon" aria-hidden="true">
        {plugin.brandLogoSrc ? (
          <img className="aw-plugin-card__brand-image" src={plugin.brandLogoSrc} alt="" />
        ) : isAwakenPluginBrand(plugin.brandIcon) ? (
          <AwakenPluginBrandIcon brand={plugin.brandIcon} size="card" />
        ) : (
          <AwakenIcon concept="plugin-default" size={20} />
        )}
      </span>
      <span className="aw-plugin-card__copy">
        <span className="aw-plugin-card__title-row">
          <span className="aw-plugin-card__name">{visibleName}</span>
          {plugin.builtin ? (
            <span className="aw-plugin-card__badge">Awaken 内置</span>
          ) : plugin.dialect ? (
            <span className="aw-plugin-card__badge">{dialectLabels[plugin.dialect] ?? plugin.dialect}</span>
          ) : null}
        </span>
        {plugin.diagnostic ? (
          <span className="aw-plugin-card__diagnostic">{plugin.diagnostic}</span>
        ) : plugin.description ? (
          <span className="aw-plugin-card__description">{plugin.description}</span>
        ) : null}
      </span>
    </>
  );

  return (
    <article
      {...props}
      className={["aw-plugin-card", className ?? ""].filter(Boolean).join(" ")}
      data-enabled={plugin.enabled ? "true" : "false"}
    >
      {onOpen ? (
        <button
          type="button"
          className="aw-plugin-card__main"
          aria-label={`查看插件 ${visibleName}`}
          onClick={onOpen}
        >
          {body}
        </button>
      ) : (
        <div className="aw-plugin-card__main aw-plugin-card__main--static">{body}</div>
      )}
      <div className="aw-plugin-card__actions" data-connection={plugin.connectionStatus ? "true" : undefined}>
        {plugin.connectionStatus ? (
          <button
            type="button"
            className="aw-plugin-card__connection"
            data-connected={plugin.connectionStatus.connected ? "true" : "false"}
            aria-label={`打开 ${visibleName} 连接设置`}
            onClick={onOpen}
            disabled={!onOpen}
          >
            {plugin.connectionStatus.connected ? <AwakenIcon concept="step-done" size={14} /> : null}
            {plugin.connectionStatus.label}
          </button>
        ) : <AwakenSwitch
          checked={plugin.enabled}
          label={visibleName}
          busy={plugin.busy}
          size="compact"
          disabled={!onEnabledChange}
          onCheckedChange={onEnabledChange}
        />}
      </div>
    </article>
  );
}
