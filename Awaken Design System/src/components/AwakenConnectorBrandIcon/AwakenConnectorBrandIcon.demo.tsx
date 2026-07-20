import { AwakenConnectorBrandIcon } from "./AwakenConnectorBrandIcon";
import { AwakenConnectorBrandIconStates } from "./AwakenConnectorBrandIcon.states";

export function AwakenConnectorBrandIconDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenConnectorBrandIcon demo">
      <h3>AwakenConnectorBrandIcon</h3>
      <div className="ads-component-demo-canvas" style={{ display: "flex", flexDirection: "row" }}>
        {AwakenConnectorBrandIconStates.map((state) => <AwakenConnectorBrandIcon key={state.providerId} {...state} />)}
      </div>
    </section>
  );
}
