import { describe, expect, it } from "vitest";
import {
  openProjectEntryWithApplication,
  projectEntryOpenBundleIds,
  projectEntryOpenWindowsExecutableNames,
  windowsRegistryExecutablePath,
} from "./project-entry-actions.js";

describe("project entry application mapping", () => {
  it("maps supported productivity and development applications to fixed bundle ids", () => {
    expect(projectEntryOpenBundleIds("wps", "/tmp/report.docx", "file")).toEqual([
      "com.kingsoft.wpsoffice.mac",
    ]);
    expect(projectEntryOpenBundleIds("office", "/tmp/report.docx", "file")).toEqual([
      "com.microsoft.Word",
    ]);
    expect(projectEntryOpenBundleIds("office", "/tmp/data.xlsx", "file")).toEqual([
      "com.microsoft.Excel",
    ]);
    expect(projectEntryOpenBundleIds("office", "/tmp/slides.pptx", "file")).toEqual([
      "com.microsoft.Powerpoint",
    ]);
    expect(projectEntryOpenBundleIds("cursor", "/tmp/project", "directory")).toEqual([
      "com.todesktop.230313mzl4w4u92",
    ]);
    expect(projectEntryOpenBundleIds("trae", "/tmp/project", "directory")).toEqual([
      "cn.trae.app",
      "cn.trae.solo.app",
    ]);
  });

  it("does not offer office applications for folders or unsupported file formats", () => {
    expect(projectEntryOpenBundleIds("wps", "/tmp/project", "directory")).toEqual([]);
    expect(projectEntryOpenBundleIds("office", "/tmp/project", "directory")).toEqual([]);
    expect(projectEntryOpenBundleIds("office", "/tmp/readme.md", "file")).toEqual([]);
  });

  it("maps Windows applications to fixed executable identities", () => {
    expect(
      projectEntryOpenWindowsExecutableNames(
        "office",
        String.raw`C:\work\report.docx`,
        "file",
      ),
    ).toEqual(["WINWORD.EXE"]);
    expect(
      projectEntryOpenWindowsExecutableNames(
        "office",
        String.raw`C:\work\data.xlsx`,
        "file",
      ),
    ).toEqual(["EXCEL.EXE"]);
    expect(
      projectEntryOpenWindowsExecutableNames(
        "office",
        String.raw`C:\work\slides.pptx`,
        "file",
      ),
    ).toEqual(["POWERPNT.EXE"]);
    expect(
      projectEntryOpenWindowsExecutableNames("wps", String.raw`C:\work\data.xlsx`, "file"),
    ).toEqual(["et.exe"]);
    expect(
      projectEntryOpenWindowsExecutableNames("wps", String.raw`C:\work\slides.pptx`, "file"),
    ).toEqual(["wpp.exe"]);
    expect(
      projectEntryOpenWindowsExecutableNames("cursor", String.raw`C:\work\project`, "directory"),
    ).toEqual(["Cursor.exe"]);
    expect(
      projectEntryOpenWindowsExecutableNames("trae", String.raw`C:\work\project`, "directory"),
    ).toEqual(["TRAE.exe", "Trae.exe"]);
    expect(
      projectEntryOpenWindowsExecutableNames("xcode", String.raw`C:\work\project`, "directory"),
    ).toEqual([]);
  });

  it("parses registered Windows executable paths without accepting trailing arguments", () => {
    expect(
      windowsRegistryExecutablePath(
        String.raw`    (Default)    REG_SZ    C:\Program Files\Cursor\Cursor.exe`,
      ),
    ).toBe(String.raw`C:\Program Files\Cursor\Cursor.exe`);
    expect(
      windowsRegistryExecutablePath(
        String.raw`    (Default)    REG_EXPAND_SZ    "%LOCALAPPDATA%\Programs\Microsoft VS Code\Code.exe" --reuse-window`,
        { LOCALAPPDATA: String.raw`C:\Users\Nolan\AppData\Local` },
      ),
    ).toBe(String.raw`C:\Users\Nolan\AppData\Local\Programs\Microsoft VS Code\Code.exe`);
  });

  it("opens a Windows entry with the resolved executable and an argument array", async () => {
    const calls: unknown[][] = [];
    await expect(
      openProjectEntryWithApplication(
        "cursor",
        String.raw`C:\work folder\Awaken`,
        "directory",
        "win32",
        {
          resolveWindowsExecutable: async (application, executableNames) => {
            calls.push(["resolve", application, executableNames]);
            return String.raw`C:\Program Files\Cursor\Cursor.exe`;
          },
          launchWindowsApplication: async (executablePath, absolutePath) => {
            calls.push(["launch", executablePath, absolutePath]);
            return true;
          },
        },
      ),
    ).resolves.toEqual({ ok: true });
    expect(calls).toEqual([
      ["resolve", "cursor", ["Cursor.exe"]],
      [
        "launch",
        String.raw`C:\Program Files\Cursor\Cursor.exe`,
        String.raw`C:\work folder\Awaken`,
      ],
    ]);
  });

  it("returns a stable application-specific prompt when the app is not installed", async () => {
    await expect(
      openProjectEntryWithApplication(
        "trae",
        String.raw`C:\work\Awaken`,
        "directory",
        "win32",
        { resolveWindowsExecutable: async () => undefined },
      ),
    ).resolves.toEqual({
      ok: false,
      code: "APPLICATION_UNAVAILABLE",
      message: "未找到 TRAE，请先安装后重试。",
    });
  });

  it("reports Xcode as unsupported on Windows", async () => {
    await expect(
      openProjectEntryWithApplication(
        "xcode",
        String.raw`C:\work\Awaken`,
        "directory",
        "win32",
      ),
    ).resolves.toEqual({
      ok: false,
      code: "UNSUPPORTED_APPLICATION",
      message: "Xcode 不支持 Windows。",
    });
  });

  it("reports a missing macOS app with its visible name", async () => {
    await expect(
      openProjectEntryWithApplication("cursor", "/tmp/Awaken", "directory", "darwin", {
        openMacApplication: async () => false,
      }),
    ).resolves.toEqual({
      ok: false,
      code: "APPLICATION_UNAVAILABLE",
      message: "未找到 Cursor，请先安装后重试。",
    });
  });
});
