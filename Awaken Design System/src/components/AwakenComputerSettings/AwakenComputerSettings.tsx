import { AwakenButton } from "../AwakenButton";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenSettingsSwitch } from "../AwakenSettingsSwitch";
import "./AwakenComputerSettings.css";

export type AwakenComputerHostStatus = {
  available: boolean;
  healthy: boolean;
  version?: string;
  protocol_version?: number;
  os?: string;
  tcc_screen_recording: boolean;
  tcc_accessibility: boolean;
  node_path?: string;
  host_script_path?: string;
  message?: string;
};

export type AwakenComputerSettingsValue = {
  enabled: boolean;
  allow_background_window_actions: boolean;
};

export type AwakenComputerSettingsProps = {
  status: AwakenComputerHostStatus | null;
  settings: AwakenComputerSettingsValue | null;
  loading?: boolean;
  settingsLoading?: boolean;
  settingsBusy?: boolean;
  settingsError?: string | null;
  settingsMessageTone?: "info" | "error";
  onRefresh?: () => void;
  onSettingsChange?: (settings: AwakenComputerSettingsValue) => void;
  onSetupRuntime?: () => void;
  onRequestPermissions?: () => void;
  onOpenPermissionSettings?: (
    permission: "screen_recording" | "accessibility",
  ) => void;
};

function StatusItem({
  label,
  ok,
  detail,
  actionLabel,
  onAction,
  actionDisabled,
}: {
  label: string;
  ok: boolean;
  detail?: string;
  actionLabel?: string;
  onAction?: () => void;
  actionDisabled?: boolean;
}) {
  return (
    <div className="computer-settings-status-item" data-ok={ok}>
      <div className="computer-settings-status-icon" aria-hidden="true">
        {ok ? (
          <AwakenIcon concept="success" size={20} />
        ) : (
          <AwakenIcon concept="failure" size={20} />
        )}
      </div>
      <div className="computer-settings-status-copy">
        <span className="computer-settings-status-label">{label}</span>
        {detail ? (
          <span className="computer-settings-status-detail">{detail}</span>
        ) : null}
      </div>
      {actionLabel && onAction ? (
        <AwakenButton
          size="sm"
          variant="ghost"
          disabled={actionDisabled}
          onClick={onAction}
        >
          <AwakenIcon concept="external-open" size={14} />
          {actionLabel}
        </AwakenButton>
      ) : null}
    </div>
  );
}

