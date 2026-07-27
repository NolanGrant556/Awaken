import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenScheduledTaskTemplateDialog } from "./AwakenScheduledTaskTemplateDialog";
import { AwakenScheduledTaskTemplateDialogState } from "./AwakenScheduledTaskTemplateDialog.states";

export function AwakenScheduledTaskTemplateDialogDemo() {
  const [open, setOpen] = useState(false);
  return (
    <section className="ads-component-demo-state" aria-label="AwakenScheduledTaskTemplateDialog demo">
      <h3>AwakenScheduledTaskTemplateDialog</h3>
      <div className="ads-component-demo-canvas">
        <AwakenButton variant="default" size="md" onClick={() => setOpen(true)}>
          打开模板窗口（循环任务 / 日程任务两分类）
        </AwakenButton>
        <AwakenScheduledTaskTemplateDialog
          open={open}
          title={AwakenScheduledTaskTemplateDialogState.title}
          description={AwakenScheduledTaskTemplateDialogState.description}
          categories={AwakenScheduledTaskTemplateDialogState.categories}
          onOpenChange={setOpen}
          onSelect={() => setOpen(false)}
        />
      </div>
    </section>
  );
}
