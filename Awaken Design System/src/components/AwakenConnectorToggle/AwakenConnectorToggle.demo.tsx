import { useState } from "react";
import { AwakenConnectorToggle } from "./AwakenConnectorToggle";
import { AwakenConnectorToggleStates } from "./AwakenConnectorToggle.states";

export function AwakenConnectorToggleDemo() {
  const [checked, setChecked] = useState<boolean>(AwakenConnectorToggleStates[0].checked);
  return (
    <section className="ads-component-demo-state" aria-label="AwakenConnectorToggle demo">
      <h3>AwakenConnectorToggle</h3>
      <div className="ads-component-demo-canvas">
        <AwakenConnectorToggle checked={checked} label="playwright" onCheckedChange={setChecked} />
        <AwakenConnectorToggle {...AwakenConnectorToggleStates[2]} />
      </div>
    </section>
  );
}
