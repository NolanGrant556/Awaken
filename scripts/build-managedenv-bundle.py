#!/usr/bin/env python3
"""Build an immutable native dependency bundle outside the repository.

Maintainer-only: accepts an already verified upstream distribution, copies its
licenses/resources, checks executable architecture, and probes the relocated
copy. End users only download the resulting hashed archive through Core.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile

REPOSITORY = Path(__file__).resolve().parents[1]
TARGETS = {"darwin-arm64", "darwin-x64", "windows-x64"}


def run(argv, **kwargs):
    return subprocess.check_output(argv, text=True, stderr=subprocess.STDOUT, **kwargs).strip()


def contained(root, path):
    return path == root or root in path.parents


def macho(path):
    with path.open("rb") as stream:
        return stream.read(4) in (b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf", b"\xca\xfe\xba\xbe", b"\xbe\xba\xfe\xca")


def relocate_dylibs(root):
    """Copy non-system absolute dylibs, then resolve them relative to each image."""
    libraries = root / "awaken-libs"
    pending = [p for p in root.rglob("*") if p.is_file() and not p.is_symlink() and macho(p)]
    processed = set()
    copied = {}
    while pending:
        binary = pending.pop()
        if binary in processed:
            continue
        processed.add(binary)
        dependencies = run(["otool", "-L", str(binary)]).splitlines()[1:]
        for line in dependencies:
            dependency = line.strip().split(" (", 1)[0]
            if not dependency.startswith("/") or dependency.startswith(("/System/Library/", "/usr/lib/")):
                continue
            source = Path(dependency).resolve()
            if contained(root, source):
                destination = source
            else:
                if not source.is_file():
                    raise ValueError(f"missing dynamic library: {dependency}")
                # Content prefix avoids collisions between equal library basenames.
                digest = hashlib.sha256(source.read_bytes()).hexdigest()[:12]
                destination = libraries / f"{digest}-{source.name}"
                libraries.mkdir(exist_ok=True)
                if destination not in copied:
                    shutil.copy2(source, destination)
                    copied[destination] = dependency
                    pending.append(destination)
            relative = os.path.relpath(destination, binary.parent)
            run(["install_name_tool", "-change", dependency, f"@loader_path/{relative}", str(binary)])
        if binary in copied:
            run(["install_name_tool", "-id", f"@rpath/{binary.name}", str(binary)])
    for binary in sorted(processed):
        run(["codesign", "--force", "--sign", "-", str(binary)])
    return {str(p.relative_to(root)): source for p, source in copied.items()}


def validate_tree(root, executables, target):
    for path in root.rglob("*"):
        if path.is_symlink() and not contained(root, path.resolve()):
            raise ValueError(f"out-of-bundle symlink: {path}")
    for relative in executables.values():
        path = root / relative
        if not path.is_file() or not contained(root, path.resolve()):
            raise ValueError(f"missing or escaping executable: {relative}")
        if target.startswith("darwin"):
            arch = "arm64" if target.endswith("arm64") else "x86_64"
            if arch not in run(["lipo", "-archs", str(path)]).split():
                raise ValueError(f"wrong architecture: {relative}")
            commands = run(["otool", "-l", str(path)])
            versions = re.findall(r"\bminos\s+([0-9.]+)", commands)
            versions += re.findall(r"LC_VERSION_MIN_MACOSX\s+cmdsize\s+\d+\s+version\s+([0-9.]+)", commands)
            if not versions or any(tuple((list(map(int, version.split('.')))+[0, 0])[:2]) > (12, 0) for version in versions):
                raise ValueError(f"{relative}: requires a newer macOS than the bundled Electron minimum (12.0)")
        else:
            data = path.read_bytes()
            offset = int.from_bytes(data[60:64], "little")
            if data[:2] != b"MZ" or data[offset:offset + 6] != b"PE\x00\x00\x64\x86":
                raise ValueError(f"not a Windows x64 PE executable: {relative}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--component", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--source-url", required=True)
    parser.add_argument("--license", required=True)
    parser.add_argument("--license-file", type=Path, action="append", required=True)
    parser.add_argument("--target", choices=sorted(TARGETS), required=True)
    parser.add_argument("--executable", action="append", required=True, help="logical-name=relative/path")
    parser.add_argument("--resource", action="append", default=[], help="resource-name=relative/path")
    parser.add_argument("--probe", required=True)
    parser.add_argument("--probe-arg", action="append", default=[])
    parser.add_argument("--release", required=True, help="immutable deps-* release tag")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--relocate-dylibs", action="store_true")
    parser.add_argument("--skip-native-probe", action="store_true", help="cross-build manifest checks only; records that execution remains unverified")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", args.component) or not re.fullmatch(r"[0-9][0-9A-Za-z.-]*", args.version):
        parser.error("component and version must be safe immutable identifiers")
    if not args.release.startswith("deps-") or "/" in args.release or ".." in args.release:
        parser.error("use a separate immutable deps-* release tag")
    source, output = args.source_root.resolve(), args.output.resolve()
    if not source.is_dir() or contained(REPOSITORY, output) or contained(source, output):
        parser.error("output must be outside the repository and upstream source")
    host = f"{'windows' if os.name == 'nt' else platform.system().lower()}-{'arm64' if platform.machine() in ('arm64', 'aarch64') else 'x64'}"
    if host != args.target and not args.skip_native_probe:
        parser.error("native bundle probes must run on the target architecture")
    executables = dict(item.split("=", 1) for item in args.executable)
    resources = dict(item.split("=", 1) for item in args.resource)
    if args.probe not in executables:
        parser.error("probe must name a declared executable")
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="awaken-bundle-", dir=output) as temporary:
        root = Path(temporary) / "payload"
        shutil.copytree(source, root, symlinks=True)
        licenses = root / "awaken-licenses"
        licenses.mkdir(exist_ok=True)
        for index, path in enumerate(args.license_file):
            shutil.copy2(path, licenses / f"{index}-{path.name}")
        closure = relocate_dylibs(root) if args.relocate_dylibs else {}
        validate_tree(root, executables, args.target)
        for relative in resources.values():
            resource = (root / relative).resolve()
            if not contained(root, resource) or not resource.is_file() or resource.stat().st_size == 0:
                raise ValueError(f"missing or escaping resource: {relative}")
        pe_validation = None
        if args.target == "windows-x64":
            spec = importlib.util.spec_from_file_location("managedenv_pe", Path(__file__).with_name("check-managedenv-pe.py"))
            checker = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(checker)
            pe_validation = checker.check(root, list(executables.values()))
            if pe_validation["missing"]:
                raise ValueError(f"missing Windows runtime DLLs: {pe_validation['missing']}")
        environment = {**os.environ, "PATH": os.environ.get("SystemRoot", "C:/Windows") + "/System32" if os.name == "nt" else "/usr/bin:/bin"}
        probe = None if args.skip_native_probe else run([str(root / executables[args.probe]), *(args.probe_arg or ["--version"])], cwd=root, env=environment, timeout=60)
        if probe is not None and args.version not in probe:
            raise ValueError("relocated component did not report the pinned version")
        provenance = {"component": args.component, "version": args.version, "target": args.target, "source": args.source_url, "license": args.license, "dynamic_libraries": closure, "pe_validation": pe_validation, "probe_output": probe, "native_execution_verified": not args.skip_native_probe}
        (root / "awaken-provenance.json").write_text(json.dumps(provenance, indent=2) + "\n")
        name = f"{args.component}-{args.version}-{args.target}.tar.gz"
        archive = output / name
        if archive.exists():
            raise ValueError("immutable asset already exists; use a new resource version")
        with tarfile.open(archive, "w:gz") as bundle:
            for path in sorted(root.iterdir()):
                bundle.add(path, arcname=path.name, recursive=True)
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        asset = {"url": f"https://github.com/NolanGrant556/Awaken-Releases/releases/download/{args.release}/{name}", "sha256": digest, "format": "tar.gz", "executables": executables, "max_bytes": archive.stat().st_size}
        if resources: asset["resources"] = resources
        entry = {**provenance, "asset": asset, "probe": args.probe, "probe_args": args.probe_arg or ["--version"]}
        (output / f"{name}.json").write_text(json.dumps(entry, indent=2) + "\n")
        print(json.dumps({"asset": str(archive), "sha256": digest, "published": False}))


if __name__ == "__main__":
    main()
