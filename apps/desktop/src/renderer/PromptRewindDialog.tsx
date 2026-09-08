import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import { usePromptRewind } from "./usePromptRewind";

type Props = {
  promptReplacementPending: boolean;
  promptActions: Pick<
    ReturnType<typeof usePromptRewind>,
    | "pendingPromptRewind"
    | "dismissPromptRewindDialog"
    | "handleConfirmPromptRewind"
    | "promptRewindDialog"
    | "promptRewindOverlay"
  >;
};
export function PromptRewindDialog({
  promptActions,
  promptReplacementPending,
}: Props) {
  const {
    pendingPromptRewind,
    dismissPromptRewindDialog,
    handleConfirmPromptRewind,
    promptRewindDialog,
    promptRewindOverlay,
  } = promptActions;
  return (
    <Dialog
      open={promptRewindOverlay.open}
      title={promptRewindDialog?.title ?? "确认改写对话"}
      modal={false}
      backdropClassName={promptRewindOverlay.backdropClassName}
      onOpenChange={(open) => {
        if (!open && promptRewindOverlay.allowDismiss) {
          dismissPromptRewindDialog();
        }
      }}
      footer={
        <>
          <Button
            variant="default"
            size="md"
            onClick={() => dismissPromptRewindDialog()}
          >
            取消
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={
              pendingPromptRewind?.mode === "edit" && promptReplacementPending
            }
            onClick={() => {
              void handleConfirmPromptRewind();
            }}
          >
            {promptRewindDialog?.confirmLabel ?? "确认"}
          </Button>
        </>
      }
    >
      <div className="prompt-rewind-dialog-body">
        {(promptRewindDialog?.paragraphs ?? []).map((paragraph) => (
          <p key={paragraph} className="project-delete-dialog-copy">
            {paragraph}
          </p>
        ))}
      </div>
    </Dialog>
  );
}