export function AwakenComputerSettings({
  status,
  settings,
  loading,
  settingsLoading,
  settingsBusy,
  settingsError,
  settingsMessageTone = "info",
  onRefresh,
  onSettingsChange,
  onSetupRuntime,
  onRequestPermissions,
  onOpenPermissionSettings,
}: AwakenComputerSettingsProps) {
  const hostReady = status?.available === true && status?.healthy === true;
  const screenRecordingReady = status?.tcc_screen_recording === true;
  const accessibilityReady = status?.tcc_accessibility === true;

  function patch(partial: Partial<AwakenComputerSettingsValue>) {
    if (!settings) return;
    const next = { ...settings, ...partial };
    onSettingsChange?.(next);
  }

  return (
    <section className="computer-settings" aria-label="计算机操作">
      <div className="computer-settings-header">
        <div className="computer-settings-intro">
          <h2>计算机操作</h2>
          <p>
            允许 Agent 在缺少可靠工具时观察并操作本机 GUI。窗口与桌面范围默认可用，
            有外部后果的动作仍按运行时风险策略复核。
          </p>
        </div>
        <AwakenButton
          size="sm"
          variant="ghost"
          disabled={loading}
          onClick={() => onRefresh?.()}
        >
          <AwakenIcon concept="refresh" size={16} />
          {loading ? "检测中…" : "刷新状态"}
        </AwakenButton>
      </div>

      <div className="computer-settings-card">
        <div className="computer-settings-card-header">
          <AwakenIcon concept="computer-use" size={20} />
          <h3>运行状态</h3>
        </div>
        <div className="computer-settings-status-list">
          <StatusItem
            label="CUA Host 侧链"
            ok={hostReady}
            detail={
              status === null
                ? "未检测"
                : hostReady
                  ? `版本 ${status.version ?? "未知"}`
                  : status.message ?? "不可用"
            }
          />
          <StatusItem
            label="Node runtime"
            ok={status?.node_path !== undefined && status.node_path !== ""}
            detail={status?.node_path}
          />
          <StatusItem
            label="屏幕录制权限"
            ok={screenRecordingReady}
            detail={
              screenRecordingReady
                ? "已授权"
                : "需要在系统设置中开启屏幕录制权限"
            }
            actionLabel={screenRecordingReady ? undefined : "去授权"}
            actionDisabled={settingsBusy}
            onAction={
              screenRecordingReady
                ? undefined
                : () => onOpenPermissionSettings?.("screen_recording")
            }
          />
          <StatusItem
            label="辅助功能权限"
            ok={accessibilityReady}
            detail={
              accessibilityReady
                ? "已授权"
                : "需要在系统设置中开启辅助功能权限"
            }
            actionLabel={accessibilityReady ? undefined : "去授权"}
            actionDisabled={settingsBusy}
            onAction={
              accessibilityReady
                ? undefined
                : () => onOpenPermissionSettings?.("accessibility")
            }
          />
        </div>
        {settings?.enabled &&
        (!hostReady || !screenRecordingReady || !accessibilityReady) ? (
          <div className="computer-settings-status-actions">
            <AwakenButton
              size="sm"
              loading={settingsBusy}
              disabled={settingsBusy}
              onClick={() => {
                if (!hostReady) {
                  onSetupRuntime?.();
                } else {
                  onRequestPermissions?.();
                }
              }}
            >
              {settingsBusy
                ? "配置中…"
                : !hostReady
                  ? "准备 CUA 运行时"
                  : "继续授权"}
            </AwakenButton>
          </div>
        ) : null}
      </div>

      <div className="computer-settings-card">
        <div className="computer-settings-card-header">
          <h3>全局设置</h3>
        </div>
        {settingsLoading || settings === null ? (
          <div className="computer-settings-empty">正在读取设置…</div>
        ) : (
          <div className="computer-settings-form">
            <div className="computer-settings-enable-group">
              <AwakenSettingsSwitch
                label="启用计算机操作"
                description="关闭后 Agent 将无法创建 Computer 绑定，已有 run 不受影响。"
                checked={settings.enabled}
                disabled={settingsBusy}
                busy={settingsBusy}
                onCheckedChange={(checked) => patch({ enabled: checked })}
              />
              {settingsBusy ? (
                <div className="computer-settings-setup" role="status">
                  <span className="computer-settings-setup-spinner" aria-hidden="true" />
                  {settingsError ?? "正在配置计算机操作环境…"}
                </div>
              ) : settingsError ? (
                <div
                  className={`computer-settings-setup${
                    settingsMessageTone === "error"
                      ? " computer-settings-setup-error"
                      : ""
                  }`}
                  role={settingsMessageTone === "error" ? "alert" : "status"}
                >
                  {settingsError}
                </div>
              ) : null}
            </div>

            <AwakenSettingsSwitch
              label="允许后台窗口动作"
              description="开启后 Agent 可以对非前台窗口执行动作，仍受运行期安全策略与复核约束。"
              checked={settings.allow_background_window_actions}
              disabled={settingsBusy}
              busy={settingsBusy}
              onCheckedChange={(checked) =>
                patch({ allow_background_window_actions: checked })
              }
            />

          </div>
        )}
      </div>

      <div className="computer-settings-card">
        <div className="computer-settings-card-header">
          <h3>能力范围与限制</h3>
        </div>
        <ul className="computer-settings-list">
          <li>窗口与完整桌面范围默认可用，Agent 会优先使用窗口范围。</li>
          <li>点击、滚动、按键、非密码文本输入和等待可直接执行低风险动作。</li>
          <li>发送、提交、删除等对外有后果的动作会触发一次性的用户复核。</li>
          <li>密码、OTP、支付、登录/身份验证、系统安全授权必须交由用户接管完成。</li>
          <li>截图与 accessibility 树按会话隔离，最长保留 24 小时，总量上限 500 MiB。</li>
        </ul>
      </div>

    </section>
  );
}
