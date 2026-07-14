package readartifact

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

type xlsxFixtureSheet struct {
	name   string
	relID  string
	target string
	// entryName is the real ZIP entry storing the worksheet XML.
	entryName string
	rows      [][]string
}

type xlsxFixture struct {
	sheets        []xlsxFixtureSheet
	sharedStrings []string
	// extraRels are appended verbatim inside the Relationships element.
	extraRels string
}

func writeXLSXFixture(t *testing.T, path string, fixture xlsxFixture) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}
	defer file.Close()
	zw := zip.NewWriter(file)

	writeEntry := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}

	writeEntry("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
</Types>`)
	writeEntry("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`)

	var workbookSheets strings.Builder
	var workbookRels strings.Builder
	for i, sheet := range fixture.sheets {
		fmt.Fprintf(&workbookSheets, `<sheet name="%s" sheetId="%d" r:id="%s"/>`, sheet.name, i+1, sheet.relID)
		fmt.Fprintf(&workbookRels, `<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="%s"/>`, sheet.relID, sheet.target)
	}
	workbookRels.WriteString(fixture.extraRels)

	writeEntry("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>`+workbookSheets.String()+`</sheets>
</workbook>`)
	writeEntry("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+workbookRels.String()+`</Relationships>`)

	if len(fixture.sharedStrings) > 0 {
		var shared strings.Builder
		for _, value := range fixture.sharedStrings {
			fmt.Fprintf(&shared, "<si><t>%s</t></si>", value)
		}
		writeEntry("xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="`+fmt.Sprint(len(fixture.sharedStrings))+`" uniqueCount="`+fmt.Sprint(len(fixture.sharedStrings))+`">`+shared.String()+`</sst>`)
	}

	for _, sheet := range fixture.sheets {
		entryName := sheet.entryName
		if entryName == "" {
			continue
		}
		var rows strings.Builder
		for rowIndex, row := range sheet.rows {
			fmt.Fprintf(&rows, `<row r="%d">`, rowIndex+1)
			for colIndex, cell := range row {
				ref := fmt.Sprintf("%c%d", 'A'+colIndex, rowIndex+1)
				fmt.Fprintf(&rows, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, cell)
			}
			rows.WriteString("</row>")
		}
		writeEntry(entryName, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>`+rows.String()+`</sheetData>
</worksheet>`)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
}

func executeReadArtifact(t *testing.T, root, input string) (string, error) {
	t.Helper()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	return (&ReadArtifactTool{}).Execute(ctx, input)
}

// A legal workbook relationship may use a package absolute target like
// "/xl/worksheets/sheet1.xml". The previous implementation concatenated
// "xl/" unconditionally and looked up "xl/xl/worksheets/sheet1.xml".
func TestReadXLSXAbsoluteRelationshipTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "absolute.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{{
			name:      "Summary",
			relID:     "rId1",
			target:    "/xl/worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
			rows:      [][]string{{"account", "balance"}, {"cash", "1200"}},
		}},
	})

	out, err := executeReadArtifact(t, root, `{"path":"absolute.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "[Sheet] Summary") || !strings.Contains(out, "cash") || !strings.Contains(out, "1200") {
		t.Fatalf("expected sheet content from absolute target, got %q", out)
	}
}

func TestReadXLSXRelativeRelationshipTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "relative.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{{
			name:      "Data",
			relID:     "rId1",
			target:    "worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
			rows:      [][]string{{"item", "qty"}, {"widget", "7"}},
		}},
	})

	out, err := executeReadArtifact(t, root, `{"path":"relative.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "[Sheet] Data") || !strings.Contains(out, "widget") {
		t.Fatalf("expected sheet content from relative target, got %q", out)
	}
}

func TestReadXLSXDotSegmentRelationshipTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dotted.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{{
			name:      "Dotted",
			relID:     "rId1",
			target:    "./worksheets/../worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
			rows:      [][]string{{"ok"}},
		}},
	})

	out, err := executeReadArtifact(t, root, `{"path":"dotted.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "[Sheet] Dotted") || !strings.Contains(out, "ok") {
		t.Fatalf("expected sheet content from dotted target, got %q", out)
	}
}

