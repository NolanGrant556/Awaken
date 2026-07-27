import { AwakenScheduledTaskStateBadge } from "./AwakenScheduledTaskStateBadge";
import { AwakenScheduledTaskStateBadgeStates } from "./AwakenScheduledTaskStateBadge.states";

export function AwakenScheduledTaskStateBadgeDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenScheduledTaskStateBadge demo">
      <h3>AwakenScheduledTaskStateBadge</h3>
      <div
        className="ads-component-demo-canvas"
        style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}
      >
        {AwakenScheduledTaskStateBadgeStates.map((item) => (
          <AwakenScheduledTaskStateBadge key={item.id} state={item.state} label={item.label} />
        ))}
      </div>
    </section>
  );
}
