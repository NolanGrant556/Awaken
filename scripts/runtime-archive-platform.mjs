export function runtimeArchivePlatform(platform = process.platform) {
  return platform === "win32" ? "windows" : platform;
}

export function runtimeOfficeExecutableRelativePath(platform = process.platform) {
  if (platform === "darwin") {
    return "Contents/MacOS/soffice";
  }
  if (platform === "win32") {
    return "program/soffice.com";
  }
  return "program/soffice";
}

function powershellLiteral(value) {
  return `'${String(value).replaceAll("'", "''")}'`;
}

export function createRuntimeArchiveInvocation(
  platform,
  sourceRoot,
  archivePath,
) {
  if (platform === "darwin") {
    return {
      command: "ditto",
      args: [
        "-c",
        "-k",
        "--sequesterRsrc",
        "--keepParent",
        sourceRoot,
        archivePath,
      ],
    };
  }
  if (platform === "win32") {
    return {
      command: "powershell.exe",
      args: [
        "-NoLogo",
        "-NoProfile",
        "-NonInteractive",
        "-Command",
        [
          `$source = ${powershellLiteral(sourceRoot)}`,
          `$destination = ${powershellLiteral(archivePath)}`,
          "$hidden = @(Get-ChildItem -LiteralPath $source -Recurse -Force | Where-Object { $_.Attributes -band [IO.FileAttributes]::Hidden })",
          "if ($hidden.Count -gt 0) { throw 'managed runtime contains hidden entries that Compress-Archive would omit' }",
          "Compress-Archive -LiteralPath $source -DestinationPath $destination -CompressionLevel Optimal -Force",
        ].join("; "),
      ],
    };
  }
  throw new Error(`runtime archive packaging is unsupported on ${platform}`);
}

export function extractRuntimeArchiveInvocation(
  platform,
  archivePath,
  destinationRoot,
) {
  if (platform === "darwin") {
    return {
      command: "ditto",
      args: ["-x", "-k", archivePath, destinationRoot],
    };
  }
  if (platform === "win32") {
    return {
      command: "powershell.exe",
      args: [
        "-NoLogo",
        "-NoProfile",
        "-NonInteractive",
        "-Command",
        [
          `$archive = ${powershellLiteral(archivePath)}`,
          `$destination = ${powershellLiteral(destinationRoot)}`,
          "Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force",
        ].join("; "),
      ],
    };
  }
  throw new Error(`runtime archive installation is unsupported on ${platform}`);
}

export function runtimeArchiveIntegrityManifest() {
  return {
    algorithm: "sha256",
    scope: "archive",
    trustBoundary: "packaged-host",
  };
}

export function hasRuntimeArchiveIntegrity(manifest) {
  return (
    manifest?.integrity?.algorithm === "sha256" &&
    manifest?.integrity?.scope === "archive" &&
    manifest?.integrity?.trustBoundary === "packaged-host"
  );
}