func TestReadXLSXMultipleSheetsMixedTargets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "multi.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{
			{
				name:      "First",
				relID:     "rId1",
				target:    "/xl/worksheets/sheet1.xml",
				entryName: "xl/worksheets/sheet1.xml",
				rows:      [][]string{{"alpha"}},
			},
			{
				name:      "Second",
				relID:     "rId2",
				target:    "worksheets/sheet2.xml",
				entryName: "xl/worksheets/sheet2.xml",
				rows:      [][]string{{"beta"}},
			},
			{
				name:      "Third",
				relID:     "rId3",
				target:    "worksheets/sheet3.xml",
				entryName: "xl/worksheets/sheet3.xml",
				rows:      [][]string{{"gamma"}},
			},
		},
	})

	out, err := executeReadArtifact(t, root, `{"path":"multi.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, expected := range []string{"[Sheet] First", "alpha", "[Sheet] Second", "beta", "[Sheet] Third", "gamma"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in output, got %q", expected, out)
		}
	}
}

func TestReadXLSXSharedStringsWithAbsoluteTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "shared.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sharedStrings: []string{"收入", "支出"},
		sheets: []xlsxFixtureSheet{{
			name:      "财务",
			relID:     "rId1",
			target:    "/xl/worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
		}},
	})
	// Rewrite the worksheet to use shared string cells.
	rewriteZipEntry(t, path, "xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
  </sheetData>
</worksheet>`)

	out, err := executeReadArtifact(t, root, `{"path":"shared.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "收入") || !strings.Contains(out, "支出") {
		t.Fatalf("expected shared string values, got %q", out)
	}
}

func TestReadXLSXNonStandardRelationshipIDs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nonstandard.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{{
			name:      "Odd",
			relID:     "workbookRelId_007",
			target:    "worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
			rows:      [][]string{{"custom-id"}},
		}},
	})

	out, err := executeReadArtifact(t, root, `{"path":"nonstandard.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "[Sheet] Odd") || !strings.Contains(out, "custom-id") {
		t.Fatalf("expected sheet resolved via non-standard relationship id, got %q", out)
	}
}

// External targets are URIs, not ZIP entries; they must be skipped rather
// than looked up (and must not fail the whole workbook).
func TestReadXLSXExternalRelationshipIgnored(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "external.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{{
			name:      "Real",
			relID:     "rId1",
			target:    "worksheets/sheet1.xml",
			entryName: "xl/worksheets/sheet1.xml",
			rows:      [][]string{{"present"}},
		}},
		extraRels: `<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/externalLink" Target="https://example.com/data.xlsx" TargetMode="External"/>`,
	})

	out, err := executeReadArtifact(t, root, `{"path":"external.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "present") {
		t.Fatalf("expected real sheet content, got %q", out)
	}
}

// Targets escaping the package root through ".." must be rejected, never
// resolved to a path outside the archive namespace.
func TestReadXLSXTargetEscapingPackageRootRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "escape.xlsx")
	writeXLSXFixture(t, path, xlsxFixture{
		sheets: []xlsxFixtureSheet{
			{
				name:      "Evil",
				relID:     "rId1",
				target:    "../../etc/passwd",
				entryName: "",
			},
			{
				name:      "Good",
				relID:     "rId2",
				target:    "worksheets/sheet2.xml",
				entryName: "xl/worksheets/sheet2.xml",
				rows:      [][]string{{"safe"}},
			},
		},
	})

	out, err := executeReadArtifact(t, root, `{"path":"escape.xlsx"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out, "Evil") {
		t.Fatalf("escaping target must be skipped, got %q", out)
	}
	if !strings.Contains(out, "safe") {
		t.Fatalf("expected remaining valid sheet, got %q", out)
	}
}

