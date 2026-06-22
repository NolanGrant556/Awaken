import "./AwakenThoughtTrace.css";
type ThoughtTraceStatus = "active" | "done" | "error";

export type AwakenThoughtTraceProps = {
  text: string;
  status?: ThoughtTraceStatus;
  label?: string;
  durationLabel?: string;
  defaultOpen?: boolean;
};

export function AwakenThoughtTrace({
  text,
  status = "active",
  label = "Thought",
  durationLabel,
  defaultOpen = true,
}: AwakenThoughtTraceProps) {
  const heading = durationLabel ? `${label} ${durationLabel}` : label;

  return (
    <details className={`thought-trace thought-trace-${status}`} open={defaultOpen}>
      <summary className="thought-trace-heading">
        <span>{heading}</span>
      </summary>
      <p>{text}</p>
    </details>
  );
}
