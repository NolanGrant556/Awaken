import { AwakenCliConnectorDetailDialog } from "./AwakenCliConnectorDetailDialog";
import { AwakenCliConnectorDetailDialogState } from "./AwakenCliConnectorDetailDialog.states";

export function AwakenCliConnectorDetailDialogDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenCliConnectorDetailDialog demo">
      <h3>AwakenCliConnectorDetailDialog</h3>
      <AwakenCliConnectorDetailDialog {...AwakenCliConnectorDetailDialogState} onOpenChange={() => undefined} onAction={() => undefined} />
    </section>
  );
}
