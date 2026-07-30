import { afterEach, describe, expect, it } from "vitest";
import fs from "node:fs/promises";
import fsSync from "node:fs";
import os from "node:os";
import path from "node:path";
import type { RuntimeThreadTranscript } from "../shared/runtime.js";
import {
  openRecordedArtifact,
  resolveArtifactMediaTarget,
  resolveArtifactTarget,
} from "./artifact-open.js";

function symlinksSupported(): boolean {
  const tmp = fsSync.mkdtempSync(
    path.join(os.tmpdir(), "awaken-artifact-open-symlink-probe-"),
  );
  const target = path.join(tmp, "target.txt");
  const link = path.join(tmp, "link.txt");
  fsSync.writeFileSync(target, "");
  try {
    fsSync.symlinkSync(target, link);
    return true;
  } catch {
    return false;
  } finally {
    fsSync.rmSync(tmp, { recursive: true, force: true });
  }
}

const itIfSymlinks = it.skipIf(!symlinksSupported());

const temporaryRoots: string[] = [];

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

async function fixture(options?: { role?: "deliverable" | "preview" | "support" }) {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-artifact-open-"));
  temporaryRoots.push(root);
  const output = path.join(root, "output");
  await fs.mkdir(output);
  const file = path.join(output, "交付报告.pdf");
  await fs.writeFile(file, "pdf");
  const transcript: RuntimeThreadTranscript = {
    thread: {
      threadId: "thread-1",
      label: "test",
      workspace: {
        workspaceRoot: root,
        writableRoots: [root],
        outputRoots: [output],
      },
      createdAt: new Date(0).toISOString(),
      updatedAt: new Date(0).toISOString(),
    },
    runs: [
      {
        run: {
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
          cancelRequested: false,
          createdAt: new Date(0).toISOString(),
          updatedAt: new Date(0).toISOString(),
        },
        events: [
          {
            type: "AWAKEN_ARTIFACT_CREATED",
            artifactId: "artifact-1",
            artifactPath: file,
            artifactTitle: "交付报告.pdf",
            artifactStatus: "ready",
            artifactRole: options?.role ?? "deliverable",
            artifactContractVersion: 2,
            artifactProducer: "artifact_export",
          },
        ],
      },
    ],
  };
  return { root, output, file, transcript };
}

