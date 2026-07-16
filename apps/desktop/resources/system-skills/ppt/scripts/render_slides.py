#!/usr/bin/env python3
"""将 pptx 转为逐页 JPEG，供 Awaken Read（视觉）自检。"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path


def find_soffice() -> str | None:
    for name in ("soffice", "libreoffice"):
        path = shutil.which(name)
        if path:
            return path
    system = platform.system()
    if system == "Darwin":
        mac = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
        if mac.exists():
            return str(mac)
    elif system == "Windows":
        for env_name in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            base = os.environ.get(env_name)
            if not base:
                continue
            candidate = Path(base) / "LibreOffice" / "program" / "soffice.exe"
            if candidate.exists():
                return str(candidate)
    return None


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: render_slides.py <deck.pptx> [outdir] [dpi]", file=sys.stderr)
        return 2

    src = Path(sys.argv[1]).expanduser().resolve()
    outdir = Path(sys.argv[2]).expanduser().resolve() if len(sys.argv) > 2 else src.parent / "preview"
    dpi = sys.argv[3] if len(sys.argv) > 3 else "150"

    if not src.exists():
        print(f"not found: {src}", file=sys.stderr)
        return 1

    soffice = find_soffice()
    pdftoppm = shutil.which("pdftoppm")
    if not soffice:
        print("LibreOffice (soffice) not found", file=sys.stderr)
        return 1
    if not pdftoppm:
        print("pdftoppm (poppler) not found; install poppler-utils / brew install poppler", file=sys.stderr)
        return 1

    outdir.mkdir(parents=True, exist_ok=True)
    conv = subprocess.run(
        [
            soffice,
            "--headless",
            "--nologo",
            "--convert-to",
            "pdf",
            "--outdir",
            str(outdir),
            str(src),
        ],
        capture_output=True,
        text=True,
        timeout=180,
    )
    pdfs = list(outdir.glob("*.pdf"))
    if not pdfs:
        print(conv.stderr or "pdf conversion failed", file=sys.stderr)
        return 1

    pdf = pdfs[0]
    prefix = outdir / "slide"
    render = subprocess.run(
        [pdftoppm, "-jpeg", "-r", str(dpi), str(pdf), str(prefix)],
        capture_output=True,
        text=True,
        timeout=120,
    )
    if render.returncode != 0:
        print(render.stderr or "pdftoppm failed", file=sys.stderr)
        return 1

    slides = sorted(outdir.glob("slide*.jpg"))
    print(f"rendered {len(slides)} slides into {outdir}")
    for p in slides:
        print(p)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
