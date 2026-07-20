import { AwakenMcpSettingsDialog } from "./AwakenMcpSettingsDialog";
import { AwakenMcpSettingsDialogState } from "./AwakenMcpSettingsDialog.states";

export function AwakenMcpSettingsDialogDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenMcpSettingsDialog demo">
      <h3>AwakenMcpSettingsDialog</h3>
      <AwakenMcpSettingsDialog {...AwakenMcpSettingsDialogState} onOpenChange={() => undefined} onConfigValueChange={() => undefined} onDelete={() => undefined} onSave={() => undefined} />
    </section>
  );
}
