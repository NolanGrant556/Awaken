from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SKILLS = REPO / 'apps/desktop/resources/system-skills'


def run_script(skill, script, *args):
    return subprocess.run([sys.executable, '-B', str(SKILLS / skill / 'scripts' / script),
                           *map(str, args)], capture_output=True, text=True)


class CreatorResourceTests(unittest.TestCase):
    def validate_both(self, body, resources=()):
        results = []
        with tempfile.TemporaryDirectory() as directory:
            plugin = Path(directory) / 'example-plugin'
            skill = plugin / 'skills' / 'example-skill'
            skill.mkdir(parents=True)
            (plugin / 'plugin.json').write_text(json.dumps({
                '$schema': 'https://agent-plugins.org/schemas/1.0.0/plugin.schema.json',
                'name': 'example-plugin'}))
            (skill / 'SKILL.md').write_text('---\nname: example-skill\ndescription: Example.\n---\n' + body)
            for path in resources:
                resource = skill / path
                resource.parent.mkdir(parents=True, exist_ok=True)
                resource.write_text('reference content')
            for owner, entry, candidate in [('skill-creator', 'validate_skill.py', skill),
                                             ('plugin-creator', 'validate_plugin.py', plugin)]:
                result = run_script(owner, entry, candidate)
                results.append((result.returncode, json.loads(result.stdout)))
        return results

    def test_unicode_spaces_and_encoded_links_resolve(self):
        body = ('Read [contract](<references/Awaken 功能.md>) and '
                '`references/字段合同.md` or [same](references/%E5%AD%97%E6%AE%B5%E5%90%88%E5%90%8C.md#one). '
                'Run "${AWAKEN_SKILL_DIR}/scripts/检查 文件.py".')
        for code, result in self.validate_both(body, ['references/Awaken 功能.md',
                'references/字段合同.md', 'scripts/检查 文件.py']):
            self.assertEqual(0, code, result)

    def test_missing_unicode_resource_is_rejected(self):
        for code, result in self.validate_both('Read `references/缺少.md`.'):
            self.assertNotEqual(0, code)
            self.assertIn('references/缺少.md', ' '.join(result['errors']))

    def test_external_paths_are_not_package_resources(self):
        for code, result in self.validate_both(
                'Use `/opt/tools/scripts/helper.py`, "${CODEX_HOME}/skills/creator/scripts/check.py" '
                'or [external](https://example.com/references/api.md).'):
            self.assertEqual(0, code, result)

    def test_traversal_is_rejected(self):
        for code, result in self.validate_both('Read `references/../../outside.md`.'):
            self.assertNotEqual(0, code)
            self.assertIn('escapes package', ' '.join(result['errors']))


class TypstDecisionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        path = SKILLS / 'PDF_Max/scripts/plan_document.py'
        spec = importlib.util.spec_from_file_location('awaken_typst_decisions', path)
        cls.planner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.planner)
        cls.catalog = cls.planner.load_object(cls.planner.DEFAULT_CATALOG)

    def manifest(self):
        return self.planner.validate_manifest({'task_kind': 'new-document', 'signals': ['report'],
                    'content_features': {'custom_vector_drawings': 1, 'styled_code_blocks': 1}})

    def plan(self, manifest, catalog=None):
        return self.planner.create_plan(manifest, catalog or self.catalog, 'typst-test-version', None)

    def test_accept_and_reject_continue_without_reasking(self):
        manifest = self.manifest()
        first = self.plan(manifest)
        self.assertEqual('needs-confirmation', first['state'])
        manifest['enhancement_decisions'] = {'context': first['decision_context'],
                                            'accept': ['cetz'], 'reject': ['codly']}
        final = self.plan(self.planner.validate_manifest(manifest))
        self.assertEqual('ready', final['state'])
        self.assertIn('cetz', [item['id'] for item in final['enhancements']])
        self.assertIn('codly', [item['id'] for item in final['exclusions']])
        self.assertEqual([], final['confirmation_candidates'])

    def test_partial_decisions_only_ask_remaining_choice(self):
        manifest = self.manifest()
        manifest['enhancement_decisions'] = {'context': self.plan(manifest)['decision_context'],
                                             'reject': ['cetz']}
        final = self.plan(manifest)
        self.assertEqual(['codly'], [item['id'] for item in final['confirmation_candidates']])

    def test_changed_task_or_catalog_invalidates_decisions(self):
        manifest = self.manifest()
        manifest['enhancement_decisions'] = {'context': self.plan(manifest)['decision_context'],
                                             'accept': ['cetz']}
        changed = deepcopy(manifest)
        changed['signals'].append('changed-layout')
        with self.assertRaisesRegex(ValueError, '已过期'):
            self.plan(changed)
        catalog = deepcopy(self.catalog)
        catalog['packages'][0]['version'] = '999.0.0'
        with self.assertRaisesRegex(ValueError, '已过期'):
            self.plan(manifest, catalog)

    def test_conflicting_or_unknown_choices_fail(self):
        manifest = self.manifest()
        manifest['enhancement_decisions'] = {'context': self.plan(manifest)['decision_context'],
                                             'accept': ['cetz'], 'reject': ['cetz']}
        with self.assertRaisesRegex(ValueError, '同时'):
            self.planner.validate_manifest(manifest)
        manifest['enhancement_decisions'] = {'context': self.plan(self.manifest())['decision_context'],
                                             'accept': ['unknown']}
        with self.assertRaisesRegex(ValueError, '未知'):
            self.plan(manifest)

    def test_confirmation_does_not_override_required_conflict_owner(self):
        manifest = self.manifest()
        manifest['content_features']['node_arrow_diagrams'] = 1
        manifest['enhancement_decisions'] = {'context': self.plan(manifest)['decision_context'],
                                             'accept': ['cetz'], 'reject': ['codly']}
        final = self.plan(manifest)
        diagrams = [item['id'] for item in final['enhancements']
                    if item.get('conflict_group') == 'general-diagram']
        self.assertEqual(['fletcher'], diagrams)
        self.assertIn('cetz', [item['id'] for item in final['exclusions']])


