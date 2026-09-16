#!/usr/bin/env python3
"""Assemble pinned, relocatable Office bundles in one maintainer work root.

Uses existing native resource descriptors and exact Python/npm locks. No end
user package manager is needed. Assembly is not native or Desktop acceptance.
"""
import argparse
import hashlib
import importlib.util
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import struct
import sys
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = {"darwin-arm64": "aarch64-apple-darwin", "darwin-x64": "x86_64-apple-darwin", "windows-x64": "x86_64-pc-windows-msvc"}
spec = importlib.util.spec_from_file_location("native_resources", ROOT / "scripts/build-managedenv-resources.py")
native = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native)


def read(path):
    return json.loads(path.read_text())


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def unpack(archive, asset, destination):
    with tempfile.TemporaryDirectory(prefix="extract-", dir=destination.parent) as temporary:
        temporary = Path(temporary)
        if asset["format"] == "zip":
            with zipfile.ZipFile(archive) as bundle:
                for name in bundle.namelist():
                    if Path(name).is_absolute() or ".." in Path(name).parts or "\\" in name:
                        raise ValueError("escaping archive member")
                bundle.extractall(temporary)
        else:
            with tarfile.open(archive) as bundle:
                bundle.extractall(temporary, filter="data")
        source = temporary / asset.get("strip_prefix", "")
        if not source.is_dir(): raise ValueError("missing archive prefix")
        for path in source.rglob("*"):
            if path.is_symlink() and not path.resolve().is_relative_to(source.resolve()):
                raise ValueError(f"escaping symlink: {path}")
        shutil.copytree(source, destination, symlinks=True)


def fetch_component(catalog, component, target, work, destination):
    asset = catalog["components"][component]["assets"][target]
    archive = native.download(asset["url"], asset["sha256"], work / "downloads/by-hash" / asset["sha256"])
    unpack(archive, asset, destination)
    return asset


def language_tools(catalog, work):
    result = {}
    for component in ("python", "node", "uv"):
        destination = work / "toolchains" / component
        destination.parent.mkdir(parents=True, exist_ok=True)
        asset = catalog["components"][component]["assets"]["darwin-arm64"]
        if not destination.exists(): fetch_component(catalog, component, "darwin-arm64", work, destination)
        result[component] = destination / asset["executables"][component]
        output = subprocess.check_output([result[component], "--version"], text=True)
        if catalog["components"][component]["version"] not in output: raise ValueError("wrong build interpreter")
        if component == "node": result["npm"] = destination / asset["executables"]["npm"]
    return result


