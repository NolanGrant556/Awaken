import { AwakenButton } from "../AwakenButton";
import "./AwakenOfficeEnvironment.css";

export type EnvironmentRow = {
  id: string;
  name: string;
  version?: string;
  state: "checking" | "ready" | "missing" | "damaged" | "unknown" | "working";
};
export type AwakenOfficeEnvironmentProps = {
  rows: EnvironmentRow[];
  busy: boolean;
  refreshing: boolean;
  canRepair: boolean;
  canCancel: boolean;
  progress?: string;
  percent?: number;
  message?: string;
  error?: boolean;
  onRepair: () => void;
  onCancel: () => void;
};
const labels = { checking: "检查中", ready: "正常", missing: "需要安装", damaged: "需要修复", unknown: "未能检查", working: "处理中" };

/** Pure presentation. Core owns health checks; Desktop owns requests. */
export function AwakenOfficeEnvironment(props: AwakenOfficeEnvironmentProps) {
  return <section className="office-environment" aria-label="环境管理">
    <ul className="office-environment-list" aria-label="运行环境">
      {props.rows.map(row => <li className="office-environment-row" key={row.id}>
        <span className="office-environment-name">{row.name}</span>
        <span className="office-environment-version">{row.version || "—"}</span>
        <span className="office-environment-status" data-state={row.state} role="status">
          {row.state === "ready" ? <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m3.5 8 3 3 6-6" /></svg> : <span className="office-environment-status-dot" aria-hidden="true" />}
          {labels[row.state]}
        </span>
      </li>)}
    </ul>
    {props.busy && <div className="office-environment-progress" role="status">
      <div className="office-environment-progress-label"><span>{props.progress || "正在准备环境…"}</span>{props.percent != null && <span>{Math.floor(props.percent)}%</span>}</div>
      <progress aria-label="环境准备进度" max={100} value={props.percent} />
    </div>}
    {props.message && <p className={props.error ? "office-environment-feedback office-environment-error" : "office-environment-feedback"} role={props.error ? "alert" : "status"}>{props.message}</p>}
    <div className="office-environment-actions">
      <AwakenButton disabled={props.canCancel ? false : props.busy || props.refreshing || !props.canRepair} onClick={props.canCancel ? props.onCancel : props.onRepair}>
        {props.canCancel ? "取消" : props.busy ? "修复中…" : "修复"}
      </AwakenButton>
    </div>
  </section>;
}