@unittest.skipUnless(importlib.util.find_spec('pypdf') and importlib.util.find_spec('reportlab'),
                     'requires pypdf >= 6.10 and reportlab')
class PDFFormTests(unittest.TestCase):
    def create_form(self, path, repeated=False):
        from reportlab.pdfgen import canvas
        from pypdf import PdfReader, PdfWriter
        from pypdf.generic import ArrayObject, DictionaryObject, NameObject, TextStringObject
        drawing = canvas.Canvas(str(path))
        for name in ['first', 'second']:
            drawing.drawString(50, 740, name)
            drawing.acroForm.textfield(name=name, x=50, y=680, width=200, height=30)
            drawing.showPage()
        drawing.save()
        if repeated:
            writer = PdfWriter(clone_from=PdfReader(path))
            widgets = [page['/Annots'][0] for page in writer.pages]
            parent = DictionaryObject({NameObject('/FT'): NameObject('/Tx'),
                NameObject('/T'): TextStringObject('shared'), NameObject('/Kids'): ArrayObject(widgets)})
            parent_ref = writer._add_object(parent)
            for widget_ref in widgets:
                widget = widget_ref.get_object()
                for key in ['/T', '/FT', '/V', '/DV']:
                    widget.pop(key, None)
                widget[NameObject('/Parent')] = parent_ref
            writer.root_object['/AcroForm'][NameObject('/Fields')] = ArrayObject([parent_ref])
            writer.write(path)

    def test_extract_and_fill_second_page_then_read_back(self):
        from pypdf import PdfReader
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            source, output = path / 'source.pdf', path / 'filled.pdf'
            self.create_form(source)
            extracted = run_script('pdf', 'extract_form_field_info.py', source)
            self.assertEqual(0, extracted.returncode, extracted.stderr)
            fields = json.loads(extracted.stdout)
            self.assertEqual({'first': 1, 'second': 2}, {f['field_id']: f['page'] for f in fields})
            fields[1]['value'] = 'SECOND PAGE VALUE'
            values = path / 'values.json'
            values.write_text(json.dumps(fields))
            result = run_script('pdf', 'fill_fillable_fields.py', source, values, output)
            self.assertEqual(0, result.returncode, result.stderr)
            self.assertEqual('SECOND PAGE VALUE', PdfReader(output).get_fields()['second']['/V'])
            self.assertEqual('', PdfReader(source).get_fields()['second']['/V'])
            self.assertEqual([2], json.loads(result.stdout)['pages'])

    def test_repeated_field_updates_all_widget_pages(self):
        from pypdf import PdfReader
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            source, output = path / 'source.pdf', path / 'filled.pdf'
            self.create_form(source, repeated=True)
            extracted = run_script('pdf', 'extract_form_field_info.py', source)
            self.assertEqual(0, extracted.returncode, extracted.stderr)
            self.assertEqual([1, 2], [f['page'] for f in json.loads(extracted.stdout)])
            values = path / 'values.json'
            values.write_text(json.dumps([{'field_id': 'shared', 'page': 2, 'value': 'BOTH PAGES'}]))
            result = run_script('pdf', 'fill_fillable_fields.py', source, values, output)
            self.assertEqual(0, result.returncode, result.stderr)
            self.assertEqual([1, 2], json.loads(result.stdout)['pages'])
            reader = PdfReader(output)
            self.assertEqual('BOTH PAGES', reader.get_fields()['shared']['/V'])
            for page in reader.pages:
                self.assertIn(b'BOTH PAGES', page['/Annots'][0].get_object()['/AP']['/N'].get_data())

    def test_wrong_page_and_existing_output_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            source, output = path / 'source.pdf', path / 'filled.pdf'
            self.create_form(source)
            values = path / 'values.json'
            values.write_text(json.dumps([{'field_id': 'second', 'page': 1, 'value': 'bad'}]))
            result = run_script('pdf', 'fill_fillable_fields.py', source, values, output)
            self.assertNotEqual(0, result.returncode)
            self.assertIn('actual pages: [2]', result.stderr)
            self.assertFalse(output.exists())
            output.write_bytes(b'keep existing output')
            result = run_script('pdf', 'fill_fillable_fields.py', source, values, output)
            self.assertNotEqual(0, result.returncode)
            self.assertEqual(b'keep existing output', output.read_bytes())


if __name__ == '__main__':
    unittest.main()
