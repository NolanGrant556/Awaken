#!/usr/bin/env python3
"""Produce native resource artifacts on a macOS arm64 maintainer/CI host.

All downloads, package caches, compilers and staging directories stay outside
the repository. End-user preparation needs none of these build tools. Foreign
targets receive architecture/import checks and explicitly no execution claim.
"""
import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
LOCKS = ROOT / "src/managedenv/assets/native-locks"
TARGETS = {"darwin-arm64": "osx-arm64", "darwin-x64": "osx-64", "windows-x64": "win-64"}
LO_VERSION = "26.2.2.2"
LO_SOURCES = {
    "darwin-arm64": ("mac/aarch64/LibreOffice_26.2.2.2_MacOS_aarch64.dmg", "2a603303b0a7a17c2f6dd381d2039ca79fecc73cb7c25a13ebea15cf0919c751"),
    "darwin-x64": ("mac/x86_64/LibreOffice_26.2.2.2_MacOS_x86-64.dmg", "3fd198d2af8d715a8f7def300f8882422332022babb96f97a4d59f99d07370c2"),
    "windows-x64": ("win/x86_64/LibreOffice_26.2.2.2_Win_x86-64.msi", "fa83774e81dae12bf26747b26e5a90d05f5e40330347f016c681fc5e019acadf"),
}
MAMBA_URL = "https://api.anaconda.org/download/conda-forge/micromamba/2.9.0/osx-arm64/micromamba-2.9.0-0.tar.bz2"
MAMBA_SHA = "500f5074feb8d02c4296ef9921c3650ed2874171805a9fbb8fbb53896433646b"


def run(argv, **kwargs):
    return subprocess.check_output([str(x) for x in argv], **kwargs)


def download(url, sha, destination):
    destination.parent.mkdir(parents=True, exist_ok=True)
    if not destination.exists():
        temporary = destination.with_suffix(destination.suffix + ".partial")
        with urllib.request.urlopen(url, timeout=120) as response, temporary.open("wb") as output:
            shutil.copyfileobj(response, output, 1024*1024)
        with temporary.open("rb") as stream:
            if hashlib.file_digest(stream, "sha256").hexdigest() != sha:
                raise ValueError(f"download checksum mismatch: {destination.name}")
        temporary.rename(destination)
    with destination.open("rb") as stream:
        if hashlib.file_digest(stream, "sha256").hexdigest() != sha:
            raise ValueError(f"cached checksum mismatch: {destination.name}")
    return destination