def audit_native_tree(root, target, minimum_os):
    spec = importlib.util.spec_from_file_location("bundle_checks", ROOT / "scripts/build-managedenv-bundle.py")
    checks = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(checks)
    pe = None
    managed_assemblies = []
    if target.startswith("windows"):
        spec = importlib.util.spec_from_file_location("pe_checks", ROOT / "scripts/check-managedenv-pe.py")
        pe = importlib.util.module_from_spec(spec); spec.loader.exec_module(pe)
    entries = {}
    for p in root.rglob("*"):
        if not p.is_file() or p.is_symlink(): continue
        with p.open("rb") as stream: header = stream.read(8)
        magic = header[:4]
        # Java classes share CAFEBABE with fat Mach-O, but their following
        # version fields are not a bounded architecture count.
        fat = magic in (b"\xca\xfe\xba\xbe", b"\xbe\xba\xfe\xca") and 0 < int.from_bytes(header[4:8], "big" if magic==b"\xca\xfe\xba\xbe" else "little") <= 16
        if (target.startswith("darwin") and (magic in (b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf") or fat)) or (target.startswith("windows") and magic[:2] == b"MZ"):
            relative = p.relative_to(root).as_posix()
            if pe and pe.is_anycpu_assembly(p):
                managed_assemblies.append(relative)
                continue
            entries[relative] = relative
    if target.startswith("darwin"):
        failures = []
        arch = "arm64" if target.endswith("arm64") else "x86_64"
        for relative in entries:
            binary = root / relative
            if arch not in checks.run(["lipo", "-archs", str(binary)]).split(): failures.append(relative + ": wrong architecture")
            commands = checks.run(["otool", "-l", str(binary)])
            versions = re.findall(r"\bminos\s+([0-9.]+)", commands) + re.findall(r"LC_VERSION_MIN_MACOSX\s+cmdsize\s+\d+\s+version\s+([0-9.]+)", commands)
            if not versions or any(tuple((list(map(int,v.split('.')))+[0,0])[:2])>tuple(map(int,minimum_os.split(".")[:2])) for v in versions): failures.append(relative + ": minimum macOS " + ",".join(versions))
        if failures: raise ValueError("incompatible native files:\n" + "\n".join(failures))
    else:
        checks.validate_tree(root, entries, target)
    if target.startswith("windows"):
        result = pe.check(root, list(entries))
        result["platform_neutral_clr_assemblies"] = managed_assemblies
        if result["missing"]: raise ValueError("missing bundled Windows imports: " + json.dumps(result["missing"]))
    else:
        result = {"architecture": target, "files_checked": len(entries), "minimum_macos": minimum_os, "native_execution_verified": False}
    return result


def assemble(catalog, locks, target, work, release, tools):
    office = catalog["office_bundle"]
    name = f"office-{office['version']}-{target}.tar.gz"
    output = work / "assets" / name
    if output.exists(): raise ValueError(f"immutable candidate exists: {output}")
    with tempfile.TemporaryDirectory(prefix=f"office-{target}-", dir=work) as temporary:
        root = Path(temporary) / "payload"
        dependencies = root / "dependencies"
        dependencies.mkdir(parents=True)
        entries, sources = {}, {}
        for component in ("python", "node", "typst", "fonts"):
            destination = dependencies / component
            asset = fetch_component(catalog, component, target, work, destination)
            sources[component] = {"version": catalog["components"][component]["version"], "license": catalog["components"][component]["license"], "asset": asset}
            for logical in ("python", "node", "typst"):
                if logical in asset["executables"]: entries[logical] = (destination / asset["executables"][logical]).relative_to(root).as_posix()
        # The basic bundle needs regular and bold SC; do not ship all seven weights.
        for font in (dependencies / "fonts").glob("*.otf"):
            if font.name not in ("NotoSansCJKsc-Regular.otf", "NotoSansCJKsc-Bold.otf"): font.unlink()
        fixups = []
        for component in ("libreoffice", "poppler"):
            version = catalog["components"][component]["version"]
            descriptor_path = work / "assets" / f"{component}-{version}-{target}.tar.gz.json"
            descriptor = read(descriptor_path)
            asset = descriptor["asset"]
            archive = descriptor_path.with_suffix("")
            if digest(archive) != asset["sha256"]: raise ValueError("native candidate checksum mismatch")
            destination = dependencies / "native" / component
            destination.parent.mkdir(parents=True, exist_ok=True)
            unpack(archive, asset, destination)
            sources[component] = descriptor
            for logical, relative in asset["executables"].items(): entries[logical] = (destination / relative).relative_to(root).as_posix()
            if component == "poppler":
                directory = Path(entries["pdftoppm"]).parent
                for logical in ['pdffonts', 'pdfimages', 'pdfseparate', 'pdfunite', 'pdftocairo', 'pdftohtml', 'pdftops', 'pdfattach', 'pdfdetach']:
                    relative = directory / (logical + (".exe" if target.startswith("windows") else ""))
                    if not (root / relative).is_file(): raise ValueError("missing bundled Poppler command: " + logical)
                    entries[logical] = relative.as_posix()
            prefixes = destination / "awaken-prefixes.json"
            if prefixes.exists():
                relative = destination.relative_to(root).as_posix()
                fixups += [{**f, "path": relative + "/" + f["path"], "root": relative} for f in read(prefixes)]
        write(root / "awaken-prefixes.json", fixups)
        lo = dependencies / "native/libreoffice"
        excluded = []
        if target.startswith("windows"):
            # This headless Office package does not provide TWAIN scanner UI.
            # Its optional 32-bit helper cannot share the x64 VC runtime DLLs.
            for relative in ("program/twain32shim.exe", "program/spsupp_x86.dll", "program/officebean.dll"):
                optional = lo / relative
                if optional.exists():
                    excluded.append(optional.relative_to(root).as_posix())
                    optional.unlink()
            # The .NET UNO bridge is a separate integration, unused by our
            # headless converters. Upstream includes a 32BITREQUIRED policy
            # assembly; omit the whole optional bridge instead of a broken half.
            for pattern in ("program/cli_*.dll", "program/cli_*.config", "program/policy.*.cli_*.dll"):
                for optional in lo.glob(pattern):
                    excluded.append(optional.relative_to(root).as_posix())
                    optional.unlink()
            # LibreOffice's internal pip/setuptools carries launcher templates
            # for other CPU architectures. Keep its x64 templates and Python.
            for pattern in ("program/python-core-*/lib/pip/_vendor/distlib/*.exe", "program/python-core-*/lib/setuptools/*.exe"):
                for template in lo.glob(pattern):
                    data = template.read_bytes()
                    if data[:2] != b"MZ": continue
                    pe = struct.unpack_from("<I", data, 60)[0]
                    if struct.unpack_from("<H", data, pe+4)[0] != 0x8664:
                        excluded.append(template.relative_to(root).as_posix())
                        template.unlink()
        lo_fonts = lo / ("Contents/Resources/fonts/truetype" if target.startswith("darwin") else "share/fonts/truetype")
        lo_fonts.mkdir(parents=True, exist_ok=True)
        for font in (dependencies / "fonts").glob("*.otf"): shutil.copy2(font, lo_fonts / font.name)
        python_lock = next(e["python"] for e in locks["entries"] if e["spec"].get("python") == office["spec"]["python"])
        node_lock = next(e for e in locks["entries"] if e["spec"].get("node") == office["spec"]["node"])
        (root / "requirements.txt").write_text(python_lock)
        packages = dependencies / "python" / ("Lib/site-packages" if target.startswith("windows") else "lib/python3.12/site-packages")
        # Keep the shipped bootstrap pip; install the already locked application
        # graph without solving or compiling native code.
        env = {**os.environ, "UV_CACHE_DIR": str(work / "uv-cache"), "UV_NO_CONFIG": "1", "UV_PYTHON_DOWNLOADS": "never", "npm_config_cache": str(work / "npm-cache"), "npm_config_userconfig": str(work / "empty-npmrc"), "npm_config_globalconfig": str(work / "empty-global-npmrc"), "PATH": str(tools["node"].parent) + os.pathsep + os.defpath}
        subprocess.run([tools["uv"], "pip", "install", "--python", tools["python"], "--target", packages, "--python-platform", TARGETS[target], "--python-version", "3.12", "--require-hashes", "--only-binary", ":all:", "--no-deps", "-r", root / "requirements.txt"], check=True, env=env)
        if target.startswith("windows"):
            # MarkItDown uses Magika's Python CPU API. Its separate native CLI
            # would add DirectML/GPU dependencies outside the Office contract.
            standalone = packages / "bin/magika.exe"
            if standalone.exists():
                excluded.append(standalone.relative_to(root).as_posix())
                standalone.unlink()
        # The fixed runtime does not install packages on end-user machines.
        # Omit pip's unused multi-architecture launchers from this distribution.
        for p in [packages / "pip", *packages.glob("pip-*.dist-info")]:
            if p.is_dir(): shutil.rmtree(p)
        node_packages = dependencies / "node-packages"
        node_packages.mkdir()
        (node_packages / "package.json").write_text(node_lock["package"])
        (node_packages / "package-lock.json").write_text(node_lock["node"])
        subprocess.run([tools["node"], tools["npm"], "ci", "--ignore-scripts", "--no-audit", "--no-fund"], cwd=node_packages, check=True, env=env)
        if list(node_packages.rglob("*.node")): raise ValueError("Node graph has native modules; target-specific installation is required")
        override, fallback = dependencies / "bin/override", dependencies / "bin/fallback"
        override.mkdir(parents=True); fallback.mkdir()
        extension = ".exe" if target.startswith("windows") else ""
        launcher = Path(temporary) / ("launcher" + extension)
        subprocess.run(["go", "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w", "-o", launcher, "./cmd/awaken-office-launcher"], cwd=ROOT, check=True, env={**os.environ,"GOOS": target.split("-")[0], "GOARCH": "arm64" if target.endswith("arm64") else "amd64", "CGO_ENABLED": "0"})
        for logical in ("python", "python3", "node", "soffice", "pdftoppm", "pdfinfo", "pdftotext", "pdffonts", "pdfimages", "pdfseparate", "pdfunite", "pdftocairo", "pdftohtml", "pdftops", "pdfattach", "pdfdetach", "typst"):
            shutil.copy2(launcher, override / (logical + extension))
        licenses = root / "licenses"
        licenses.mkdir()
        write(licenses / "sources.json", sources)
        write(licenses / "excluded-optional-files.json", excluded)
        write(licenses / "package-locks.json", {"python": python_lock, "node": json.loads(node_lock["node"])})
        # Preserve source license text in place and provide an auditable index.
        license_paths = [p.relative_to(root).as_posix() for p in dependencies.rglob("*") if p.is_file() and any(s in p.name.lower() for s in ("license", "copying", "copyright", "notice"))]
        if not license_paths: raise ValueError("license inventory is empty")
        write(licenses / "files.json", sorted(license_paths))
        write(root / "office-runtime.json", {"version": office["version"], "layout_version": office["layout_version"], "target": target, "executables": entries, "spec": office["spec"], "minimum_os": office["minimum_os"][target.split("-")[0]], "minimum_core_protocol": office["minimum_core_protocol"]})
        write(root / "native-audit.json", audit_native_tree(root, target, office["minimum_os"][target.split("-")[0]]))
        for p in root.rglob("*"):
            if p.is_symlink() and not p.resolve().is_relative_to(root): raise ValueError(f"escaping bundle symlink: {p}")
        expanded = sum(p.stat().st_size for p in root.rglob("*") if p.is_file())
        output.parent.mkdir(parents=True, exist_ok=True)
        temporary_archive = output.with_suffix(output.suffix + ".partial")
        try:
            with tarfile.open(temporary_archive, "w:gz", dereference=False) as archive:
                for path in sorted(root.iterdir()): archive.add(path, arcname=path.name)
            temporary_archive.rename(output)
        finally:
            temporary_archive.unlink(missing_ok=True)
        asset = {"url": f"https://github.com/NolanGrant556/Awaken-Releases/releases/download/{release}/{name}", "sha256": digest(output), "format": "tar.gz", "max_bytes": output.stat().st_size, "max_expanded_bytes": expanded + 1024*1024, "executables": {n: f"dependencies/bin/override/{n}{extension}" for n in ("python", "node", "soffice", "pdftoppm", "pdfinfo", "pdftotext", "pdffonts", "pdfimages", "pdfseparate", "pdfunite", "pdftocairo", "pdftohtml", "pdftops", "pdfattach", "pdfdetach", "typst")}, "resources": {"python_packages": packages.relative_to(root).as_posix(), "node_packages": "dependencies/node-packages", "fonts": "dependencies/fonts", "override": "dependencies/bin/override", "fallback": "dependencies/bin/fallback", "licenses": "licenses"}}
        write(output.with_suffix(output.suffix + ".json"), {"component": "office-bundle", "version": office["version"], "layout_version": office["layout_version"], "target": target, "asset": asset, "native_execution_verified": False, "published": False, "expanded_bytes": expanded})
        print(json.dumps({"candidate": str(output), "download_bytes": asset["max_bytes"], "expanded_bytes": expanded, "published": False}), flush=True)


def verify_candidate(catalog, target, work):
    if target != "darwin-arm64": return  # Cross-built payloads receive static checks only.
    descriptor_path = work / "assets" / f"office-{catalog['office_bundle']['version']}-{target}.tar.gz.json"
    descriptor = read(descriptor_path)
    local = json.loads(json.dumps(catalog))
    asset = dict(descriptor["asset"], url=descriptor_path.with_suffix("").as_uri())
    local["office_bundle"]["assets"][target] = asset
    candidate = work / "validation-catalog.json"
    write(candidate, local)
    subprocess.run(["go", "test", "./src/managedenv", "-run", "^TestOfficeBundleCandidate$", "-count=1", "-v", "-timeout=25m"], cwd=ROOT, check=True,
        env={**os.environ, "AWAKEN_OFFICE_CANDIDATE_CATALOG": str(candidate), "AWAKEN_OFFICE_VERIFY_ROOT": str(work / "validation" / target)})
    descriptor["native_execution_verified"] = True
    write(descriptor_path, descriptor)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", type=Path, required=True)
    parser.add_argument("--target", choices=[*TARGETS,"all"], default="all")
    parser.add_argument("--release", required=True)
    parser.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()
    work = args.work.resolve()
    if work == ROOT or work.is_relative_to(ROOT): parser.error("work must be outside the repository")
    if not args.release.startswith("deps-") or any(s in args.release for s in ("/", "..")): parser.error("use an immutable deps-* release")
    work.mkdir(parents=True, exist_ok=True)
    catalog = read(ROOT / "src/managedenv/assets/catalog.json")
    locks = read(ROOT / "src/managedenv/assets/office-package-locks.json")
    tools = {} if args.verify_only else language_tools(catalog, work)
    for target in TARGETS if args.target=="all" else [args.target]:
        print("Assembling Office/"+target, flush=True)
        if not args.verify_only: assemble(catalog, locks, target, work, args.release, tools)
        verify_candidate(catalog, target, work)


if __name__ == "__main__": main()