// An intact ZIP whose workbook variant is not handled by the lightweight
// reader must route to a specialized tool, never report invalid_or_corrupt.
func TestReadXLSXIntactUnsupportedVariantRoutesToSpecializedTool(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "variant.xlsx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}
	zw := zip.NewWriter(file)
	w, err := zw.Create("workbook/main.xml")
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}
	if _, err := w.Write([]byte("<workbook/>")); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close file: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&ReadArtifactTool{}).ExecuteOutcome(ctx, `{"path":"variant.xlsx"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "read_artifact_specialized_tool_required" {
		t.Fatalf("expected specialized_tool_required, got %+v", outcome)
	}
	if outcome.ErrorCode == "read_artifact_invalid_or_corrupt" {
		t.Fatalf("intact file must not be reported as corrupt: %+v", outcome)
	}
	found := false
	for _, option := range outcome.RecoveryOptions {
		if option == "use_specialized_artifact_tool" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected specialized tool recovery option: %+v", outcome.RecoveryOptions)
	}
}

// A file that is not even a valid ZIP archive is still reported as
// invalid_or_corrupt.
func TestReadXLSXBrokenZipStillReportsCorrupt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.xlsx")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatalf("failed to write broken file: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&ReadArtifactTool{}).ExecuteOutcome(ctx, `{"path":"broken.xlsx"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "read_artifact_invalid_or_corrupt" {
		t.Fatalf("expected invalid_or_corrupt for broken zip, got %+v", outcome)
	}
}

func TestResolveOPCRelationshipTargetTable(t *testing.T) {
	cases := []struct {
		name     string
		rel      xlsxRelationship
		expected string
		ok       bool
	}{
		{"package absolute", xlsxRelationship{Target: "/xl/worksheets/sheet1.xml"}, "xl/worksheets/sheet1.xml", true},
		{"relative", xlsxRelationship{Target: "worksheets/sheet1.xml"}, "xl/worksheets/sheet1.xml", true},
		{"dot segments", xlsxRelationship{Target: "./worksheets/../worksheets/sheet2.xml"}, "xl/worksheets/sheet2.xml", true},
		{"backslash separators", xlsxRelationship{Target: "worksheets\\sheet1.xml"}, "xl/worksheets/sheet1.xml", true},
		{"external", xlsxRelationship{Target: "https://example.com/x.xlsx", TargetMode: "External"}, "", false},
		{"external lowercase", xlsxRelationship{Target: "https://example.com/x.xlsx", TargetMode: "external"}, "", false},
		{"escape root", xlsxRelationship{Target: "../../secret.xml"}, "", false},
		{"absolute escape", xlsxRelationship{Target: "/../secret.xml"}, "", false},
		{"empty", xlsxRelationship{Target: "   "}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := resolveOPCRelationshipTarget("xl/workbook.xml", tc.rel)
			if ok != tc.ok || resolved != tc.expected {
				t.Fatalf("resolveOPCRelationshipTarget(%q) = (%q, %v), want (%q, %v)", tc.rel.Target, resolved, ok, tc.expected, tc.ok)
			}
		})
	}
}

// rewriteZipEntry rebuilds the archive replacing one entry's content.
func rewriteZipEntry(t *testing.T, path, entryName, content string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		reader.Close()
		t.Fatalf("failed to create temp zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for _, file := range reader.File {
		w, err := zw.Create(file.Name)
		if err != nil {
			t.Fatalf("failed to create entry %s: %v", file.Name, err)
		}
		if file.Name == entryName {
			if _, err := w.Write([]byte(content)); err != nil {
				t.Fatalf("failed to write replaced entry: %v", err)
			}
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("failed to open entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read entry %s: %v", file.Name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("failed to copy entry %s: %v", file.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close temp zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	reader.Close()
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("failed to replace zip: %v", err)
	}
}
