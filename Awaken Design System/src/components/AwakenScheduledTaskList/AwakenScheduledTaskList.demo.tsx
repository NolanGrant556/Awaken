import { AwakenScheduledTaskList } from "./AwakenScheduledTaskList";
import { AwakenScheduledTaskListStates } from "./AwakenScheduledTaskList.states";

export function AwakenScheduledTaskListDemo() {
  return (
    <section className="ads-component-demo-state" aria-label="AwakenScheduledTaskList demo">
      <h3>AwakenScheduledTaskList</h3>
      {AwakenScheduledTaskListStates.map((state) => (
        <div className="ads-component-demo-canvas" key={state.id} style={{ display: "grid", gap: 8 }}>
          <p style={{ margin: 0, fontSize: 12, color: "var(--aw-text-tertiary)" }}>{state.caption}</p>
          <AwakenScheduledTaskList
            items={state.props.items ?? []}
            loading={state.props.loading}
            onCreate={() => undefined}
            onCreateFromTemplate={() => undefined}
            onToggleEnabled={() => undefined}
            onRunNow={() => undefined}
            onCancelRun={() => undefined}
            onShowHistory={() => undefined}
            onEdit={() => undefined}
            onDelete={() => undefined}
          />
        </div>
      ))}
    </section>
  );
}
