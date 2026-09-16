import { desktopErrorMessage } from "../desktopErrorMessage";
import { useCallback, useEffect, useRef, useState } from "react";
import { AwakenOfficeEnvironment } from "@awaken/design-system";
import { isOfficePreparationActive, runtimeDownloadProgress, type OfficeEnvironmentAction, type OfficeEnvironmentOperation, type OfficeEnvironmentStatus } from "../../shared/office-environment";
import type { RuntimeClient } from "../runtime/client";

const runtimeNames = { python: "Python 环境", node: "Node.js 环境" } as const;
export function OfficeEnvironmentPanel({ runtimeClient }: { runtimeClient: RuntimeClient }) {
  const [snapshot, setSnapshot] = useState<OfficeEnvironmentStatus>();
  const [operation, setOperation] = useState<OfficeEnvironmentOperation>();
  const [request, setRequest] = useState<OfficeEnvironmentAction>();
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const [refreshing, setRefreshing] = useState(true);
  const live = useRef(false);
  const current = useRef<OfficeEnvironmentAction | undefined>(undefined);
  const generation = useRef(0);
  const refresh = useCallback(async () => {
    const revision = ++generation.current;
    setRefreshing(true);
    try {
      const next = await runtimeClient.getOfficeEnvironment();
      if (!live.current || revision !== generation.current) return;
      setSnapshot(next);
      if (!current.current) setOperation(next.operation);
    } catch (error) {
      if (live.current && revision === generation.current) { setSnapshot(undefined); setFailed(true); setMessage(error instanceof Error ? error.message : "无法读取环境状态。"); }
    } finally { if (live.current && revision === generation.current) setRefreshing(false); }
  }, [runtimeClient]);
  useEffect(() => {
    live.current = true;
    const unsubscribe = runtimeClient.onOfficeEnvironmentProgress((event) => {
      if (live.current && event.requestId === current.current?.requestId) setOperation(event.operation);
    });
    void refresh();
    return () => {
      live.current = false;
      generation.current++;
      unsubscribe();
      const pending = current.current;
      current.current = undefined;
      if (pending) void runtimeClient.cancelOfficeEnvironment(pending.requestId).catch(() => {});
    };
  }, [runtimeClient, refresh]);
  // Observe preparation started by a task. Reading/replaying status never installs.
  useEffect(() => {
    if (request || !isOfficePreparationActive(operation)) return;
    const timer = setTimeout(() => void refresh(), 3000);
    return () => clearTimeout(timer);
  }, [request, operation, refresh]);
  const start = async (action: "prepare" | "repair") => {
    if (current.current) return;
    const next: OfficeEnvironmentAction = { requestId: crypto.randomUUID(), action, ...(action === "repair" ? { expectedInstanceId: snapshot?.instance_id } : {}) };
    current.current = next;
    generation.current++;
    setRefreshing(false);
    setRequest(next); setOperation(undefined); setMessage(""); setFailed(false);
    try {
      const result = await runtimeClient.prepareOfficeEnvironment(next);
      if (!live.current || current.current?.requestId !== next.requestId) return;
      setFailed(result.status === "failed");
      setMessage(result.status === "ready" ? "" : result.status === "cancelled" ? "已取消等待。" : result.message ?? "安装失败，请重试。");
    } catch (error) {
      if (live.current && current.current?.requestId === next.requestId) { setFailed(true); setMessage(error instanceof Error ? error.message : "办公环境准备失败，可以重试。"); }
    } finally {
      if (live.current && current.current?.requestId === next.requestId) {
        current.current = undefined; setRequest(undefined); await refresh();
      }
    }
  };
  const active = isOfficePreparationActive(operation);
  const bytes = active ? runtimeDownloadProgress(operation?.downloaded_bytes, operation?.total_bytes) : "";
  const historicalFailure = !message && !request && operation && ["failed", "interrupted"].includes(operation.state);
  const errorMessage = historicalFailure ? operation.message : message;
  const busy = !!request || active;
  const rows = (["python", "node"] as const).map(id => {
    const runtime = snapshot?.runtimes?.find(row => row.id === id);
    return { id, name: runtimeNames[id], version: runtime?.version,
      state: refreshing ? "checking" as const : busy ? "working" as const : runtime?.state ?? "unknown" as const };
  });
  const needsRepair = snapshot?.state === "damaged" || rows.some(row => row.state === "damaged");
  const needsPrepare = snapshot?.state === "missing" || snapshot?.state === "update_available" || rows.some(row => row.state === "missing");
  const incompleteStatus = !!snapshot && rows.some(row => row.state === "unknown");
  const notice = errorMessage || (snapshot?.error_code ? snapshot.message : incompleteStatus ? "环境状态暂不可用，请重启 Awaken 后重试。" : snapshot?.state === "damaged" && rows.every(row => row.state === "ready") ? "共享组件需要修复。" : snapshot?.state === "update_available" ? "运行环境需要更新。" : "");
  return <AwakenOfficeEnvironment
    rows={rows}
    busy={busy} refreshing={refreshing}
    progress={[operation?.message, bytes].filter(Boolean).join(" · ")}
    percent={active && operation?.state === "downloading" && operation.total_bytes && operation.total_bytes > 0 ? Math.min(100, operation.downloaded_bytes / operation.total_bytes * 100) : undefined}
    message={notice ? desktopErrorMessage(notice) : undefined} error={failed || !!historicalFailure}
    canRepair={!snapshot || (!incompleteStatus && !!snapshot.can_prepare && (needsPrepare || needsRepair))} canCancel={!!request}
    onRepair={() => {
      if (!snapshot) { setMessage(""); setFailed(false); void refresh(); }
      else void start(needsRepair && snapshot.instance_id ? "repair" : "prepare");
    }}
    onCancel={() => { if (current.current) void runtimeClient.cancelOfficeEnvironment(current.current.requestId).catch((error: unknown) => { if (live.current) { setFailed(true); setMessage(error instanceof Error ? error.message : "取消失败，请重试。"); } }); }}
  />;
}
