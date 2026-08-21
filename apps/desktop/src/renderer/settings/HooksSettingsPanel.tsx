import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";
import type { HookReviewItem, HookSourceReceipt, HookStatus, RuntimeProject } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import "./HooksSettingsPanel.css";

const EVENT_LABELS: Record<string, string> = {
  RunStart: "任务开始",
  UserPromptSubmit: "提交用户输入后",
  PreToolUse: "工具执行前",
  PostToolUse: "工具执行后",
  PostToolUseFailure: "工具执行失败后",
  PreCompact: "上下文压缩前",
  PostCompact: "上下文压缩后",
  SubagentStart: "子任务开始",
  SubagentStop: "子任务结束",
  BeforeFinish: "最终回答前",
  RunEnd: "任务结束",
};

const TYPE_LABELS: Record<HookReviewItem["type"], string> = {
  command: "命令",
  http: "HTTP 请求",
  prompt: "模型提示词",
  agent: "只读智能体",
};

const STATUS_LABELS: Record<HookSourceReceipt["status"], string> = {
  enabled: "已启用",
  disabled: "已停用",
  untrusted: "待信任",
  invalid: "配置无效",
};

const SOURCE_LABELS: Record<HookSourceReceipt["kind"], string> = {
  user: "用户配置",
  project: "项目共享配置",
  project_local: "项目本地配置",
};

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return <div className="aw-hooks-settings__detail-row"><dt>{label}</dt><dd>{children}</dd></div>;
}

function HookDetails({ hook }: { hook: HookReviewItem }) {
  return (
    <div className="aw-hooks-settings__detail-card">
      <DetailRow label="处理程序">{TYPE_LABELS[hook.type]}</DetailRow>
      {hook.command?.length ? <DetailRow label="命令"><code>{JSON.stringify(hook.command)}</code></DetailRow> : null}
      {hook.cwd ? <DetailRow label="工作目录">{hook.cwd === "project_root" ? "配置所属项目根目录" : "任务工作区根目录"}</DetailRow> : null}
      {hook.url ? <DetailRow label="地址"><code>{hook.url}</code></DetailRow> : null}
      {hook.toolNames?.length ? <DetailRow label="匹配器"><code>{hook.toolNames.join(" | ")}</code></DetailRow> : null}
      {hook.prompt ? <DetailRow label="提示词"><pre>{hook.prompt}</pre></DetailRow> : null}
      {hook.model ? <DetailRow label="模型">{hook.model}</DetailRow> : null}
      {hook.maxTurns ? <DetailRow label="最大轮次">{hook.maxTurns}</DetailRow> : null}
      {hook.secretHeaderNames?.length ? <DetailRow label="凭据请求头"><code>{hook.secretHeaderNames.join(", ")}</code><span className="aw-hooks-settings__secret-note">（值不会展示）</span></DetailRow> : null}
      {hook.type === "http" ? <DetailRow label="允许本机地址">{hook.allowLoopback ? "是" : "否"}</DetailRow> : null}
      <DetailRow label="超时">{hook.timeoutMs / 1000} 秒</DetailRow>
      <DetailRow label="输出上限">{Math.ceil(hook.maxOutputBytes / 1024)} KiB</DetailRow>
      <DetailRow label="失败策略">{hook.onFailure === "block" ? "阻止当前操作" : "记录后继续"}</DetailRow>
    </div>
  );
}

