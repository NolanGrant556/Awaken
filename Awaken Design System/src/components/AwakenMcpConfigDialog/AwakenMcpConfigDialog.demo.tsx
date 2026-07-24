import { AwakenMcpConfigDialog } from "./AwakenMcpConfigDialog";
import { AwakenMcpConfigDialogState } from "./AwakenMcpConfigDialog.states";

export function AwakenMcpConfigDialogDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenMcpConfigDialog demo">
      <h3>AwakenMcpConfigDialog</h3>
      <AwakenMcpConfigDialog {...AwakenMcpConfigDialogState} onOpenChange={() => undefined} onModeChange={() => undefined} onValueChange={() => undefined} onSubmit={() => undefined} />
    </section>
  );
}
