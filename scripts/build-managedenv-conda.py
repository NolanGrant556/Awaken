#!/usr/bin/env python3
"""Package pinned conda-forge native binaries for Core-owned preparation.

Micromamba is a maintainer tool only. The resulting archive carries original
prefix-bearing files plus a bounded relocation manifest; Core needs no conda
installation and performs final-path relocation before publishing the instance.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
PLATFORMS = {"darwin-arm64": "osx-arm64", "darwin-x64": "osx-64", "windows-x64": "win-64"}
VERSIONS = {"poppler": "26.09.0", "ffmpeg": "9.0.1", "tesseract": "5.5.3"}
ENTRIES = {"poppler": ("pdftoppm", ["-v"], ["pdftoppm", "pdfinfo", "pdftotext"]), "ffmpeg": ("ffmpeg", ["-version"], ["ffmpeg", "ffprobe"]), "tesseract": ("tesseract", ["--version"], ["tesseract"])}


def contained(root, path):
    return path == root or root in path.parents


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", choices=PLATFORMS, required=True)
    parser.add_argument("--component", choices=VERSIONS, default="poppler")
    parser.add_argument("--prefix", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--release", required=True)
    parser.add_argument("--lock", type=Path, required=True)
    args = parser.parse_args()
    prefix = args.prefix.resolve()
    version = VERSIONS[args.component]
    probe, probe_args, executables = ENTRIES[args.component]
    if contained(ROOT, prefix):
        parser.error("builder prefix must be outside the project")
    metadata = [json.loads(p.read_text()) for p in sorted((prefix / "conda-meta").glob("*.json"))]
    package = next(record for record in metadata if record["name"] == args.component)
    if package["version"] != version or package["subdir"] != PLATFORMS[args.target]:
        raise ValueError("wrong native component version or builder platform")
    locked = [{k: record[k] for k in ("name", "version", "build", "subdir", "url", "sha256", "license")} for record in metadata]
    if args.lock.exists():
        expected = {record["name"]: record for record in json.loads(args.lock.read_text())["packages"]}
        for record in locked:
            pinned = expected.get(record["name"])
            if pinned and record["url"].startswith("file:"):
                if any(record[k] != pinned[k] for k in ("version", "build", "subdir", "sha256")):
                    raise ValueError("local package does not match the pinned graph")
                record["url"] = pinned["url"]
    # A repeat build must use the recorded graph, not solve different packages.
    if args.lock.exists() and json.loads(args.lock.read_text())["packages"] != locked:
        raise ValueError("dependency graph changed; explicitly review a new source lock")
    args.lock.parent.mkdir(parents=True, exist_ok=True)
    args.lock.write_text(json.dumps({"component": args.component, "version": version, "target": args.target, "packages": locked}, indent=2) + "\n")
    staging = prefix.with_name(prefix.name + "-bundle")
    if staging.exists():
        raise ValueError("bundle staging exists; choose a fresh builder prefix")
    shutil.copytree(prefix, staging, symlinks=True, ignore=shutil.ignore_patterns("conda-meta"))
    licenses = staging / "awaken-licenses"
    licenses.mkdir(exist_ok=True)
    fixups = []
    for record in metadata:
        cache = Path(record["link"]["source"]).resolve()
        source_licenses = cache / "info/licenses"
        if source_licenses.exists():
            shutil.copytree(source_licenses, licenses / record["name"], symlinks=False)
        paths_file = cache / "info/paths.json"
        if not paths_file.exists():
            continue
        for entry in json.loads(paths_file.read_text())["paths"]:
            placeholder = entry.get("prefix_placeholder")
            if not placeholder:
                continue
            relative = entry["_path"]
            source, destination = (cache / relative).resolve(), (staging / relative).resolve()
            if not contained(cache, source) or not contained(staging, destination):
                raise ValueError("package prefix entry escapes its root")
            data = source.read_bytes()
            if hashlib.sha256(data).hexdigest() != entry["sha256"]:
                raise ValueError(f"source file changed: {relative}")
            shutil.copy2(source, destination)
            fixups.append({"path": relative, "placeholder": placeholder, "mode": entry["file_mode"]})
    (staging / "awaken-prefixes.json").write_text(json.dumps(fixups, indent=2) + "\n")
    (staging / "awaken-source-lock.json").write_text(args.lock.read_text())
    # The selected LGPL FFmpeg variant enables version3; preserve upstream
    # package metadata in the source lock but describe the actual binary terms.
    license_id = "LGPL-3.0-or-later" if args.component == "ffmpeg" else package["license"]
    primary_license = licenses / "ffmpeg/COPYING.LGPLv3" if args.component == "ffmpeg" else next(p for p in (licenses / args.component).rglob("*") if p.is_file())
    native_target = f"{'windows' if os.name == 'nt' else platform.system().lower()}-{'arm64' if platform.machine() in ('arm64', 'aarch64') else 'x64'}"
    directory, extension = ("Library/bin", ".exe") if args.target.startswith("windows") else ("bin", "")
    command = [sys.executable, "-B", str(ROOT / "scripts/build-managedenv-bundle.py"), "--component", args.component, "--version", version, "--source-root", str(staging), "--source-url", f"https://github.com/conda-forge/{args.component}-feedstock", "--license", license_id, "--license-file", str(primary_license), "--target", args.target, "--probe", probe, *("--probe-arg="+arg for arg in probe_args), "--release", args.release, "--output", str(args.output)]
    for executable in executables:
        command += ["--executable", f"{executable}={directory}/{executable}{extension}"]
    if args.component == "tesseract":
        retained = {"eng", "chi_sim", "chi_tra", "osd"}
        for file in (staging / "share/tessdata").glob("*.traineddata"):
            if file.stem not in retained: file.unlink()
        for language in sorted(retained):
            command += ["--resource", f"ocr_{language}=share/tessdata/{language}.traineddata"]
    if args.target != native_target:
        command.append("--skip-native-probe")
    subprocess.run(command, check=True)


if __name__ == "__main__":
    main()
