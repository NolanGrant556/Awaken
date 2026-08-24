import "./AwakenPluginCard.css";
import { AwakenIcon } from "../AwakenIcon";
import type { HTMLAttributes } from "react";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenPluginCardItem = {
  name: string;
  version?: string;
  description?: string;
  skillCount: number;
  serverCount: number;
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
  const summary = `${plugin.skillCount} skills / ${plugin.serverCount} servers`;
  const body = (
    <>
      <span className="aw-plugin-card__icon" aria-hidden="true">
        <AwakenIcon concept="plugin" size={20} />
      </span>
      <span className="aw-plugin-card__copy">
        <span className="aw-plugin-card__title-row">
          <span className="aw-plugin-card__name">{plugin.name}</span>
          {plugin.version ? <span className="aw-plugin-card__version">{plugin.version}</span> : null}
        </span>
        {plugin.diagnostic ? (
          <span className="aw-plugin-card__diagnostic">{plugin.diagnostic}</span>
        ) : (
          <span className="aw-plugin-card__description">{plugin.description || summary}</span>
        )}
        <span className="aw-plugin-card__meta">{summary}</span>
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
          aria-label={`查看插件 ${plugin.name}`}
          onClick={onOpen}
        >
          {body}
        </button>
      ) : (
        <div className="aw-plugin-card__main aw-plugin-card__main--static">{body}</div>
      )}
      <div className="aw-plugin-card__actions">
        <AwakenSwitch
          checked={plugin.enabled}
          label={plugin.name}
          busy={plugin.busy}
          size="compact"
          disabled={!onEnabledChange}
          onCheckedChange={onEnabledChange}
        />
      </div>
    </article>
  );
}
