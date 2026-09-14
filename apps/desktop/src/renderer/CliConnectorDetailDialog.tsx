import {
  AwakenCliConnectorDetailDialog,
  awakenCliConnectorDialogAction,
  type AwakenCliConnectorAction,
} from "@awaken/design-system";
import type {
  CliAuthSessionView,
  CliConnectorReadiness,
  CliInstallProgress,
  RuntimeConnector,
} from "../shared/runtime";
import { awakenPluginBrandAsset } from "@awaken/design-system";
const dingtalkLogo = awakenPluginBrandAsset("dingtalk");
const feishuLogo = awakenPluginBrandAsset("feishu");
import { cliConnectorProductCopy, cliConnectorUserMessage } from "./cliConnectorProduct";

export type CliConnectorDialogAction = AwakenCliConnectorAction;

export function cliConnectorDialogAction(
  readiness: CliConnectorReadiness,
  authSession?: CliAuthSessionView,
): CliConnectorDialogAction {
  return awakenCliConnectorDialogAction(
    {
      install: readiness.install,
      executable: readiness.executable,
      authentication: readiness.authentication,
      account: readiness.account,
      safeErrorMessage: cliConnectorUserMessage(readiness.safeError?.message),
    },
    authSession ? { phase: authSession.phase } : undefined,
  );
}

type CliConnectorDetailDialogProps = {
  open: boolean;
  connector: RuntimeConnector | null;
  readiness: CliConnectorReadiness | null;
  authSession?: CliAuthSessionView;
  installProgress?: CliInstallProgress;
  loadingProfile?: boolean;
  busy?: boolean;
  onOpenChange: (open: boolean) => void;
  onAction: (action: Exclude<CliConnectorDialogAction, "unsupported">) => void;
};

export function CliConnectorDetailDialog({
  open,
  connector,
  readiness,
  authSession,
  installProgress,
  loadingProfile,
  busy,
  onOpenChange,
  onAction,
}: CliConnectorDetailDialogProps) {
  if (!connector) return null;
  const product = cliConnectorProductCopy(connector.id);
  return (
    <AwakenCliConnectorDetailDialog
      open={open}
      providerId={connector.id}
      displayName={product?.displayName || connector.displayName}
      description={product?.description || connector.description?.trim() || "连接外部服务"}
      readiness={readiness ? {
        install: readiness.install,
        executable: readiness.executable,
        authentication: readiness.authentication,
        account: readiness.account,
        safeErrorMessage: cliConnectorUserMessage(readiness.safeError?.message),
      } : null}
      authSession={authSession ? { phase: authSession.phase } : undefined}
      installProgress={installProgress ? { percent: installProgress.percent, label: cliConnectorUserMessage(installProgress.label) ?? "正在安装" } : undefined}
      brandLogoSrc={connector.id === "dingtalk-workspace" ? dingtalkLogo : connector.id === "lark-cli" ? feishuLogo : undefined}
      brandTone={connector.id === "dingtalk-workspace" ? "dingtalk" : connector.id === "lark-cli" ? "lark" : "neutral"}
      loadingProfile={loadingProfile}
      busy={busy}
      onOpenChange={onOpenChange}
      onAction={onAction}
    />
  );
}
