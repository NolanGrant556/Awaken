import { useCallback, useEffect, useRef, useState } from "react";
import {
  Bot,
  Code,
  Cpu,
  Plus,
  Terminal,
  Trash2,
  Wifi,
  Zap,
} from "@awaken/design-system";
import {
  AwakenButton as Button,
  AwakenDialog,
  AwakenSwitch,
  AwakenTooltip as Tooltip,
} from "@awaken/design-system";
import type {
  ACPPermissionMode,
  ACPRunnerCreateRequest,
  ACPRunnerEnvVar,
  ACPRunnerUpdateRequest,
  ACPRunnerView,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";

const BUILTIN_ACP_ORDER = [
  "opencode",
  "qwen_code",
  "claude_code",
  "codex",
] as const;

const BUILTIN_ACP_TEMPLATES: Record<string, ACPRunnerCreateRequest> = {
  opencode: {
    id: "opencode",
    enabled: false,
    command: "opencode",
    args: ["acp"],
    env: {},
    permission_mode: "ask",
    startup_timeout_seconds: 15,
    turn_timeout_seconds: 600,
    max_frame_bytes: 8 * 1024 * 1024,
  },
  qwen_code: {
    id: "qwen_code",
    enabled: false,
    command: "qwen",
    args: ["--acp"],
    env: {},
    permission_mode: "ask",
    startup_timeout_seconds: 15,
    turn_timeout_seconds: 600,
    max_frame_bytes: 8 * 1024 * 1024,
  },
  claude_code: {
    id: "claude_code",
    enabled: false,
    command: "npx",
    args: ["-y", "@zed-industries/claude-agent-acp"],
    env: {},
    permission_mode: "ask",
    startup_timeout_seconds: 15,
    turn_timeout_seconds: 600,
    max_frame_bytes: 8 * 1024 * 1024,
  },
  codex: {
    id: "codex",
    enabled: false,
    command: "npx",
    args: ["-y", "@zed-industries/codex-acp"],
    env: {},
    permission_mode: "ask",
    startup_timeout_seconds: 15,
    turn_timeout_seconds: 600,
    max_frame_bytes: 8 * 1024 * 1024,
  },
};

const BUILTIN_ACP_DISPLAY_NAMES: Record<string, string> = {
  opencode: "OpenCode",
  qwen_code: "Qwen Code",
  claude_code: "Claude Code",
  codex: "Codex",
};

const BUILTIN_ACP_ICONS: Record<string, React.ReactNode> = {
  opencode: <Code size={22} strokeWidth={1.9} />,
  qwen_code: <Terminal size={22} strokeWidth={1.9} />,
  claude_code: <Zap size={22} strokeWidth={1.9} />,
  codex: <Cpu size={22} strokeWidth={1.9} />,
};

const RUNNER_ID_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;

type EditorMode = "create" | "edit";

function isBuiltinACPRunner(id: string): boolean {
  return BUILTIN_ACP_ORDER.includes(id as (typeof BUILTIN_ACP_ORDER)[number]);
}

function normalizeRunner(config: ACPRunnerView): ACPRunnerView {
  return {
    ...config,
    args: config.args ?? [],
    env: config.env ?? [],
    permission_mode: config.permission_mode ?? "ask",
    startup_timeout_seconds: config.startup_timeout_seconds ?? 15,
    turn_timeout_seconds: config.turn_timeout_seconds ?? 600,
    max_frame_bytes: config.max_frame_bytes ?? 8 * 1024 * 1024,
  };
}

function templateToView(template: ACPRunnerCreateRequest): ACPRunnerView {
  return {
    ...template,
    env: Object.keys(template.env ?? {}).map((key) => ({
      key,
      has_value: (template.env ?? {})[key] !== "",
    })),
  };
}

function parseArgsText(value: string): string[] {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function stringifyArgs(args: string[] = []): string {
  return args.join("\n");
}

function parseEnvText(value: string): Record<string, string> {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean)
    .reduce<Record<string, string>>((acc, line) => {
      const index = line.indexOf("=");
      if (index >= 0) {
        const key = line.slice(0, index).trim();
        const envValue = line.slice(index + 1).trim();
        if (key) acc[key] = envValue;
      }
      return acc;
    }, {});
}

function stringifyEnvForEdit(env: ACPRunnerEnvVar[] = []): string {
  return env.map(({ key }) => `${key}=`).join("\n");
}

function parseEnvTextForEdit(value: string): {
  set: Record<string, string>;
  presentKeys: Set<string>;
} {
  const set: Record<string, string> = {};
  const presentKeys = new Set<string>();
  for (const line of value.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const index = trimmed.indexOf("=");
    const key = index >= 0 ? trimmed.slice(0, index).trim() : trimmed;
    const envValue = index >= 0 ? trimmed.slice(index + 1).trim() : "";
    if (!key) continue;
    presentKeys.add(key);
    if (envValue) {
      set[key] = envValue;
    }
  }
  return { set, presentKeys };
}

function formatProbeError(error: unknown, name: string): string {
  if (!(error instanceof Error)) return "探测失败";
  let message = error.message;
  const ipcPrefix = "Error invoking remote method 'awaken:probe-acp-runner': ";
  if (message.startsWith(ipcPrefix)) {
    message = message.slice(ipcPrefix.length);
  }
  if (
    message.includes("executable file not found in %PATH%") ||
    message.includes("executable file not found in PATH")
  ) {
    return `未找到 ${name}，请确认已安装并可在终端运行。`;
  }
  if (message.includes("acp_runner_not_ready")) {
    return `${name} 未就绪，请检查安装与 PATH 配置。`;
  }
  if (message.includes("acp_runner_not_configured")) {
    return "runner 尚未配置。";
  }
  return `探测失败：${message}`;
}

function computeDisplayOrder(
  runnerMap: Record<string, ACPRunnerView>,
): string[] {
  const allIds = Object.keys(runnerMap);
  const ordered = [
    ...BUILTIN_ACP_ORDER.filter((id) => allIds.includes(id)),
    ...allIds
      .filter((id) => !isBuiltinACPRunner(id))
      .sort((a, b) => a.localeCompare(b)),
  ];
  const enabled: string[] = [];
  const disabled: string[] = [];
  for (const id of ordered) {
    if (runnerMap[id]?.enabled) enabled.push(id);
    else disabled.push(id);
  }
  return [...enabled, ...disabled];
}

function findInvalidEnvLine(value: string): string | null {
  const lines = value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
  for (const line of lines) {
    const index = line.indexOf("=");
    if (index <= 0 || !line.slice(0, index).trim()) {
      return line;
    }
  }
  return null;
}

type FormState = {
  id: string;
  enabled: boolean;
  command: string;
  argsText: string;
  envText: string;
  permission_mode: ACPPermissionMode;
  startup_timeout_seconds: number;
  turn_timeout_seconds: number;
  max_frame_bytes: number;
};

function runnerToFormState(runner: ACPRunnerView): FormState {
  return {
    id: runner.id,
    enabled: runner.enabled,
    command: runner.command,
    argsText: stringifyArgs(runner.args),
    envText: stringifyEnvForEdit(runner.env),
    permission_mode: runner.permission_mode,
    startup_timeout_seconds: runner.startup_timeout_seconds,
    turn_timeout_seconds: runner.turn_timeout_seconds,
    max_frame_bytes: runner.max_frame_bytes,
  };
}

function formStateToRunner(form: FormState): ACPRunnerCreateRequest {
  return {
    id: form.id.trim(),
    enabled: form.enabled,
    command: form.command.trim(),
    args: parseArgsText(form.argsText),
    env: parseEnvText(form.envText),
    permission_mode: form.permission_mode,
    startup_timeout_seconds: Number(form.startup_timeout_seconds) || 15,
    turn_timeout_seconds: Number(form.turn_timeout_seconds) || 600,
    max_frame_bytes: Number(form.max_frame_bytes) || 8 * 1024 * 1024,
  };
}

function validateForm(form: FormState, existingIds: string[]): string | null {
  const id = form.id.trim();
  if (!RUNNER_ID_RE.test(id)) {
    return "Runner ID 只能包含小写字母、数字、下划线和连字符，长度 1–64。";
  }
  if (!form.command.trim()) {
    return "启动命令不能为空。";
  }
  const invalidEnv = findInvalidEnvLine(form.envText);
  if (invalidEnv) {
    return `环境变量格式错误：${invalidEnv}`;
  }
  if (existingIds.includes(id)) {
    return "Runner ID 已存在。";
  }
  return null;
}

function mergeRunners(
  apiRunners: ACPRunnerView[],
): Record<string, ACPRunnerView> {
  const merged: Record<string, ACPRunnerView> = {};
  for (const key of BUILTIN_ACP_ORDER) {
    merged[key] = normalizeRunner(templateToView(BUILTIN_ACP_TEMPLATES[key]));
  }
  for (const runner of apiRunners) {
    const normalized = normalizeRunner(runner);
    merged[normalized.id] = normalized;
  }
  return merged;
}

type ACPSettingsPanelProps = {
  runtimeClient: RuntimeClient;
};

export function ACPSettingsPanel({ runtimeClient }: ACPSettingsPanelProps) {
  const [runners, setRunners] = useState<Record<string, ACPRunnerView>>(() =>
    mergeRunners([]),
  );
  const [displayOrder, setDisplayOrder] = useState<string[]>(() =>
    computeDisplayOrder(mergeRunners([])),
  );
  const [persistedIds, setPersistedIds] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorMode, setEditorMode] = useState<EditorMode>("create");
  const [activeId, setActiveId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(() =>
    runnerToFormState(templateToView(BUILTIN_ACP_TEMPLATES.opencode)),
  );
  const [saving, setSaving] = useState(false);
  const [probeStates, setProbeStates] = useState<
    Record<string, { busy: boolean; result?: string }>
  >({});
  const initialEditorFocusRef = useRef<HTMLInputElement>(null);
  const editingBuiltin =
    editorMode === "edit" && activeId !== null && isBuiltinACPRunner(activeId);

  const loadRunners = useCallback(
    async (preserveOrder = false) => {
      setLoading(true);
      setMessage(null);
      try {
        const settings = await runtimeClient.getACPRunners();
        const merged = mergeRunners(settings.runners);
        setRunners(merged);
        if (!preserveOrder) {
          setDisplayOrder(computeDisplayOrder(merged));
        }
        setPersistedIds(new Set(settings.runners.map((runner) => runner.id)));
      } catch (error) {
        setMessage(
          error instanceof Error ? error.message : "无法读取 ACP runner 配置。",
        );
      } finally {
        setLoading(false);
      }
    },
    [runtimeClient],
  );

  useEffect(() => {
    void loadRunners(false);
  }, [loadRunners]);

  function openCreateDialog() {
    setEditorMode("create");
    setActiveId(null);
    setForm(runnerToFormState(templateToView(BUILTIN_ACP_TEMPLATES.opencode)));
    setForm((current) => ({ ...current, id: "" }));
    setEditorOpen(true);
    setMessage(null);
  }

  function openEditDialog(id: string) {
    const runner = runners[id];
    if (!runner) return;
    setEditorMode("edit");
    setActiveId(id);
    setForm(runnerToFormState(runner));
    setEditorOpen(true);
    setMessage(null);
  }

  function closeDialog() {
    setEditorOpen(false);
    setActiveId(null);
    setMessage(null);
  }

  async function handleToggle(id: string) {
    const runner = runners[id];
    if (!runner) return;
    try {
      if (persistedIds.has(id)) {
        await runtimeClient.updateACPRunner(id, { enabled: !runner.enabled });
      } else {
        const template = BUILTIN_ACP_TEMPLATES[id];
        if (!template) return;
        await runtimeClient.createACPRunner({
          ...template,
          enabled: !runner.enabled,
        });
      }
      await loadRunners(true);
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "切换 runner 状态失败。",
      );
    }
  }

  async function handleProbe(id: string) {
    const runner = runners[id];
    if (!runner) return;
    const displayName = BUILTIN_ACP_DISPLAY_NAMES[id] ?? id;
    setProbeStates((current) => ({ ...current, [id]: { busy: true } }));
    try {
      if (!persistedIds.has(id)) {
        const template = BUILTIN_ACP_TEMPLATES[id];
        if (!template) return;
        await runtimeClient.createACPRunner(template);
        setPersistedIds((current) => new Set([...current, id]));
      }
      const result = await runtimeClient.probeACPRunner(id);
      setProbeStates((current) => ({
        ...current,
        [id]: {
          busy: false,
          result: result.ready ? `${displayName} 已就绪` : "探测未就绪",
        },
      }));
    } catch (error) {
      setProbeStates((current) => ({
        ...current,
        [id]: {
          busy: false,
          result: formatProbeError(error, displayName),
        },
      }));
    }
  }

  async function handleSave() {
    const existingIds = Object.keys(runners);
    const targetId = form.id.trim();
    const isRename = editorMode === "edit" && activeId && targetId !== activeId;
    const validationIds = isRename
      ? existingIds.filter((id) => id !== activeId)
      : editorMode === "create"
        ? existingIds
        : existingIds.filter((id) => id !== activeId);
    const validationError = validateForm(form, validationIds);
    if (validationError) {
      setMessage(validationError);
      return;
    }

    setSaving(true);
    setMessage(null);
    try {
      const runner = formStateToRunner(form);
      if (editorMode === "create") {
        await runtimeClient.createACPRunner(runner);
      } else if (isRename && activeId) {
        if (persistedIds.has(activeId)) {
          await runtimeClient.deleteACPRunner(activeId);
        }
        await runtimeClient.createACPRunner(runner);
      } else if (activeId) {
        if (persistedIds.has(activeId)) {
          const originalKeys = new Set(
            (runners[activeId]?.env ?? []).map((e) => e.key),
          );
          const { set: envSet, presentKeys } = parseEnvTextForEdit(
            form.envText,
          );
          const envRemove = [...originalKeys].filter(
            (key) => !presentKeys.has(key),
          );
          const update: ACPRunnerUpdateRequest = {
            enabled: runner.enabled,
            command: runner.command,
            args: runner.args,
            env_set: envSet,
            env_remove: envRemove,
            permission_mode: runner.permission_mode,
            startup_timeout_seconds: runner.startup_timeout_seconds,
            turn_timeout_seconds: runner.turn_timeout_seconds,
            max_frame_bytes: runner.max_frame_bytes,
          };
          await runtimeClient.updateACPRunner(activeId, update);
        } else {
          await runtimeClient.createACPRunner(runner);
        }
      }
      await loadRunners();
      closeDialog();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "保存 runner 失败。");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!activeId || isBuiltinACPRunner(activeId)) return;
    if (
      !window.confirm(`确定删除自定义 runner ${activeId} 吗？此操作不可撤销。`)
    )
      return;
    setSaving(true);
    setMessage(null);
    try {
      await runtimeClient.deleteACPRunner(activeId);
      await loadRunners();
      closeDialog();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "删除 runner 失败。");
    } finally {
      setSaving(false);
    }
  }

  function updateForm(patch: Partial<FormState>) {
    setForm((current) => ({ ...current, ...patch }));
  }

  return (
    <section className="acp-settings" aria-label="ACP runner">
      <div className="acp-settings-toolbar">
        <div>
          <h2>外部 Agent / ACP</h2>
          <p>
            配置并启用本地 ACP runner 后，主 Agent 可通过
            delegate_external_agent 工具与其协作。
          </p>
        </div>
        <Button
          variant="default"
          size="md"
          icon={<Plus size={14} />}
          onClick={openCreateDialog}
        >
          添加 runner
        </Button>
      </div>

      {message && !editorOpen ? (
        <div className="model-settings-message" role="status">
          {message}
        </div>
      ) : null}

      {loading ? (
        <div className="acp-runner-empty">加载中…</div>
      ) : (
        <div className="acp-runner-list">
          {displayOrder
            .filter((id) => runners[id] !== undefined)
            .map((id) => {
              const runner = runners[id];
              if (!runner) return null;
              const builtin = isBuiltinACPRunner(id);
              const probe = probeStates[id];
              const icon = BUILTIN_ACP_ICONS[id] ?? (
                <Bot size={22} strokeWidth={1.9} />
              );
              return (
                <article
                  key={id}
                  className={`acp-runner-card${runner.enabled ? " acp-runner-card-enabled" : ""}`}
                  onClick={() => openEditDialog(id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      openEditDialog(id);
                    }
                  }}
                >
                  <div className="acp-runner-icon" aria-hidden="true">
                    {icon}
                  </div>
                  <div className="acp-runner-copy">
                    <div className="acp-runner-title-row">
                      <h3>{id}</h3>
                      <span
                        className={`acp-runner-tag${builtin ? " acp-runner-tag-builtin" : " acp-runner-tag-custom"}`}
                      >
                        {builtin ? "内置" : "自定义"}
                      </span>
                      <span
                        className={`extension-state ${
                          runner.enabled
                            ? "extension-state-connected"
                            : "extension-state-disabled"
                        }`}
                      >
                        {runner.enabled ? "已启用" : "已禁用"}
                      </span>
                    </div>
                    <div className="acp-runner-meta">
                      <code>{runner.command}</code>
                      {runner.args.length > 0 ? (
                        <span>{runner.args.join(" ")}</span>
                      ) : null}
                    </div>
                    {probe?.result ? (
                      <div className="search-channel-test-result" role="status">
                        {probe.result}
                      </div>
                    ) : null}
                  </div>
                  <div
                    className="acp-runner-actions"
                    onClick={(event) => event.stopPropagation()}
                    onKeyDown={(event) => event.stopPropagation()}
                  >
                    <Tooltip label="探测命令是否可访问" placement="top">
                      <button
                        type="button"
                        className="search-channel-test-button"
                        disabled={probe?.busy}
                        onClick={() => void handleProbe(id)}
                      >
                        <Wifi size={15} strokeWidth={1.9} />
                        {probe?.busy ? "探测中…" : "探测"}
                      </button>
                    </Tooltip>
                    <AwakenSwitch
                      checked={runner.enabled}
                      label={`${id} ACP runner`}
                      onCheckedChange={() => void handleToggle(id)}
                    />
                  </div>
                </article>
              );
            })}
        </div>
      )}

      <AwakenDialog
        open={editorOpen}
        title={
          editorMode === "create" ? "添加 ACP runner" : `编辑 ${activeId ?? ""}`
        }
        initialFocusRef={initialEditorFocusRef}
        className="acp-runner-dialog"
        bodyClassName="acp-runner-dialog__body"
        onOpenChange={(open) => {
          if (!open) closeDialog();
        }}
        footer={
          <div className="acp-runner-dialog-footer">
            {editorMode === "edit" &&
            activeId &&
            !isBuiltinACPRunner(activeId) ? (
              <Button
                variant="ghost"
                size="md"
                icon={<Trash2 size={14} />}
                onClick={() => void handleDelete()}
                disabled={saving}
              >
                删除
              </Button>
            ) : (
              <span />
            )}
            <div className="acp-runner-dialog-footer__actions">
              <Button
                variant="ghost"
                size="md"
                onClick={closeDialog}
                disabled={saving}
              >
                取消
              </Button>
              <Button
                variant="default"
                size="md"
                onClick={() => void handleSave()}
                disabled={saving}
              >
                {saving ? "保存中…" : "保存"}
              </Button>
            </div>
          </div>
        }
      >
        {message ? (
          <div className="acp-runner-dialog-message" role="alert">
            {message}
          </div>
        ) : null}
        <div className="acp-runner-form">
          <label className="acp-runner-field">
            <span>Runner ID</span>
            <input
              ref={editingBuiltin ? undefined : initialEditorFocusRef}
              type="text"
              value={form.id}
              disabled={
                editorMode === "edit" && activeId
                  ? isBuiltinACPRunner(activeId)
                  : false
              }
              placeholder="my_runner"
              onChange={(event) => updateForm({ id: event.target.value })}
            />
          </label>

          <div className="acp-runner-field acp-runner-field-inline">
            <span>启用</span>
            <AwakenSwitch
              checked={form.enabled}
              label="ACP runner"
              onCheckedChange={(enabled) => updateForm({ enabled })}
            />
          </div>

          <label className="acp-runner-field">
            <span>启动命令</span>
            <input
              ref={editingBuiltin ? initialEditorFocusRef : undefined}
              type="text"
              value={form.command}
              placeholder="npx"
              onChange={(event) => updateForm({ command: event.target.value })}
            />
          </label>

          <label className="acp-runner-field">
            <span>参数（每行一个）</span>
            <textarea
              value={form.argsText}
              rows={4}
              placeholder="--acp"
              onChange={(event) => updateForm({ argsText: event.target.value })}
            />
          </label>

          <label className="acp-runner-field">
            <span>环境变量（每行 KEY=VALUE）</span>
            <textarea
              value={form.envText}
              rows={4}
              placeholder="API_KEY=xxx"
              onChange={(event) => updateForm({ envText: event.target.value })}
            />
          </label>

          <label className="acp-runner-field">
            <span>权限模式</span>
            <select
              value={form.permission_mode}
              onChange={(event) =>
                updateForm({
                  permission_mode: event.target.value as ACPPermissionMode,
                })
              }
            >
              <option value="ask">ask（请求用户确认）</option>
              <option value="deny">deny（自动拒绝）</option>
            </select>
          </label>

          <div className="acp-runner-field-row">
            <label className="acp-runner-field">
              <span>启动超时（秒）</span>
              <input
                type="number"
                min={1}
                max={300}
                value={form.startup_timeout_seconds}
                onChange={(event) =>
                  updateForm({
                    startup_timeout_seconds: Number(event.target.value),
                  })
                }
              />
            </label>
            <label className="acp-runner-field">
              <span>单轮超时（秒）</span>
              <input
                type="number"
                min={1}
                max={3600}
                value={form.turn_timeout_seconds}
                onChange={(event) =>
                  updateForm({
                    turn_timeout_seconds: Number(event.target.value),
                  })
                }
              />
            </label>
            <label className="acp-runner-field">
              <span>最大帧字节</span>
              <input
                type="number"
                min={1024}
                max={64 * 1024 * 1024}
                value={form.max_frame_bytes}
                onChange={(event) =>
                  updateForm({ max_frame_bytes: Number(event.target.value) })
                }
              />
            </label>
          </div>
        </div>
      </AwakenDialog>
    </section>
  );
}
