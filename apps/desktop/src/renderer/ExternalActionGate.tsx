import { useEffect, useMemo, useState } from "react";
import type { ExternalInteractionItem } from "./runtime/reducer";

type ExternalActionGateProps = {
  interaction: ExternalInteractionItem;
  compact?: boolean;
  resolving?: boolean;
  onConfirmDone: () => void;
  onSubmitForm?: (content: Record<string, unknown>) => void;
  onCancel: () => void;
  onRetry?: () => void;
  onResolveACPOption?: (optionId: string) => void;
};

export function ExternalActionGate({
  interaction,
  compact = false,
  resolving = false,
  onConfirmDone,
  onSubmitForm,
  onCancel,
  onRetry,
  onResolveACPOption,
}: ExternalActionGateProps) {
  const isACPPermission = interaction.kind === "acp-permission";
  const isMCPElicitation = interaction.kind === "mcp-elicitation";
  const canCancel = interaction.recoveries.includes("cancel");
  const canConfirm = interaction.recoveries.includes("confirm_done");
  const canRetry =
    interaction.recoveries.includes("retry") ||
    interaction.recoveries.includes("refresh_qr");
  const title =
    interaction.title?.trim() ||
    (interaction.provider === "dingtalk-workspace"
      ? "钉钉授权"
      : interaction.provider === "lark-cli"
        ? "飞书授权"
        : "需要完成外部操作");
  const form = useMemo(() => mcpForm(interaction), [interaction]);
  const [formValues, setFormValues] = useState<Record<string, unknown>>(() => form.defaults);
  useEffect(() => {
    setFormValues(form.defaults);
  }, [form.defaults, interaction.id]);
  const formValid = form.fields.every((field) => {
    if (!field.required) return true;
    const value = formValues[field.name];
    return field.type === "boolean" ? typeof value === "boolean" : String(value ?? "").trim() !== "";
  });

  return (
    <section
      className={`external-action-gate cli-auth-card${compact ? " cli-auth-card-compact" : ""}`}
      aria-live="polite"
      data-testid="external-action-gate"
    >
      <div className="cli-auth-card-copy">
        <strong>{title}</strong>
        <span>{interaction.safeMessage ?? "请完成外部操作后继续"}</span>
      </div>
      {isMCPElicitation ? (
        <form
          className="mcp-elicitation-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (formValid) onSubmitForm?.(formValues);
          }}
        >
          {form.fields.map((field) => (
            <label key={field.name} className="mcp-elicitation-field">
              <span>
                {field.title}
                {field.required ? " *" : ""}
              </span>
              {field.description ? <small>{field.description}</small> : null}
              {field.enumValues.length > 0 ? (
                <select
                  value={String(formValues[field.name] ?? "")}
                  disabled={resolving}
                  onChange={(event) => {
                    const selected = field.enumValues.find(
                      (value) => String(value) === event.target.value,
                    );
                    setFormValues((current) => ({ ...current, [field.name]: selected }));
                  }}
                >
                  {!field.required ? <option value="">未选择</option> : null}
                  {field.enumValues.map((value) => (
                    <option key={String(value)} value={String(value)}>{String(value)}</option>
                  ))}
                </select>
              ) : field.type === "boolean" ? (
                <input
                  type="checkbox"
                  checked={Boolean(formValues[field.name])}
                  disabled={resolving}
                  onChange={(event) => {
                    setFormValues((current) => ({ ...current, [field.name]: event.target.checked }));
                  }}
                />
              ) : (
                <input
                  type={field.type === "number" || field.type === "integer" ? "number" : "text"}
                  value={String(formValues[field.name] ?? "")}
                  required={field.required}
                  disabled={resolving}
                  onChange={(event) => {
                    const raw = event.target.value;
                    const value = field.type === "number" || field.type === "integer"
                      ? raw === "" ? "" : Number(raw)
                      : raw;
                    setFormValues((current) => ({ ...current, [field.name]: value }));
                  }}
                />
              )}
            </label>
          ))}
          <div className="cli-auth-card-actions">
            <button type="submit" disabled={resolving || !formValid}>确认</button>
            {canCancel ? (
              <button type="button" disabled={resolving} onClick={onCancel}>取消</button>
            ) : null}
          </div>
        </form>
      ) : isACPPermission && interaction.options && interaction.options.length > 0 ? (
        <div className="cli-auth-card-actions cli-auth-card-actions--options">
          {interaction.options.map((option) => (
            <button
              key={option.optionId}
              type="button"
              disabled={resolving}
              onClick={() => onResolveACPOption?.(option.optionId)}
            >
              {option.name}
            </button>
          ))}
          {canCancel ? (
            <button type="button" disabled={resolving} onClick={onCancel}>
              取消
            </button>
          ) : null}
        </div>
      ) : (
        <>
          <div className="cli-auth-qr-frame">
            {interaction.qrImageUrl ? (
              <img src={interaction.qrImageUrl} alt="授权二维码" width={224} height={224} />
            ) : (
              <div className="cli-auth-qr-placeholder">
                {interaction.verificationUrl ? "使用下方链接完成授权" : "等待授权信息"}
              </div>
            )}
          </div>
          <div className="cli-auth-card-actions">
            {interaction.verificationUrl ? (
              <a href={interaction.verificationUrl} target="_blank" rel="noreferrer">
                打开授权链接
              </a>
            ) : null}
            {canConfirm ? (
              <button type="button" disabled={resolving} onClick={onConfirmDone}>
                我已完成
              </button>
            ) : null}
            {canRetry && onRetry ? (
              <button type="button" disabled={resolving} onClick={onRetry}>
                重新获取
              </button>
            ) : null}
            {canCancel ? (
              <button type="button" disabled={resolving} onClick={onCancel}>
                取消
              </button>
            ) : null}
          </div>
        </>
      )}
    </section>
  );
}

type MCPFormField = {
  name: string;
  title: string;
  description?: string;
  type: string;
  required: boolean;
  enumValues: unknown[];
  defaultValue?: unknown;
};

function mcpForm(interaction: ExternalInteractionItem): {
  fields: MCPFormField[];
  defaults: Record<string, unknown>;
} {
  const schema = interaction.formSchema ?? {};
  const rawProperties = schema.properties;
  const properties = rawProperties && typeof rawProperties === "object" && !Array.isArray(rawProperties)
    ? rawProperties as Record<string, unknown>
    : {};
  const required = new Set(
    Array.isArray(schema.required)
      ? schema.required.filter((value): value is string => typeof value === "string")
      : [],
  );
  const fields = Object.entries(properties).map(([name, raw]) => {
    const property = raw && typeof raw === "object" && !Array.isArray(raw)
      ? raw as Record<string, unknown>
      : {};
    return {
      name,
      title: typeof property.title === "string" && property.title.trim() ? property.title : name,
      description: typeof property.description === "string" ? property.description : undefined,
      type: typeof property.type === "string" ? property.type : "string",
      required: required.has(name),
      enumValues: Array.isArray(property.enum) ? property.enum : [],
      defaultValue: property.default,
    } satisfies MCPFormField;
  });
  const defaults: Record<string, unknown> = {};
  for (const field of fields) {
    if (field.defaultValue !== undefined) defaults[field.name] = field.defaultValue;
    else if (field.type === "boolean") defaults[field.name] = false;
    else if (field.enumValues.length > 0 && field.required) defaults[field.name] = field.enumValues[0];
    else defaults[field.name] = "";
  }
  return { fields, defaults };
}
