import {
  AlertTriangle,
  CircleCheck,
  FileText,
  RefreshCw,
  Save,
  Trash2,
} from "@awaken/design-system";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { GlobalAgentsDocument } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import "./GlobalAgentsSettingsPanel.css";

type Props = {
  runtimeClient: Pick<
    RuntimeClient,
    "getGlobalAgentsDocument" | "saveGlobalAgentsDocument" | "deleteGlobalAgentsDocument"
  >;
};

export function GlobalAgentsSettingsPanel({ runtimeClient }: Props) {
  const [document, setDocument] = useState<GlobalAgentsDocument>();
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string>();
  const [conflict, setConflict] = useState(false);
  const byteCount = useMemo(() => new TextEncoder().encode(draft).byteLength, [draft]);

  const load = useCallback(async () => {
    setLoading(true);
    setMessage(undefined);
    try {
      if (!runtimeClient.getGlobalAgentsDocument) {
        throw new Error("当前 Desktop bridge 不支持全局指令。");
      }
      const next = await runtimeClient.getGlobalAgentsDocument();
      setDocument(next);
      setDraft(next.content);
      setConflict(false);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法读取全局 AGENTS.md。");
    } finally {
      setLoading(false);
    }
  }, [runtimeClient]);

  useEffect(() => {
    void load();
  }, [load]);

  async function save(): Promise<void> {
    if (!document) return;
    setSaving(true);
    setMessage(undefined);
    try {
      if (!runtimeClient.saveGlobalAgentsDocument) {
        throw new Error("当前 Desktop bridge 不支持保存全局指令。");
      }
      const next = await runtimeClient.saveGlobalAgentsDocument({
        expectedRevision: document.revision,
        content: draft,
      });
      setDocument(next);
      setDraft(next.content);
      setConflict(false);
      setMessage("已保存，下次运行生效。");
    } catch (error) {
      const text = error instanceof Error ? error.message : "保存失败。";
      const revisionConflict = text.includes("INSTRUCTION_REVISION_CONFLICT");
      setConflict(revisionConflict);
      setMessage(
        revisionConflict
          ? "文件已在其他位置修改。当前草稿仍保留，请先重新加载再决定如何合并。"
          : text,
      );
    } finally {
      setSaving(false);
    }
  }

  async function remove(): Promise<void> {
    if (!document?.exists) return;
    setSaving(true);
    setMessage(undefined);
    try {
      if (!runtimeClient.deleteGlobalAgentsDocument) {
        throw new Error("当前 Desktop bridge 不支持删除全局指令。");
      }
      const next = await runtimeClient.deleteGlobalAgentsDocument({
        expectedRevision: document.revision,
      });
      setDocument(next);
      setDraft("");
      setConflict(false);
      setMessage("已删除，下次运行不再加载。");
    } catch (error) {
      const text = error instanceof Error ? error.message : "删除失败。";
      const revisionConflict = text.includes("INSTRUCTION_REVISION_CONFLICT");
      setConflict(revisionConflict);
      setMessage(revisionConflict ? "文件已在其他位置修改，请重新加载后再删除。" : text);
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="aw-agents-settings aw-agents-settings--loading" role="status">
        正在读取全局规则…
      </div>
    );
  }

  return (
    <section className="aw-agents-settings" aria-labelledby="aw-agents-settings-title">
      <div className="aw-agents-settings__card">
        <header className="aw-agents-settings__header">
          <div className="aw-agents-settings__identity">
            <span className="aw-agents-settings__icon" aria-hidden="true">
              <FileText size={20} />
            </span>
            <h2 id="aw-agents-settings-title">AGENTS.md</h2>
            <span className={`aw-agents-settings__state${document?.exists ? " is-active" : ""}`}>
              {document?.exists ? "已启用" : "未创建"}
            </span>
          </div>
        </header>

        <div className="aw-agents-settings__editor">
          <div className="aw-agents-settings__editor-head">
            <div className="aw-agents-settings__editor-title">
              <label htmlFor="aw-global-agents-content">Markdown</label>
            </div>
            <span className={byteCount > (document?.maxBytes ?? 16_384) ? "is-over" : undefined}>
              {byteCount.toLocaleString()} / {(document?.maxBytes ?? 16_384).toLocaleString()} 字节
            </span>
          </div>
          <textarea
            id="aw-global-agents-content"
            aria-label="全局 AGENTS.md 内容"
            spellCheck={false}
            value={draft}
            onChange={(event) => setDraft(event.currentTarget.value)}
            placeholder="# 全局规则"
          />
        </div>

        <footer>
          <div
            className={`aw-agents-settings__footer-note${message ? " has-message" : ""}${conflict ? " is-error" : ""}`}
            role={message ? (conflict ? "alert" : "status") : undefined}
            aria-live={message ? "polite" : undefined}
          >
            {message ? (
              conflict ? <AlertTriangle size={15} aria-hidden="true" /> : <CircleCheck size={15} aria-hidden="true" />
            ) : null}
            <span>{message ?? "冲突时项目规则优先 · 仅影响新任务 · 请勿写入密钥"}</span>
          </div>
          <div className="aw-agents-settings__actions">
            {conflict ? (
              <button type="button" onClick={() => void load()}>
                <RefreshCw size={15} />
                重新加载
              </button>
            ) : null}
            <button
              className="is-danger"
              type="button"
              disabled={!document?.exists || saving}
              onClick={() => void remove()}
            >
              <Trash2 size={15} />
              删除
            </button>
            <button
              className="is-primary"
              type="button"
              disabled={!document || saving || draft === document.content}
              onClick={() => void save()}
            >
              <Save size={15} />
              保存
            </button>
          </div>
        </footer>
      </div>
    </section>
  );
}