def environment(mamba, work, name, target, lock):
    prefix = work/name
    if prefix.exists(): raise ValueError(f"choose a fresh build directory: {prefix}")
    packages = json.loads(lock.read_text())["packages"]
    local = [download(p["url"], p["sha256"], work/"downloads"/target/Path(p["url"]).name) for p in packages]
    explicit = work/(name+"-explicit.txt")
    explicit.write_text("@EXPLICIT\n" + "\n".join(p.as_uri() for p in local) + "\n")
    run([mamba, "create", "--no-rc", "--offline", "--yes", "--prefix", prefix, "--platform", TARGETS[target], "--file", explicit], env={**os.environ, "MAMBA_ROOT_PREFIX": str(work/"mamba")})
    if name == "builder-tools":
        # Prefix relocation invalidates ad-hoc Mach-O signatures. Sign derived
        # maintainer files after breaking package-cache hardlinks.
        for path in prefix.rglob("*"):
            if not path.is_file() or path.is_symlink(): continue
            with path.open("rb") as stream: magic = stream.read(4)
            if magic not in (b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf"): continue
            if path.stat().st_nlink > 1:
                temporary = path.with_name(path.name + ".signing")
                shutil.copy2(path, temporary); temporary.replace(path)
            run(["codesign", "--force", "--sign", "-", path], stderr=subprocess.STDOUT)
    return prefix


def libreoffice(target, work, output, release, tools):
    relative, sha = LO_SOURCES[target]
    url = f"https://downloadarchive.documentfoundation.org/libreoffice/old/{LO_VERSION}/{relative}"
    archive = download(url, sha, work/"downloads"/Path(relative).name)
    with tempfile.TemporaryDirectory(prefix="libreoffice-"+target+"-", dir=work) as temporary:
        staging = Path(temporary)
        mount = None
        if target.startswith("darwin"):
            mount = staging/"mount"
            mount.mkdir()
            run(["hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, archive])
            source, license_file = mount/"LibreOffice.app", mount/"LibreOffice.app/Contents/Resources/LICENSE"
            entries = ["soffice=Contents/MacOS/soffice"]
        else:
            extract, cabinet = staging/"windows", staging/"cabinet"
            extract.mkdir(); cabinet.mkdir()
            run([tools/"bin/msiextract", "-C", extract, archive])
            source = extract/"LibreOffice:."
            license_file = source/"license.txt"
            # MSI contains x86/amd64/arm64 files with the same destination names.
            # Extract cabinet IDs separately, then choose only amd64 table entries.
            table = run([tools/"bin/msiinfo", "export", archive, "File"]).decode().splitlines()
            rows = list(csv.reader(table, delimiter="\t"))[3:]
            cabfile = staging/"libreoffice1.cab"
            with cabfile.open("wb") as stream:
                subprocess.run([str(tools/"bin/msiinfo"), "extract", str(archive), "libreoffice1.cab"], stdout=stream, check=True)
            run([tools/"bin/gcab", "-x", "-C", cabinet, cabfile])
            for row in rows:
                if ".dll_amd64." not in row[0]: continue
                file = cabinet/row[0]
                data = file.read_bytes()
                pe, = struct.unpack_from("<I", data, 60)
                if struct.unpack_from("<H", data, pe+4)[0] != 0x8664:
                    raise ValueError("MSI runtime architecture mismatch")
                shutil.copy2(file, source/"program"/row[2].split("|")[-1])
            entries = ["soffice=program/soffice.exe", "soffice-bin=program/soffice.bin"]
        try:
            command = [sys.executable, "-B", ROOT/"scripts/build-managedenv-bundle.py", "--component", "libreoffice", "--version", LO_VERSION, "--source-root", source, "--source-url", url, "--license", "MPL-2.0", "--license-file", license_file, "--target", target, "--probe", "soffice", "--release", release, "--output", output]
            for entry in entries: command += ["--executable", entry]
            if target != "darwin-arm64": command += ["--skip-native-probe"]
            run(command)
        finally:
            if mount: run(["hdiutil", "detach", mount])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--release", required=True)
    parser.add_argument("--target", choices=[*TARGETS, "all"], default="all")
    parser.add_argument("--component", choices=["poppler", "libreoffice", "git", "ffmpeg", "tesseract", "all"], default="all")
    args = parser.parse_args()
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        parser.error("this cross-platform producer requires a macOS arm64 maintainer host")
    work, output = args.work.resolve(), args.output.resolve()
    if any(p == ROOT or ROOT in p.parents for p in (work, output)):
        parser.error("build work/output must stay outside the repository")
    work.mkdir(parents=True, exist_ok=True)
    mamba = work/"bootstrap/bin/micromamba"
    if not mamba.exists():
        archive = download(MAMBA_URL, MAMBA_SHA, work/"downloads/micromamba.tar.bz2")
        with tarfile.open(archive) as bundle: bundle.extractall(work/"bootstrap", filter="data")
    tools = None
    targets = TARGETS if args.target == "all" else [args.target]
    components = ["poppler", "libreoffice", "git", "ffmpeg", "tesseract"] if args.component == "all" else [args.component]
    for target in targets:
        for component in components:
            print(f"Building {component}/{target}", flush=True)
            if component in ("poppler", "ffmpeg", "tesseract"):
                lock = LOCKS/f"{component}-{target}.json"
                prefix = environment(mamba, work, f"{component}-{target}", target, lock)
                run([sys.executable, "-B", ROOT/"scripts/build-managedenv-conda.py", "--component", component, "--target", target, "--prefix", prefix, "--output", output, "--release", args.release, "--lock", lock])
            elif component == "libreoffice":
                if target == "windows-x64" and tools is None:
                    tools = environment(mamba, work, "builder-tools", "darwin-arm64", LOCKS/"builder-tools-darwin-arm64.json")
                libreoffice(target, work, output, args.release, tools)
            elif target.startswith("darwin"):
                run([sys.executable, "-B", ROOT/"scripts/build-managedenv-git.py", "--target", target, "--work", work/f"git-{target}", "--output", output, "--release", args.release])
    print("Native artifacts built; publication and Windows execution are separate actions.")


if __name__ == "__main__": main()
