#!/usr/bin/env python3
"""Build baseline package locks with Awaken's pinned interpreters (maintainer only)."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--uv", required=True)
    parser.add_argument("--python", required=True)
    parser.add_argument("--node", required=True)
    parser.add_argument("--npm", required=True, help="npm-cli.js path in the pinned Node distribution")
    parser.add_argument("--work", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--office", action="store_true", help="build the complete Office graph, including cross-platform compatibility constraints")
    args = parser.parse_args()
    work = args.work.resolve()
    if ROOT == work or ROOT in work.parents:
        parser.error("build work/cache must be outside the project")
    work.mkdir(parents=True, exist_ok=True)
    catalog = json.loads((ROOT / "src/managedenv/assets/catalog.json").read_text())
    environment = {**os.environ, "HOME": str(work), "USERPROFILE": str(work), "UV_CACHE_DIR": str(work / "uv-cache"), "UV_NO_CONFIG": "1", "UV_PYTHON_DOWNLOADS": "never", "npm_config_cache": str(work / "npm-cache"), "npm_config_userconfig": str(work / "empty-npmrc"), "npm_config_globalconfig": str(work / "empty-global-npmrc"), "npm_config_registry": "https://registry.npmjs.org/", "PATH": str(Path(args.node).parent) + os.pathsep + os.defpath}
    for component, executable in [("python", args.python), ("uv", args.uv), ("node", args.node)]:
        output = subprocess.check_output([executable, "--version"], env=environment, text=True)
        if catalog["components"][component]["version"] not in output:
            raise ValueError(f"{component}: use the catalog's pinned runtime")
    entries = []
    seen = set()
    capabilities = {"office-bundle": catalog["office_bundle"]["spec"]} if args.office else catalog["capabilities"]
    for capability, spec in capabilities.items():
        packages = {k: spec[k] for k in ("python", "node") if spec.get(k)}
        key = json.dumps(packages, sort_keys=True)
        if not packages or key in seen:
            continue
        seen.add(key)
        with tempfile.TemporaryDirectory(prefix="lock-", dir=work) as directory:
            directory = Path(directory)
            entry = {"spec": packages}
            if packages.get("python"):
                requirements = directory / "requirements.in"
                requirements.write_text("".join(f"{name}=={version}\n" for name, version in sorted(packages["python"].items())))
                if args.office:
                    # cryptography 50 has no Intel macOS wheel. A complete
                    # Office release must have binary wheels on all targets.
                    with requirements.open("a") as stream: stream.write("cryptography==46.0.7\n")
                lock = directory / "requirements.txt"
                subprocess.run([args.uv, "pip", "compile", "--python", args.python, "--python-version", "3.12", "--universal", "--generate-hashes", "--no-header", "--no-annotate", "--only-binary", ":all:", "--index-url", "https://pypi.org/simple", str(requirements), "-o", str(lock)], cwd=directory, env=environment, check=True, stdout=subprocess.DEVNULL)
                entry["python"] = lock.read_text()
            if packages.get("node"):
                package = {"name": "awaken-managed-environment", "version": "1.0.0", "private": True, "dependencies": packages["node"]}
                (directory / "package.json").write_text(json.dumps(package, indent=2) + "\n")
                subprocess.run([args.node, args.npm, "install", "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund"], cwd=directory, env=environment, check=True, stdout=subprocess.DEVNULL)
                entry["node"] = (directory / "package-lock.json").read_text()
                entry["package"] = (directory / "package.json").read_text()
            entries.append(entry)
            print(f"Locked {capability}", flush=True)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"schema_version": 1, "targets": ["darwin-arm64", "darwin-x64", "windows-x64"], "python_resolution": "universal Python 3.12", "entries": entries}, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
