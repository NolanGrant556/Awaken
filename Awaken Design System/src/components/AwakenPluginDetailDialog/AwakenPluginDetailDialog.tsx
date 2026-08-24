import "./AwakenPluginDetailDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenPluginDetailSkill = {
  id: string;
  name: string;
  description?: string;
  previewable?: boolean;
};

export type AwakenPluginDetailServer = {
  shortName: string;
  identity: string;
  state?: string;
  enabled: boolean;
  errorReason?: string;
  invalid?: string;
};

export type AwakenPluginDetailDialogProps = {
  open: boolean;
  name: string;
  version?: string;
  description?: string;
  sourceLabel?: string;
  digest?: string;
  skills: AwakenPluginDetailSkill[];
  servers: AwakenPluginDetailServer[];
  diagnostics?: string[];
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onPreviewSkill?: (skillId: string) => void;
  onServerEnabledChange?: (shortName: string, enabled: boolean) => void;
  onUpgrade?: () => void;
  onUninstall?: () => void;
};

export function AwakenPluginDetailDialog({
  open,
  name,
  version,
  description,
  sourceLabel,
  digest,
  skills,
  servers,
  diagnostics = [],
  busy = false,
  onOpenChange,
  onPreviewSkill,
  onServerEnabledChange,
  onUpgrade,
  onUninstall,
}: AwakenPluginDetailDialogProps) {
  return (
    <AwakenDialog
      open={open}
      size="large"
      className="aw-plugin-detail-dialog"
      title={name}
      onOpenChange={onOpenChange}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={busy} onClick={onUninstall}>
            卸载
          </AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} onClick={onUpgrade}>
            升级
          </AwakenButton>
        </>
      }
    >
      <div className="aw-plugin-detail-dialog__body">
        {description ? <p>{description}</p> : null}
        <p className="aw-plugin-detail-dialog__meta">
          {version ? `版本 ${version}` : "未声明版本"}
          {sourceLabel ? ` · ${sourceLabel}` : ""}
        </p>
        {digest ? <p className="aw-plugin-detail-dialog__digest">{digest}</p> : null}
        {diagnostics.length > 0 ? (
          <ul className="aw-plugin-detail-dialog__diagnostics">
            {diagnostics.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        ) : null}
        <section>
          <h3>Skills</h3>
          {skills.length === 0 ? <p>此插件没有可发现的 Skill。</p> : null}
          <ul>
            {skills.map((skill) => (
              <li key={skill.id}>
                <button
                  type="button"
                  disabled={!skill.previewable || !onPreviewSkill}
                  onClick={() => onPreviewSkill?.(skill.id)}
                >
                  {skill.name}
                </button>
                {skill.description ? <span>{skill.description}</span> : null}
              </li>
            ))}
          </ul>
        </section>
        <section>
          <h3>MCP servers</h3>
          {servers.length === 0 ? <p>此插件没有 MCP server。</p> : null}
          <ul>
            {servers.map((server) => (
              <li key={server.identity}>
                <span>
                  {server.shortName}
                  {server.state ? ` · ${server.state}` : ""}
                  {server.invalid ? ` · ${server.invalid}` : ""}
                  {server.errorReason ? ` · ${server.errorReason}` : ""}
                </span>
                <AwakenSwitch
                  checked={server.enabled}
                  label={server.shortName}
                  size="compact"
                  disabled={busy || Boolean(server.invalid) || !onServerEnabledChange}
                  onCheckedChange={(enabled) => onServerEnabledChange?.(server.shortName, enabled)}
                />
              </li>
            ))}
          </ul>
        </section>
        <p className="aw-plugin-detail-dialog__risk">
          卸载会把插件包和数据目录移入回收站。MCP 子进程以当前用户身份运行，不受沙盒约束。
        </p>
      </div>
    </AwakenDialog>
  );
}
