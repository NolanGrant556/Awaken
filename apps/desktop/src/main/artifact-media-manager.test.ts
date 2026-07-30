import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import type { Session } from "electron";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeThreadTranscript } from "../shared/runtime.js";
import { ArtifactMediaManager } from "./artifact-media-manager.js";

const temporaryRoots: string[] = [];

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("artifact media manager", () => {
  it("returns an opaque source and serves byte ranges from the authorized artifact", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-artifact-media-"));
    temporaryRoots.push(root);
    const output = path.join(root, "output");
    await fs.mkdir(output);
    const file = path.join(output, "clip.mp4");
    await fs.writeFile(file, Buffer.from("0123456789"));
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-1",
        label: "media",
        workspace: { workspaceRoot: root, writableRoots: [root], outputRoots: [output] },
        createdAt: new Date(0).toISOString(),
        updatedAt: new Date(0).toISOString(),
      },
      runs: [{
        run: {
          runId: "run-1",
          threadId: "thread-1",
          status: "running",
          cancelRequested: false,
          createdAt: new Date(0).toISOString(),
          updatedAt: new Date(0).toISOString(),
        },
        events: [{
          type: "AWAKEN_ARTIFACT_CREATED",
          artifactId: "video-1",
          artifactPath: file,
          artifactTitle: "clip.mp4",
          artifactKind: "video",
          artifactStatus: "ready",
          artifactRole: "deliverable",
          artifactContractVersion: 2,
          artifactProducer: "video_generate",
        }],
      }],
    };
    let protocolHandler: ((request: Request) => Promise<Response>) | undefined;
    const defaultSession = {
      protocol: {
        handle: vi.fn((_scheme: string, handler: (request: Request) => Promise<Response>) => {
          protocolHandler = handler;
        }),
      },
    } as unknown as Session;
    const manager = new ArtifactMediaManager({
      defaultSession,
      getTranscript: async () => transcript,
    });
    manager.initialize();

    const source = await manager.source({
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "video-1",
    });
    expect(source).toMatchObject({ ok: true, mimeType: "video/mp4", byteLength: 10 });
    expect(defaultSession.protocol.handle).toHaveBeenCalledWith(
      "awaken-artifact",
      expect.any(Function),
    );
    if (!source.ok || !protocolHandler) throw new Error("media source was not initialized");

    const response = await protocolHandler(new Request(source.src, {
      headers: { Range: "bytes=2-5" },
    }));
    expect(response.status).toBe(206);
    expect(response.headers.get("Content-Range")).toBe("bytes 2-5/10");
    expect(Buffer.from(await response.arrayBuffer()).toString()).toBe("2345");

    await fs.writeFile(file, Buffer.from("changed"));
    const changedResponse = await protocolHandler(new Request(source.src));
    expect(changedResponse.status).toBe(410);
  });
});