function SourceDetails({ source, projectId, loading, onTrustChange }: {
  source: HookSourceReceipt;
  projectId: string;
  loading: boolean;
  onTrustChange: (source: HookSourceReceipt, trust: boolean) => void;
}) {
  const grouped = useMemo(() => {
    const groups = new Map<string, HookReviewItem[]>();
    for (const hook of source.hooks ?? []) groups.set(hook.event, [...(groups.get(hook.event) ?? []), hook]);
    return [...groups.entries()];
  }, [source.hooks]);

  return (
    <article className="aw-hooks-settings__source">
      <div className="aw-hooks-settings__source-heading">
        <div><strong>{SOURCE_LABELS[source.kind]}</strong><span>{source.hookCount} 个钩子</span></div>
        <span className={`aw-hooks-settings__status aw-hooks-settings__status--${source.status}`}>{STATUS_LABELS[source.status]}</span>
      </div>
      <code className="aw-hooks-settings__path" title={source.path}>{source.path}</code>
      {source.digest ? <small>配置摘要 {source.digest.slice(0, 12)}</small> : null}
      {source.diagnostic ? <p className="aw-hooks-settings__diagnostic">{source.diagnostic}</p> : null}
      {grouped.map(([event, hooks]) => (
        <section key={event} className="aw-hooks-settings__event">
          <div className="aw-hooks-settings__event-heading"><AwakenIcon concept="hook" size={16} /><div><strong>{event}</strong><span>{EVENT_LABELS[event] ?? event}</span></div></div>
          {hooks.map((hook, index) => (
            <details key={`${event}:${hook.id}`} className="aw-hooks-settings__hook" open={index === 0}>
              <summary><span>{hook.id}</span><span>{TYPE_LABELS[hook.type]}</span><AwakenIcon concept="dropdown" size={16} /></summary>
              <HookDetails hook={hook} />
            </details>
          ))}
        </section>
      ))}
      {source.status !== "invalid" && grouped.length === 0 ? <p className="aw-hooks-settings__empty-source">此配置没有已声明的钩子。</p> : null}
      {source.kind !== "user" && projectId ? (
        <div className="aw-hooks-settings__trust-actions">
          {source.status === "enabled"
            ? <button type="button" disabled={loading} onClick={() => onTrustChange(source, false)}>撤销信任</button>
            : source.status === "untrusted"
              ? <button type="button" disabled={loading || !source.digest} onClick={() => onTrustChange(source, true)}><AwakenIcon concept="trusted" size={16} />信任当前配置</button>
              : null}
        </div>
      ) : null}
    </article>
  );
}

export function HooksSettingsPanel({ runtimeClient, currentProjectId }: { runtimeClient: RuntimeClient; currentProjectId?: string }) {
  const [projects, setProjects] = useState<RuntimeProject[]>([]);
  const [projectId, setProjectId] = useState(currentProjectId ?? "");
  const [status, setStatus] = useState<HookStatus | null>(null);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async (selected = projectId) => {
    if (!runtimeClient.getHookStatus) {
      setMessage("当前 Desktop 运行时尚未提供钩子配置接口。");
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const [nextStatus, nextProjects] = await Promise.all([runtimeClient.getHookStatus(selected || undefined), runtimeClient.listProjects(false)]);
      setStatus(nextStatus);
      setProjects(nextProjects.filter((project) => project.state === "active"));
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "读取钩子状态失败。");
    } finally { setLoading(false); }
  }, [projectId, runtimeClient]);

  useEffect(() => { void refresh(projectId); }, [projectId, refresh]);

  const updateTrust = async (source: HookSourceReceipt, trust: boolean) => {
    if (!projectId) return;
    setLoading(true);
    try {
      const next = trust
        ? await runtimeClient.trustProjectHook?.({ projectId, sourcePath: source.path, digest: source.digest ?? "" })
        : await runtimeClient.revokeProjectHook?.({ projectId, sourcePath: source.path });
      if (next) setStatus(next);
      setMessage(trust ? "已信任当前配置摘要；只会影响后续新任务。" : "已撤销信任；只会影响后续新任务。");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "更新钩子信任失败。");
    } finally { setLoading(false); }
  };

  return (
    <section className="aw-hooks-settings" aria-label="钩子设置">
      <header><p>查看用户与项目配置，并在信任前审查每个钩子的执行条件和操作。</p></header>
      <div className="aw-hooks-settings__warning"><AwakenIcon concept="warning" size={20} /><span>钩子可以在沙盒外以当前用户身份运行。请审查最近安装或修改的所有钩子。</span></div>
      <label className="aw-hooks-settings__project">项目<select value={projectId} onChange={(event) => setProjectId(event.target.value)}><option value="">仅用户配置</option>{projects.map((project) => <option key={project.projectId} value={project.projectId}>{project.displayName}</option>)}</select></label>
      {message ? <p role="status" className="aw-hooks-settings__message">{message}</p> : null}
      {loading ? <p className="aw-hooks-settings__loading">正在读取钩子状态…</p> : null}
      {!loading && (status?.sources?.length ?? 0) === 0 ? <p className="aw-hooks-settings__empty">当前范围没有钩子配置。</p> : null}
      <div className="aw-hooks-settings__sources">
        {status?.sources?.map((source) => <SourceDetails key={`${source.kind}:${source.path}`} source={source} projectId={projectId} loading={loading} onTrustChange={(item, trust) => void updateTrust(item, trust)} />)}
      </div>
    </section>
  );
}
