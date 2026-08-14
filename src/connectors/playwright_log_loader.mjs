const targetSuffix = "/node_modules/@executeautomation/playwright-mcp-server/dist/index.js";
const upstreamExpression = "filePath: `${process.env.HOME || '/tmp'}/playwright-mcp-server.log`,";
const awakenExpression = "filePath: process.env.AWAKEN_PLAYWRIGHT_MCP_LOG_PATH || `${process.env.HOME || '/tmp'}/playwright-mcp-server.log`,";

export async function load(url, context, nextLoad) {
  const loaded = await nextLoad(url, context);
  if (loaded.format !== "module" || !url.includes(targetSuffix)) {
    return loaded;
  }

  const source = typeof loaded.source === "string"
    ? loaded.source
    : Buffer.from(loaded.source).toString("utf8");
  if (!source.includes(upstreamExpression)) {
    throw new Error("Awaken cannot route the Playwright MCP log because the upstream logger contract changed");
  }
  return {
    ...loaded,
    source: source.replace(upstreamExpression, awakenExpression),
  };
}
