#!/usr/bin/env python3
"""Maintainer-only macOS Git build; compiler/cache/output stay outside projects."""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
VERSION = "2.55.0"
URL = f"https://www.kernel.org/pub/software/scm/git/git-{VERSION}.tar.xz"
SHA256 = "457fdb04dc8728e007d4688695e6912e6f680727920f2a40bf11eacc17505357"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--release", required=True)
    parser.add_argument("--target", choices=["darwin-arm64", "darwin-x64"])
    args = parser.parse_args()
    arch = "arm64" if platform.machine() == "arm64" else "x64"
    target = args.target or f"darwin-{arch}"
    work = args.work.resolve()
    if platform.system() != "Darwin" or work == ROOT or ROOT in work.parents:
        parser.error("use a macOS builder and work directory outside the repository")
    work.mkdir(parents=True, exist_ok=True)
    archive = work / f"git-{VERSION}.tar.xz"
    if not archive.exists():
        with urllib.request.urlopen(URL, timeout=90) as response, archive.open("wb") as output:
            while data := response.read(1024 * 1024):
                output.write(data)
    if hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
        raise ValueError("Git source checksum mismatch")
    source = work / f"git-{VERSION}"
    if not source.exists():
        with tarfile.open(archive) as bundle:
            bundle.extractall(work, filter="data")
    prefix = work / "git-install"
    # Runtime prefix support and Apple's system curl avoid references to a
    # maintainer's Homebrew keg. HTTPS remains available via git-remote-https.
    flags = [f"prefix={prefix}", "RUNTIME_PREFIX=YesPlease", "NO_GETTEXT=YesPlease", "NO_TCLTK=YesPlease", "NO_PERL=YesPlease", "NO_PYTHON=YesPlease", "NO_OPENSSL=YesPlease", "NO_RUST=YesPlease", "CFLAGS=-O2 -mmacosx-version-min=12.0", "LDFLAGS=-mmacosx-version-min=12.0"]
    environment = {**os.environ, "PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(work), "MACOSX_DEPLOYMENT_TARGET": "12.0"}
    if target != f"darwin-{arch}":
        flags.append("CC=clang -arch " + ("x86_64" if target.endswith("x64") else "arm64"))
    subprocess.run(["make", "-j4", *flags, "all"], cwd=source, env=environment, check=True)
    subprocess.run(["make", *flags, "install"], cwd=source, env=environment, check=True)
    subprocess.run([sys.executable, str(ROOT / "scripts/build-managedenv-bundle.py"), "--component", "git", "--version", VERSION, "--source-root", str(prefix), "--source-url", URL, "--license", "GPL-2.0-only", "--license-file", str(source / "COPYING"), "--target", target, "--executable", "git=bin/git", "--executable", "git-remote-https=libexec/git-core/git-remote-https", "--probe", "git", "--release", args.release, "--output", str(args.output), *(["--skip-native-probe"] if target != f"darwin-{arch}" else [])], check=True)


if __name__ == "__main__":
    main()