describe("artifact open authorization", () => {
  it("opens a recorded ready deliverable inside output roots", async () => {
    const { file, transcript } = await fixture();
    let opened = "";
    const result = await openRecordedArtifact(
      transcript,
      { threadId: "thread-1", runId: "run-1", artifactId: "artifact-1" },
      async (target) => {
        opened = target;
        return "";
      },
    );

    expect(result).toEqual({ ok: true });
    expect(opened).toBe(await fs.realpath(file));
  });

  it("folds partial artifact updates before authorizing the durable record", async () => {
    const { file, transcript } = await fixture();
    transcript.runs[0].events = [
      {
        ...transcript.runs[0].events[0],
        artifactStatus: "pending",
      },
      {
        type: "AWAKEN_ARTIFACT_UPDATED",
        artifactId: "artifact-1",
        artifactStatus: "ready",
      },
    ];

    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toEqual({ ok: true, path: await fs.realpath(file) });
  });

  it("requires an explicit ready status after folding artifact history", async () => {
    const missing = await fixture();
    delete missing.transcript.runs[0].events[0].artifactStatus;
    await expect(
      resolveArtifactTarget(missing.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({
      ok: false,
      result: { code: "artifact_not_deliverable" },
    });

    const unknown = await fixture();
    unknown.transcript.runs[0].events[0].artifactStatus = "complete" as never;
    await expect(
      resolveArtifactTarget(unknown.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({
      ok: false,
      result: { code: "artifact_not_deliverable" },
    });
  });

  it("rejects preview artifacts", async () => {
    const { transcript } = await fixture({ role: "preview" });
    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toMatchObject({
      ok: false,
      result: { code: "artifact_not_deliverable" },
    });
  });

  itIfSymlinks("rejects a symlink that resolves outside output roots", async () => {
    const { root, output, transcript } = await fixture();
    const outside = path.join(root, "outside.pdf");
    await fs.writeFile(outside, "outside");
    const link = path.join(output, "linked.pdf");
    await fs.symlink(outside, link);
    transcript.runs[0].events[0].artifactPath = link;

    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toMatchObject({
      ok: false,
      result: { code: "outside_output_roots" },
    });
  });

  it("rejects relative traversal outside output roots", async () => {
    const { root, transcript } = await fixture();
    const outside = path.join(root, "outside.pdf");
    await fs.writeFile(outside, "outside");
    transcript.runs[0].events[0].artifactPath = "outside.pdf";

    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toMatchObject({
      ok: false,
      result: { code: "outside_output_roots" },
    });
  });

  itIfSymlinks("accepts an internal symlink only after resolving its target", async () => {
    const { output, file, transcript } = await fixture();
    const link = path.join(output, "internal-link.pdf");
    await fs.symlink(file, link);
    transcript.runs[0].events[0].artifactPath = link;

    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toEqual({ ok: true, path: await fs.realpath(file) });
  });

  itIfSymlinks("rejects a supported-looking symlink whose real target has another extension", async () => {
    const { output, transcript } = await fixture();
    const command = path.join(output, "payload.command");
    await fs.writeFile(command, "#!/bin/sh\necho unsafe\n");
    const link = path.join(output, "report.pdf");
    await fs.symlink(command, link);
    transcript.runs[0].events[0].artifactPath = link;

    const resolved = await resolveArtifactTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "artifact-1",
    });
    expect(resolved).toMatchObject({
      ok: false,
      result: { code: "unsupported_type" },
    });
  });

  it("rejects missing files, directories, unsupported types, and active runs", async () => {
    const missing = await fixture();
    await fs.rm(missing.file);
    await expect(
      resolveArtifactTarget(missing.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({ ok: false, result: { code: "file_not_found" } });

    const directory = await fixture();
    const directoryPath = path.join(directory.output, "folder.pdf");
    await fs.mkdir(directoryPath);
    directory.transcript.runs[0].events[0].artifactPath = directoryPath;
    await expect(
      resolveArtifactTarget(directory.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({ ok: false, result: { code: "not_a_file" } });

    const unsupported = await fixture();
    const source = path.join(unsupported.output, "script.py");
    await fs.writeFile(source, "print('no')");
    unsupported.transcript.runs[0].events[0].artifactPath = source;
    await expect(
      resolveArtifactTarget(unsupported.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({ ok: false, result: { code: "unsupported_type" } });

    const active = await fixture();
    active.transcript.runs[0].run.status = "running";
    await expect(
      resolveArtifactTarget(active.transcript, {
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({ ok: false, result: { code: "run_not_terminal" } });
  });

  it("returns a domain error when no default application is configured", async () => {
    const { transcript } = await fixture();
    const result = await openRecordedArtifact(
      transcript,
      { threadId: "thread-1", runId: "run-1", artifactId: "artifact-1" },
      async () => "No application association exists for this file type",
    );
    expect(result).toEqual({
      ok: false,
      code: "no_default_application",
      message: "系统没有为该文件格式设置默认打开应用。",
    });
  });

  it("authorizes ready media during an active run without exposing arbitrary files", async () => {
    const { output, transcript } = await fixture();
    const image = path.join(output, "generated.png");
    await fs.writeFile(image, Buffer.from([0x89, 0x50, 0x4e, 0x47]));
    transcript.runs[0].run.status = "running";
    transcript.runs[0].events = [{
      type: "AWAKEN_ARTIFACT_CREATED",
      artifactId: "media-1",
      artifactPath: image,
      artifactTitle: "generated.png",
      artifactKind: "image",
      artifactStatus: "ready",
      artifactRole: "deliverable",
      artifactContractVersion: 2,
      artifactProducer: "image_generate",
    }];

    await expect(resolveArtifactMediaTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "media-1",
    })).resolves.toMatchObject({
      ok: true,
      path: await fs.realpath(image),
      mimeType: "image/png",
      byteLength: 4,
    });

    transcript.runs[0].events[0].artifactKind = "document";
    await expect(resolveArtifactMediaTarget(transcript, {
      threadId: "thread-1",
      runId: "run-1",
      artifactId: "media-1",
    })).resolves.toMatchObject({ ok: false, result: { code: "unsupported_type" } });
  });
});
