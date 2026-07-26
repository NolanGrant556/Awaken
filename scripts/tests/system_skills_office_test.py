from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
SYSTEM_SKILLS = REPO / "apps" / "desktop" / "resources" / "system-skills"


def load_script(skill: str, script: str, module_name: str):
    script_dir = SYSTEM_SKILLS / skill / "scripts"
    sys.path.insert(0, str(script_dir))
    try:
        spec = importlib.util.spec_from_file_location(module_name, script_dir / script)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"cannot load {script}")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    finally:
        sys.path.pop(0)


def write_package(path: Path, files: dict[str, str | bytes]) -> None:
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as package:
        for name, content in files.items():
            package.writestr(name, content)


def minimal_docx_files() -> dict[str, str]:
    return {
        "[Content_Types].xml": """<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>""",
        "_rels/.rels": """<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>""",
        "word/document.xml": """<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>第一</w:t></w:r><w:r><w:t>段</w:t></w:r></w:p></w:body></w:document>""",
    }


def minimal_pptx_files() -> dict[str, str]:
    return {
        "[Content_Types].xml": """<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
  <Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
</Types>""",
        "_rels/.rels": """<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>""",
        "ppt/presentation.xml": """<?xml version="1.0" encoding="UTF-8"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>""",
        "ppt/_rels/presentation.xml.rels": """<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
</Relationships>""",
        "ppt/slides/slide1.xml": """<?xml version="1.0" encoding="UTF-8"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1" name="Title"/></p:nvSpPr></p:sp></p:spTree></p:cSld></p:sld>""",
    }


class SkillTextTests(unittest.TestCase):
    def test_three_skills_keep_awaken_contract_and_chinese_workflow(self):
        for skill in ("word", "ppt", "excel"):
            body = (SYSTEM_SKILLS / skill / "SKILL.md").read_text(encoding="utf-8")
            self.assertIn("version: 2.0.0", body)
            self.assertIn("${AWAKEN_SKILL_DIR}", body)
            self.assertIn("artifact_publish", body)
            self.assertIn("依赖", body)
            self.assertIn("校验", body)


class WordScriptTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.validator = load_script("word", "validate_docx.py", "awaken_word_validate_test")
        cls.package = load_script("word", "ooxml_package.py", "awaken_word_package_test")
        cls.merger = load_script("word", "merge_runs.py", "awaken_word_merge_test")
        cls.acceptor = load_script("word", "accept_changes.py", "awaken_word_accept_test")

    def test_valid_docx_and_relationship_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            valid = root / "valid.docx"
            write_package(valid, minimal_docx_files())
            self.assertEqual([], self.validator.inspect(valid))

            broken_files = minimal_docx_files()
            broken_files["_rels/.rels"] = broken_files["_rels/.rels"].replace(
                "word/document.xml", "word/missing.xml"
            )
            broken = root / "broken.docx"
            write_package(broken, broken_files)
            self.assertTrue(
                any("关系目标不存在" in issue for issue in self.validator.inspect(broken))
            )

    def test_safe_unpack_rejects_path_escape(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / "unsafe.docx"
            files = minimal_docx_files()
            files["../escape.txt"] = "escape"
            write_package(archive, files)
            with self.assertRaises(ValueError):
                self.package.safe_extract(archive, root / "unpacked")

    def test_merge_only_plain_identical_runs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            unpacked = root / "unpacked"
            unpacked.mkdir()
            package = root / "input.docx"
            write_package(package, minimal_docx_files())
            self.package.safe_extract(package, unpacked)
            merged = self.merger.merge_document(unpacked / "word" / "document.xml")
            self.assertEqual(1, merged)
            text = (unpacked / "word" / "document.xml").read_text(encoding="utf-8")
            self.assertIn("第一段", text)

    def test_tracked_changes_cover_headers_and_other_word_parts(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "header-revision.docx"
            files = minimal_docx_files()
            files["word/header1.xml"] = """<?xml version="1.0" encoding="UTF-8"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:ins w:id="1"><w:r><w:t>页眉修订</w:t></w:r></w:ins></w:p></w:hdr>"""
            write_package(path, files)
            self.assertEqual(1, self.acceptor.tracked_change_count(path))
            self.assertTrue(
                any("word/header1.xml" in issue and "w:author" in issue for issue in self.validator.inspect(path))
            )


class PptScriptTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.validator = load_script("ppt", "validate_pptx.py", "awaken_ppt_validate_test")
        cls.package = load_script("ppt", "ooxml_package.py", "awaken_ppt_package_test")

    def test_valid_pptx_and_orphan_slide(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            valid = root / "valid.pptx"
            write_package(valid, minimal_pptx_files())
            self.assertEqual([], self.validator.inspect(valid))

            orphan_files = minimal_pptx_files()
            orphan_files["ppt/slides/slide2.xml"] = orphan_files["ppt/slides/slide1.xml"]
            orphan = root / "orphan.pptx"
            write_package(orphan, orphan_files)
            self.assertTrue(
                any("孤立 slide" in issue for issue in self.validator.inspect(orphan))
            )

    def test_stacked_chart_out_end_is_rejected(self):
        chart = """<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart><c:plotArea><c:barChart><c:grouping val="stacked"/><c:dLbls><c:dLblPos val="outEnd"/></c:dLbls><c:axId val="1"/><c:axId val="2"/></c:barChart><c:catAx><c:axId val="1"/></c:catAx><c:valAx><c:axId val="2"/></c:valAx></c:plotArea></c:chart></c:chartSpace>"""
        from xml.etree import ElementTree

        issues = self.validator.chart_issues("ppt/charts/chart1.xml", ElementTree.fromstring(chart))
        self.assertTrue(any("outEnd" in issue for issue in issues))


@unittest.skipUnless(importlib.util.find_spec("openpyxl"), "需要 openpyxl")
class ExcelScriptTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.inspector = load_script("excel", "inspect_workbook.py", "awaken_excel_inspect_test")
        cls.recalc = load_script("excel", "recalc.py", "awaken_excel_recalc_test")

    def test_formula_and_dynamic_function_risk(self):
        from openpyxl import Workbook

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "model.xlsx"
            workbook = Workbook()
            sheet = workbook.active
            sheet["A1"] = 10
            sheet["A2"] = "=SUM(A1:A1)"
            sheet["A3"] = '=XLOOKUP(A1,B1:B2,C1:C2)'
            workbook.save(path)
            result = self.inspector.inspect(path)
            self.assertEqual(2, result["formula_count"])
            self.assertTrue(any(item["function"] == "XLOOKUP" for item in result["risky_formulas"]))

    def test_external_link_part_blocks_recalculation_before_libreoffice(self):
        from openpyxl import Workbook

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "external.xlsx"
            workbook = Workbook()
            workbook.active["A1"] = "=1+1"
            workbook.save(path)
            with zipfile.ZipFile(path, "a") as package:
                package.writestr("xl/externalLinks/externalLink1.xml", "<externalLink/>")
            formulas, external = self.recalc.workbook_risks(path)
            self.assertEqual(1, formulas)
            self.assertEqual(["xl/externalLinks/externalLink1.xml"], external)


if __name__ == "__main__":
    unittest.main()
