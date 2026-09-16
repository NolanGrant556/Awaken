#!/usr/bin/env python3
"""Refresh the checked-in managed environment catalog from pinned public releases.

This is a maintainer command, not part of application startup. It never installs
packages into a project and never embeds credentials in generated resources.
"""
import ast
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / "src/managedenv/assets/catalog.json"
TARGETS = {"darwin-arm64": "aarch64-apple-darwin", "darwin-x64": "x86_64-apple-darwin", "windows-x64": "x86_64-pc-windows-msvc"}


def release(repo, tag):
    return json.loads(subprocess.check_output(["gh", "api", f"repos/{repo}/releases/tags/{tag}"], text=True))


def asset(item, executables, prefix=""):
    digest = item.get("digest", "")
    if not digest.startswith("sha256:") or len(digest) != 71:
        raise ValueError(f"release asset needs verified digest: {item['name']}")
    return {"url": item["browser_download_url"], "sha256": digest[7:], "format": "zip" if item["name"].endswith(".zip") else "tar.gz", "strip_prefix": prefix, "executables": executables}


def main():
    components = {}
    py = release("astral-sh/python-build-standalone", "20260901")
    components["python"] = {"version": "3.12.14", "license": "PSF-2.0", "source": "https://github.com/astral-sh/python-build-standalone", "probe": "python", "probe_args": ["--version"], "assets": {}}
    uv = release("astral-sh/uv", "0.12.13")
    components["uv"] = {"version": "0.12.13", "license": "MIT OR Apache-2.0", "source": "https://github.com/astral-sh/uv", "probe": "uv", "probe_args": ["--version"], "assets": {}}
    rg = release("BurntSushi/ripgrep", "15.2.0")
    components["ripgrep"] = {"version": "15.2.0", "license": "MIT OR Unlicense", "source": "https://github.com/BurntSushi/ripgrep", "probe": "rg", "probe_args": ["--version"], "assets": {}}
    for target, triple in TARGETS.items():
        item = next(x for x in py["assets"] if x["name"] == f"cpython-3.12.14+20260901-{triple}-install_only_stripped.tar.gz")
        exe = "python.exe" if target.startswith("windows") else "bin/python3.12"
        components["python"]["assets"][target] = asset(item, {"python": exe, "python3": exe}, "python")
        name = f"uv-{triple}"
        item = next(x for x in uv["assets"] if x["name"] == name + (".zip" if target.startswith("windows") else ".tar.gz"))
        components["uv"]["assets"][target] = asset(item, {"uv": "uv.exe" if target.startswith("windows") else "uv"}, "" if target.startswith("windows") else name)
        name = f"ripgrep-15.2.0-{triple}"
        item = next(x for x in rg["assets"] if x["name"] == name + (".zip" if target.startswith("windows") else ".tar.gz"))
        components["ripgrep"]["assets"][target] = asset(item, {"rg": "rg.exe" if target.startswith("windows") else "rg"}, name)
    node = json.loads((ROOT / "apps/desktop/resources/node-runtime.json").read_text())
    components["node"] = {"version": node["version"], "license": node["license"], "source": "https://nodejs.org", "probe": "node", "probe_args": ["--version"], "assets": {}}
    for target in TARGETS:
        item = node["targets"][target.replace("windows", "win32")]
        win = target.startswith("windows")
        prefix = item["asset"].removesuffix(".tar.gz").removesuffix(".zip")
        components["node"]["assets"][target] = {"url": node["releaseBaseUrl"] + "/" + item["asset"], "sha256": item["sha256"], "format": item["format"], "strip_prefix": prefix, "executables": {"node": "node.exe" if win else "bin/node", "npm": "node_modules/npm/bin/npm-cli.js" if win else "lib/node_modules/npm/bin/npm-cli.js", "npx": "node_modules/npm/bin/npx-cli.js" if win else "lib/node_modules/npm/bin/npx-cli.js"}}
    tree = ast.parse((ROOT / "apps/desktop/resources/system-skills/PDF_Max/scripts/typst_runtime.py").read_text())
    version = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "TYPST_VERSION" for t in n.targets))
    assets = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.AnnAssign) and getattr(n.target, "id", "") == "ASSETS")
    components["typst"] = {"version": version, "license": "Apache-2.0", "source": "https://github.com/typst/typst", "probe": "typst", "probe_args": ["--version"], "assets": {}}
    for (system, arch), item in assets.items():
        target = f"{system}-{arch}"
        if target not in TARGETS:
            continue
        prefix = item["name"].removesuffix(".tar.xz").removesuffix(".zip")
        components["typst"]["assets"][target] = {"url": f"https://github.com/typst/typst/releases/download/v{version}/{item['name']}", "sha256": item["sha256"], "format": "zip" if item["name"].endswith(".zip") else "tar.xz", "strip_prefix": prefix, "executables": {"typst": item["member"]}}
    cli = json.loads((ROOT / "apps/desktop/resources/cli-connectors/manifest.json").read_text())
    for item in cli["assets"]:
        target = f"{item['platform'].replace('win32', 'windows')}-{item['architecture']}"
        if target not in TARGETS:
            continue
        component = components.setdefault(item["providerId"], {"version": item["version"], "license": "see upstream release", "source": item["url"], "probe": "dws" if item["providerId"] == "dingtalk-workspace" else "lark-cli", "probe_args": ["--version"], "assets": {}})
        component["assets"][target] = {"url": item["url"], "sha256": item["sha256"], "format": item["archiveType"], "executables": {component["probe"]: item["archiveExecutablePath"]}}
    for name, probe, license_, source in [
        ("libreoffice", "soffice", "MPL-2.0", "https://www.libreoffice.org"),
        ("poppler", "pdftoppm", "GPL-2.0-or-later", "https://poppler.freedesktop.org"),
        ("git", "git", "GPL-2.0-only", "https://git-scm.com"),
        ("ffmpeg", "ffmpeg", "see selected build", "https://ffmpeg.org"),
        ("tesseract", "tesseract", "Apache-2.0", "https://github.com/tesseract-ocr/tesseract"),
        ("browser-automation", "", "see selected browser build", "https://playwright.dev"),
        ("fonts", "", "OFL-1.1", "https://github.com/notofonts"),
    ]:
        components[name] = {"version": "", "license": license_, "source": source, "probe": probe, "probe_args": ["-v"] if name == "poppler" else ["--version"], "assets": {}, "optional": name in ("ffmpeg", "tesseract", "browser-automation")}
    components["fonts"] = {'version': '2.004', 'license': 'OFL-1.1', 'source': 'https://github.com/notofonts/noto-cjk/releases/tag/Sans2.004', 'assets': {'darwin-arm64': {'url': 'https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/08_NotoSansCJKsc.zip', 'sha256': 'a927e56f53bd6c3b920bc139c0b94aa36c7d9ad0cf009b159437a1a003581140', 'format': 'zip', 'executables': {}, 'resources': {'font_regular': 'NotoSansCJKsc-Regular.otf', 'font_bold': 'NotoSansCJKsc-Bold.otf', 'license': 'LICENSE'}, 'max_bytes': 94523633}, 'darwin-x64': {'url': 'https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/08_NotoSansCJKsc.zip', 'sha256': 'a927e56f53bd6c3b920bc139c0b94aa36c7d9ad0cf009b159437a1a003581140', 'format': 'zip', 'executables': {}, 'resources': {'font_regular': 'NotoSansCJKsc-Regular.otf', 'font_bold': 'NotoSansCJKsc-Bold.otf', 'license': 'LICENSE'}, 'max_bytes': 94523633}, 'windows-x64': {'url': 'https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/08_NotoSansCJKsc.zip', 'sha256': 'a927e56f53bd6c3b920bc139c0b94aa36c7d9ad0cf009b159437a1a003581140', 'format': 'zip', 'executables': {}, 'resources': {'font_regular': 'NotoSansCJKsc-Regular.otf', 'font_bold': 'NotoSansCJKsc-Bold.otf', 'license': 'LICENSE'}, 'max_bytes': 94523633}}}
    components["git"] = {'version': '2.55.0', 'license': 'GPL-2.0-only', 'source': 'https://git-scm.com/install/source', 'probe': 'git', 'probe_args': ['--version'], 'assets': {'windows-x64': {'url': 'https://github.com/git-for-windows/git/releases/download/v2.55.0.windows.5/MinGit-2.55.0.5-64-bit.zip', 'sha256': '56d7b226b7693196cfc71fef26568f536c4a021ab6c37ff2db4287bed908e96e', 'format': 'zip', 'executables': {'git': 'cmd/git.exe', 'git-remote-https': 'mingw64/bin/git-remote-https.exe'}, 'max_bytes': 38989688}}, 'optional': False}
    pyoffice = {"python-docx": "1.2.0", "openpyxl": "3.1.5", "pandas": "2.3.3", "python-pptx": "1.0.2", "pillow": "12.1.1", "pypdf": "6.8.0", "pdfplumber": "0.11.9", "reportlab": "4.4.10", "markitdown[xlsx,pptx]": "0.1.5"}
    nodeoffice = {"docx": "9.5.1", "pptxgenjs": "4.0.1"}
    caps = {"python-core": {"components": ["python"]}, "python-office": {"python": pyoffice}, "node-office": {"node": nodeoffice}, "office-render": {"components": ["python", "libreoffice", "poppler"]}}
    for name in components:
        caps.setdefault(name, {"components": [name]})
    for name in ["word.read", "word.edit_ooxml", "ppt.structural_qa", "creator"]:
        caps[name] = caps["python-core"]
    for name in ["excel.read_precise", "excel.read_quick", "excel.clean", "excel.create_edit", "ppt.read", "ppt.edit", "pdf.process"]:
        caps[name] = caps["python-office"]
    caps["word.create"] = caps["node-office"]
    caps["ppt.create"] = caps["node-office"]
    for name in ["word.visual_qa", "word.accept_changes", "ppt.visual_qa", "ppt.template_map", "excel.recalc"]:
        caps[name] = {"components": ["libreoffice", "poppler"], "python": pyoffice}
    caps["pdf.read"] = {"components": ["poppler"]}
    caps["word.create_python"] = {"python": {"python-docx": "1.2.0"}}
    caps["ppt.create_python"] = {"python": {"python-pptx": "1.0.2"}}
    caps["pdf_max"] = {"components": ["python", "typst", "fonts"]}
    caps["node-images"] = {"node": {**nodeoffice, "react": "19.2.4", "react-dom": "19.2.4", "react-icons": "5.6.0", "sharp": "0.34.5"}}
    caps.update({'excel.read_precise': {'python': {'openpyxl': '3.1.5'}}, 'excel.create_edit': {'python': {'openpyxl': '3.1.5'}}, 'excel.clean': {'python': {'pandas': '2.3.3'}}, 'excel.read_quick': {'python': {'markitdown[xlsx]': '0.1.5'}}, 'ppt.read': {'python': {'python-pptx': '1.0.2', 'markitdown[pptx]': '0.1.5'}}, 'ppt.edit': {'python': {'python-pptx': '1.0.2'}}, 'pdf.process': {'python': {'pypdf': '6.8.0', 'pdfplumber': '0.11.9', 'reportlab': '4.4.10'}}, 'word.visual_qa': {'components': ['python', 'libreoffice', 'poppler']}, 'word.accept_changes': {'components': ['python', 'libreoffice']}, 'ppt.visual_qa': {'components': ['python', 'libreoffice', 'poppler']}, 'ppt.template_map': {'components': ['libreoffice', 'poppler'], 'python': {'pillow': '12.1.1'}}, 'excel.recalc': {'components': ['libreoffice'], 'python': {'openpyxl': '3.1.5'}}})
    for spec in caps.values():
        if spec.get("node"):
            spec["components"] = sorted(set(spec.get("components", []) + ["python"]))
    native = json.loads((ROOT / "src/managedenv/assets/native-assets.json").read_text())
    components["browser-automation"] = json.loads((ROOT / "src/managedenv/assets/browser-assets.json").read_text())
    for entry in native["entries"]:
        component = components[entry["component"]]
        if component["version"] and component["version"] != entry["version"]:
            raise ValueError("native asset versions disagree")
        component.update({key: entry[key] for key in ("version", "license", "source", "probe", "probe_args")})
        component["assets"][entry["target"]] = entry["asset"]
    DEST.parent.mkdir(parents=True, exist_ok=True)
    DEST.write_text(json.dumps({"schema_version": 1, "revision": "2026-09-15.1", "components": components, "capabilities": caps}, ensure_ascii=False, indent=2) + "\n")
    print(f"Catalog: {len(components)} components, {len(caps)} capabilities")


if __name__ == "__main__":
    main()
