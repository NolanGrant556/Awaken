const { app, BrowserWindow } = require("electron");

const targetUrl = process.env.AWAKEN_MERMAID_E2E_URL;
const userDataDir = process.env.AWAKEN_MERMAID_E2E_USER_DATA;

function fail(message) {
  console.error(message);
  process.exit(2);
}

if (!targetUrl || !userDataDir) {
  fail("Mermaid Electron fixture requires a target URL and an isolated userData directory.");
}

const parsedTarget = new URL(targetUrl);
if (parsedTarget.protocol !== "http:" || parsedTarget.hostname !== "127.0.0.1") {
  fail(`Mermaid Electron fixture only loads loopback HTTP targets, received: ${targetUrl}`);
}

app.setName("Awaken Mermaid Electron Regression");
app.setPath("userData", userDataDir);

let fixtureWindow = null;

process.on("message", (message) => {
  if (message?.type === "awaken-mermaid-quit") app.quit();
});

app.whenReady().then(async () => {
  fixtureWindow = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 1024,
    minHeight: 720,
    show: true,
    title: "Awaken Mermaid Electron Regression",
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  fixtureWindow.webContents.on("render-process-gone", (_event, details) => {
    console.error(`Mermaid Electron fixture renderer exited: ${details.reason}`);
    app.exit(3);
  });

  await fixtureWindow.loadURL(targetUrl);
});

app.on("window-all-closed", () => {
  app.quit();
});

app.on("before-quit", () => {
  fixtureWindow = null;
});
