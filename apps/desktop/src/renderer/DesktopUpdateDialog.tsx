import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import { useDesktopUpdate } from "./useDesktopUpdate";

type Props = {
  desktopUpdate: Pick<
    ReturnType<typeof useDesktopUpdate>,
    | "desktopUpdateSnapshot"
    | "updateRestartDialogOpen"
    | "setUpdateRestartDialogOpen"
    | "setUpdateInstallBlockedCode"
    | "updateInstallStarting"
    | "updateInstallBlockedNotice"
    | "updateInstallRetryBlocked"
    | "handleConfirmUpdateRestart"
  >;
};
export function DesktopUpdateDialog({ desktopUpdate }: Props) {
  const {
    desktopUpdateSnapshot,
    updateRestartDialogOpen,
    setUpdateRestartDialogOpen,
    setUpdateInstallBlockedCode,
    updateInstallStarting,
    updateInstallBlockedNotice,
    updateInstallRetryBlocked,
    handleConfirmUpdateRestart,
  } = desktopUpdate;
  return (
    <Dialog
      open={updateRestartDialogOpen}
      title="重启以完成更新"
      onOpenChange={(open) => {
        if (!open && !updateInstallStarting) {
          // “稍后”只关闭确认框；Main 仍保持 downloaded，侧栏保留重启入口。
          setUpdateRestartDialogOpen(false);
          setUpdateInstallBlockedCode(undefined);
        }
      }}
      footer={
        <>
          <Button
            variant="default"
            size="md"
            disabled={updateInstallStarting}
            onClick={() => {
              setUpdateRestartDialogOpen(false);
              setUpdateInstallBlockedCode(undefined);
            }}
          >
            稍后
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={updateInstallStarting}
            disabled={updateInstallStarting || updateInstallRetryBlocked}
            onClick={() => {
              void handleConfirmUpdateRestart();
            }}
          >
            立即重启
          </Button>
        </>
      }
    >
      <div className="desktop-update-dialog-body">
        <p>
          当前版本 v{desktopUpdateSnapshot?.currentVersion ?? "--"}，将更新到 v
          {desktopUpdateSnapshot?.availableVersion ?? "--"}。
        </p>
        {desktopUpdateSnapshot?.releaseName ? (
          <p>{desktopUpdateSnapshot.releaseName}</p>
        ) : null}
        {updateInstallBlockedNotice ? (
          <p role="alert">{updateInstallBlockedNotice}</p>
        ) : null}
      </div>
    </Dialog>
  );
}
