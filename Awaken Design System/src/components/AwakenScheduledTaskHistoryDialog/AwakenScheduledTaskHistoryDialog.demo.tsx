import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenScheduledTaskHistoryDialog } from "./AwakenScheduledTaskHistoryDialog";
import { AwakenScheduledTaskHistoryDialogStates } from "./AwakenScheduledTaskHistoryDialog.states";

export function AwakenScheduledTaskHistoryDialogDemo() {
  const [openStateId, setOpenStateId] = useState<string | null>(null);
  return (
    <section className="ads-component-demo-state" aria-label="AwakenScheduledTaskHistoryDialog demo">
      <h3>AwakenScheduledTaskHistoryDialog</h3>
      <div className="ads-component-demo-canvas" style={{ display: "grid", gap: 8, justifyItems: "start" }}>
        {AwakenScheduledTaskHistoryDialogStates.map((state) => (
          <AwakenButton key={state.id} variant="default" size="md" onClick={() => setOpenStateId(state.id)}>
            打开：{state.caption}
          </AwakenButton>
        ))}
        {AwakenScheduledTaskHistoryDialogStates.map((state) => (
          <AwakenScheduledTaskHistoryDialog
            key={state.id}
            open={openStateId === state.id}
            title={state.title}
            items={state.items}
            onOpenChange={(open) => setOpenStateId(open ? state.id : null)}
            onOpenThread={() => setOpenStateId(null)}
          />
        ))}
      </div>
    </section>
  );
}
