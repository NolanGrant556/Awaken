type ThoughtTraceStatus = "active" | "done" | "error";

type ThoughtTraceProps = {
  text: string;
  status?: ThoughtTraceStatus;
  label?: string;
  durationLabel?: string;
  defaultOpen?: boolean;
};

export function ThoughtTrace({
  text,
  status = "active",
  label = "Thought",
  durationLabel,
  defaultOpen = true,
}: ThoughtTraceProps) {
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
