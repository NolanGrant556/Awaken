import readline from "node:readline";

const input = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

function reply(id, result) {
  process.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", id, result })}\n`);
}

input.on("line", (line) => {
  let message;
  try {
    message = JSON.parse(line);
  } catch {
    return;
  }
  if (message.id === undefined || message.id === null) return;

  if (message.method === "initialize") {
    reply(message.id, {
      protocolVersion: message.params?.protocolVersion ?? "2025-06-18",
      capabilities: { tools: {} },
      serverInfo: { name: "awaken-auth-acceptance", version: "1.0.0" },
    });
    return;
  }
  if (message.method === "tools/list") {
    reply(message.id, {
      tools: [{ name: "credential_ready", description: "Reports whether the OAuth token reached only this process", inputSchema: { type: "object" } }],
    });
    return;
  }
  if (message.method === "tools/call") {
    const ready = process.env.ACCEPTANCE_OAUTH_TOKEN === "local-access-token";
    reply(message.id, { content: [{ type: "text", text: ready ? "credential-ready" : "credential-missing" }] });
    return;
  }
  reply(message.id, {});
});
