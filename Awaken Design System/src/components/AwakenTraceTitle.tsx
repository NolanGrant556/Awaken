import { splitAwakenTraceTitle } from "../splitAwakenTraceTitle";

export function AwakenTraceTitle({ title }: { title: string }) {
  const { verb, fact } = splitAwakenTraceTitle(title);
  return (
    <span className="aw-trace-title">
      <span className="aw-trace-verb">{verb}</span>
      {fact ? (
        <>
          {" "}
          <span className="aw-trace-fact">{fact}</span>
        </>
      ) : null}
    </span>
  );
}
