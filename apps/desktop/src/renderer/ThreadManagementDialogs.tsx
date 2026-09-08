import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import { useThreadNavigation } from "./useThreadNavigation";

type Props = {
  threadNavigation: Pick<
    ReturnType<typeof useThreadNavigation>,
    | "renameDialogOpen"
    | "renameDraft"
    | "setRenameDraft"
    | "renameSubmitting"
    | "renameInputRef"
    | "pendingThreadDelete"
    | "setPendingThreadDelete"
    | "threadDeleteSubmitting"
    | "handleConfirmRename"
    | "handleCancelRename"
    | "handleConfirmThreadDelete"
    | "pendingThreadDeleteCount"
    | "threadDeleteDialogTitle"
    | "threadDeleteDialogCopy"
  >;
};
export function ThreadManagementDialogs({ threadNavigation }: Props) {
  const {
    renameDialogOpen,
    renameDraft,
    setRenameDraft,
    renameSubmitting,
    renameInputRef,
    pendingThreadDelete,
    setPendingThreadDelete,
    threadDeleteSubmitting,
    handleConfirmRename,
    handleCancelRename,
    handleConfirmThreadDelete,
    pendingThreadDeleteCount,
    threadDeleteDialogTitle,
    threadDeleteDialogCopy,
  } = threadNavigation;
  return (
    <>
      <Dialog
        open={Boolean(pendingThreadDelete)}
        title={threadDeleteDialogTitle}
        onOpenChange={(open) => {
          if (!open && !threadDeleteSubmitting) {
            setPendingThreadDelete(null);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={threadDeleteSubmitting}
              onClick={() => setPendingThreadDelete(null)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={threadDeleteSubmitting}
              disabled={pendingThreadDeleteCount === 0}
              onClick={() => {
                void handleConfirmThreadDelete();
              }}
            >
              {pendingThreadDelete?.source === "batch"
                ? `删除 ${pendingThreadDeleteCount} 个对话`
                : "删除会话"}
            </Button>
          </>
        }
      >
        <p className="project-delete-dialog-copy">{threadDeleteDialogCopy}</p>
      </Dialog>
      <Dialog
        open={renameDialogOpen}
        title="重命名会话"
        onOpenChange={(open) => {
          if (!open) {
            handleCancelRename();
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={renameSubmitting}
              onClick={handleCancelRename}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={renameSubmitting}
              disabled={renameSubmitting || !renameDraft.trim()}
              onClick={() => {
                void handleConfirmRename();
              }}
            >
              确认
            </Button>
          </>
        }
      >
        <div className="rename-thread-dialog-body">
          <input
            ref={renameInputRef}
            type="text"
            className="rename-thread-dialog-input"
            value={renameDraft}
            onChange={(event) => setRenameDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void handleConfirmRename();
              }
              if (event.key === "Escape") {
                event.preventDefault();
                handleCancelRename();
              }
            }}
            placeholder="请输入会话标题"
            disabled={renameSubmitting}
          />
        </div>
      </Dialog>
    </>
  );
}
