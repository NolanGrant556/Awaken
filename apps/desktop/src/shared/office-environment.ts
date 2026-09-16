// Settings DTOs contain status and opaque comparison tokens, never launch paths.
export type OfficeEnvironmentOperation = {
  operation_id: string;
  attempt_id: string;
  state: string;
  message: string;
  downloaded_bytes: number;
  total_bytes?: number;
  error_code?: string;
  updated_at: string;
  repair_of?: string;
};
export type OfficeEnvironmentStatus = {
  runtimes?: Array<{ id: "python" | "node"; state: "missing" | "ready" | "damaged"; version?: string }>;
  state: "missing" | "ready" | "damaged" | "update_available" | "unavailable";
  required_version: string;
  installed_version?: string;
  instance_id?: string;
  download_bytes: number;
  expanded_bytes: number;
  can_prepare: boolean;
  can_repair: boolean;
  error_code?: string;
  message: string;
  operation?: OfficeEnvironmentOperation;
};
export type OfficeEnvironmentAction = {
  requestId: string;
  action: "prepare" | "repair";
  expectedInstanceId?: string;
};
export type OfficeEnvironmentResult = { status: "ready" | "cancelled" | "failed"; message?: string };
export type OfficeEnvironmentProgress = { requestId: string; operation: OfficeEnvironmentOperation };
export type OfficeEnvironmentBridge = {
  getOfficeEnvironment: () => Promise<OfficeEnvironmentStatus>;
  prepareOfficeEnvironment: (request: OfficeEnvironmentAction) => Promise<OfficeEnvironmentResult>;
  cancelOfficeEnvironment: (requestId: string) => Promise<void>;
  onOfficeEnvironmentProgress: (callback: (event: OfficeEnvironmentProgress) => void) => () => void;
};
export const OFFICE_ENVIRONMENT_CHANNELS = {
  status: "awaken:office-environment-status",
  prepare: "awaken:office-environment-prepare",
  cancel: "awaken:office-environment-cancel",
  progress: "awaken:office-environment-progress",
} as const;

export function isOfficePreparationActive(operation?: OfficeEnvironmentOperation): boolean {
  return !!operation && !["ready", "failed", "cancelled", "interrupted"].includes(operation.state);
}
export function formatRuntimeBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "";
  return bytes >= 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GiB` : `${(bytes / 1024 ** 2).toFixed(1)} MiB`;
}
export function runtimeDownloadProgress(downloaded?: number, total?: number): string {
  if (!Number.isFinite(downloaded) || downloaded! < 0 || (!downloaded && !total)) return "";
  const value = formatRuntimeBytes(downloaded!);
  return total && Number.isFinite(total) && total > 0 ? `${value} / ${formatRuntimeBytes(total)}` : value;
}
