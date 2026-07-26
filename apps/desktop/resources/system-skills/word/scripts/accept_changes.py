#!/usr/bin/env python3
"""通过独立 LibreOffice profile 接受 DOCX 全部修订，输出新文件。"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path
from xml.etree import ElementTree

sys.dont_write_bytecode = True

from office_runtime import find_soffice, office_env


WORD_NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
MACRO = """<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE script:module PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "module.dtd">
<script:module xmlns:script="http://openoffice.org/2000/script" script:name="Module1" script:language="StarBasic">
Sub AwakenAcceptAllChanges()
  Dim dispatcher As Object
  dispatcher = createUnoService("com.sun.star.frame.DispatchHelper")
  dispatcher.executeDispatch(ThisComponent.CurrentController.Frame, ".uno:AcceptAllTrackedChanges", "", 0, Array())
  ThisComponent.store()
  ThisComponent.close(True)
End Sub
</script:module>
"""


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tracked_change_count(path: Path) -> int:
    with zipfile.ZipFile(path) as package:
        count = 0
        for name in package.namelist():
            if not name.startswith("word/") or not name.endswith(".xml"):
                continue
            root = ElementTree.fromstring(package.read(name))
            count += sum(
                1
                for node in root.iter()
                if node.tag in {f"{{{WORD_NS}}}ins", f"{{{WORD_NS}}}del"}
            )
    return count


def run(input_path: Path, output_path: Path, timeout: int) -> dict:
    if input_path.resolve() == output_path.resolve():
        raise ValueError("输出路径必须与输入路径不同，避免覆盖用户原件")
    executable = find_soffice()
    if executable is None:
        raise FileNotFoundError("未找到 LibreOffice")
    before_count = tracked_change_count(input_path)
    with tempfile.TemporaryDirectory(prefix="awaken-word-accept-", ignore_cleanup_errors=True) as directory:
        root = Path(directory)
        profile = root / "profile"
        working = root / input_path.name
        shutil.copy2(input_path, working)
        init = subprocess.run(
            [str(executable), "--headless", "--terminate_after_init", f"-env:UserInstallation={profile.resolve().as_uri()}"],
            capture_output=True,
            text=True,
            timeout=min(timeout, 30),
            check=False,
            env=office_env(),
        )
        if init.returncode != 0:
            raise RuntimeError((init.stderr or init.stdout or "LibreOffice profile 初始化失败").strip())
        macro_dir = profile / "user" / "basic" / "Standard"
        macro_dir.mkdir(parents=True, exist_ok=True)
        (macro_dir / "Module1.xba").write_text(MACRO, encoding="utf-8")
        before_hash = digest(working)
        command = [
            str(executable),
            "--headless",
            "--nologo",
            "--norestore",
            f"-env:UserInstallation={profile.resolve().as_uri()}",
            "vnd.sun.star.script:Standard.Module1.AwakenAcceptAllChanges?language=Basic&location=application",
            str(working.resolve()),
        ]
        try:
            result = subprocess.run(command, capture_output=True, text=True, timeout=timeout, check=False, env=office_env())
        except subprocess.TimeoutExpired as error:
            raise RuntimeError(f"LibreOffice 在 {timeout}s 后超时，不能视为成功") from error
        if result.returncode != 0:
            raise RuntimeError((result.stderr or result.stdout or f"LibreOffice 退出码 {result.returncode}").strip())
        after_count = tracked_change_count(working)
        after_hash = digest(working)
        if before_count and (after_count or before_hash == after_hash):
            raise RuntimeError("LibreOffice 没有真正接受并写回全部修订")
        output_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = output_path.with_name(f".{output_path.name}.awaken-tmp")
        if temporary.exists():
            raise RuntimeError(f"临时输出已存在: {temporary}")
        shutil.copy2(working, temporary)
        os.replace(temporary, output_path)
    return {
        "status": "success",
        "input_tracked_changes": before_count,
        "remaining_tracked_changes": 0,
        "output": str(output_path),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="接受 DOCX 全部修订")
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--timeout", type=int, default=90)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    try:
        result = run(args.input.resolve(), args.output.resolve(), args.timeout)
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else f"已生成 {result['output']}")
        return 0
    except Exception as error:
        result = {"status": "failed", "error": str(error)}
        print(json.dumps(result, ensure_ascii=False, indent=2) if args.json else result["error"])
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
