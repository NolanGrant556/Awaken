import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenScheduledTaskEditorDialog } from "./AwakenScheduledTaskEditorDialog";
import {
  AwakenScheduledTaskEditorDemoProfiles,
  AwakenScheduledTaskEditorDemoProjects,
  AwakenScheduledTaskEditorDemoTimezones,
  AwakenScheduledTaskEditorDialogStates,
} from "./AwakenScheduledTaskEditorDialog.states";

export function AwakenScheduledTaskEditorDialogDemo() {
  const [openStateId, setOpenStateId] = useState<string | null>(null);
  return (
    <section className="ads-component-demo-state" aria-label="AwakenScheduledTaskEditorDialog demo">
      <h3>AwakenScheduledTaskEditorDialog</h3>
      <div className="ads-component-demo-canvas" style={{ display: "grid", gap: 8, justifyItems: "start" }}>
        {AwakenScheduledTaskEditorDialogStates.map((state) => (
          <AwakenButton key={state.id} variant="default" size="md" onClick={() => setOpenStateId(state.id)}>
            打开：{state.caption}
          </AwakenButton>
        ))}
        {AwakenScheduledTaskEditorDialogStates.map((state) => (
          <AwakenScheduledTaskEditorDialog
            key={state.id}
            open={openStateId === state.id}
            mode={state.mode}
            value={state.value}
            projects={AwakenScheduledTaskEditorDemoProjects}
            profiles={AwakenScheduledTaskEditorDemoProfiles}
            timezones={AwakenScheduledTaskEditorDemoTimezones}
            systemTimezone="Asia/Shanghai"
            errorMessage={state.errorMessage ?? null}
            fieldErrors={state.fieldErrors}
            onOpenChange={(open) => setOpenStateId(open ? state.id : null)}
            onValueChange={() => undefined}
            onSubmit={() => setOpenStateId(null)}
          />
        ))}
      </div>
    </section>
  );
}
