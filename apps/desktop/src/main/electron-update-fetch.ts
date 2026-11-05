import type { ClientRequestConstructorOptions, ClientRequest } from "electron";

/** net.fetch(manual) cancels redirects; expose net.request's redirect event as a response. */
export function electronUpdateFetch(
  request: (options: ClientRequestConstructorOptions) => ClientRequest,
  url: string,
  init?: Parameters<typeof fetch>[1],
): Promise<Response> {
  return new Promise((resolve, reject) => {
    const req = request({ url, method: "GET", redirect: "manual", credentials: "omit", useSessionCookies: false });
    const headers = new Headers(init?.headers);
    headers.forEach((value, name) => req.setHeader(name, value));
    let settled = false; let ended = false;
    let body: ReadableStreamDefaultController<Uint8Array> | undefined;
    const cleanup = () => init?.signal?.removeEventListener("abort", abort);
    const fail = (error: unknown) => {
      if (!settled) { settled = true; reject(error); }
      else if (!ended && body) body.error(error);
      ended = true; cleanup();
    };
    const abort = () => { fail(init?.signal?.reason ?? new Error("update request aborted")); req.abort(); };
    req.on("error", fail);
    req.on("redirect", (status, _method, location) => {
      settled = true; ended = true; cleanup();
      resolve(new Response(null, { status, headers: { location } }));
      req.abort();
    });
    req.on("response", response => {
      const responseHeaders = new Headers();
      for (const [name, values] of Object.entries(response.headers)) {
        for (const value of Array.isArray(values) ? values : [values]) responseHeaders.append(name, value);
      }
      const stream = new ReadableStream<Uint8Array>({
        start(controller) {
          body = controller;
          response.on("end", () => { if (!ended) { ended = true; cleanup(); controller.close(); } });
          response.on("error", fail);
          response.on("aborted", () => fail(new Error("update response aborted")));
          response.on("data", chunk => {
            if (ended) return;
            // Electron IncomingMessage does not expose pause/resume. Bound queued bytes.
            if ((controller.desiredSize ?? 0) < -32 * 1024 * 1024) {
              fail(new Error("update download buffer limit")); req.abort(); return;
            }
            controller.enqueue(new Uint8Array(chunk));
          });
        },
        cancel() { ended = true; cleanup(); req.abort(); },
      }, { highWaterMark: 64 * 1024, size: chunk => chunk.byteLength });
      settled = true;
      resolve(new Response(stream, { status: response.statusCode, headers: responseHeaders }));
    });
    if (init?.signal?.aborted) { abort(); return; }
    init?.signal?.addEventListener("abort", abort, { once: true });
    req.end();
  });
}
