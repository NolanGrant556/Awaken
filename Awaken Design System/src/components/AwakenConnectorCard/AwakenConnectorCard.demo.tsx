import { AwakenConnectorCard } from "./AwakenConnectorCard";
import { AwakenConnectorCardStates } from "./AwakenConnectorCard.states";

export function AwakenConnectorCardDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenConnectorCard demo">
      <h3>AwakenConnectorCard</h3>
      <div className="ads-component-demo-canvas">
        {AwakenConnectorCardStates.map((item) => <AwakenConnectorCard key={`${item.variant}:${item.id}`} item={item} />)}
      </div>
    </section>
  );
}
