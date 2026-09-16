package managedenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in public asset acceptance uses an explicit isolated root, never HOME.
func TestPublicOfficeEnvironment(t *testing.T) {
	root := os.Getenv("AWAKEN_MANAGEDENV_TEST_ROOT")
	if root == "" {
		t.Skip("set AWAKEN_MANAGEDENV_TEST_ROOT for public asset acceptance")
	}
	catalog, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m := managerForTest(t, Options{Root: root, Catalog: catalog})
	capabilities := os.Getenv("AWAKEN_MANAGEDENV_TEST_CAPABILITIES")
	if capabilities == "" {
		capabilities = "excel.create_edit,node-office,pdf_max"
	}
	for _, capability := range strings.Split(capabilities, ",") {
		t.Run(capability, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			last := ""
			b, e := m.Ensure(ctx, Request{Capability: capability}, testOwner("项目 A"), func(op Operation) {
				key := op.State + op.Component
				if key != last {
					t.Log(op.State, op.Component, op.Message)
					last = key
				}
			})
			if e != nil {
				t.Fatal(e)
			}
			project := filepath.Join(root, "fixtures", "项目 B 空格")
			if e = os.MkdirAll(project, 0700); e != nil {
				t.Fatal(e)
			}
			launch := b.Executables["python"]
			args := []string{"-c", "import docx, openpyxl, pandas, pptx, PIL, pypdf, pdfplumber, reportlab; print('office imports ready')"}
			if capability == "excel.create_edit" {
				args = []string{"-c", "import openpyxl; w=openpyxl.Workbook(); w.active['A1']='共享环境'; w.save('环境验证.xlsx'); print('xlsx generated')"}
			}
			if capability == "node-office" {
				launch = b.Executables["node"]
				args = []string{"--input-type=module", "-e", "import {createRequire} from 'node:module'; import * as docx from 'docx'; import pptxgen from 'pptxgenjs'; const require=createRequire(import.meta.url); if(!require('docx').Document || !docx.Document || !pptxgen) throw Error('modules missing'); console.log('CJS and ESM ready')"}
			}
			if capability == "pdf_max" {
				launch = b.Executables["typst"]
				args = []string{"--version"}
			}
			cmd := exec.CommandContext(ctx, launch.Executable, append(launch.Args, args...)...)
			cmd.Dir = project
			cmd.Env = b.ExecutionEnvironment([]string{"PATH=" + systemPATH(), "HOME=" + root})
			output, e := cmd.CombinedOutput()
			if e != nil {
				t.Fatalf("launch failed: %v: %s", e, output)
			}
			t.Log(strings.TrimSpace(string(output)), b.InstanceID)
			again, e := m.Ensure(ctx, Request{Capability: capability}, testOwner("项目 B"), nil)
			if e != nil || !again.Reused || again.InstanceID != b.InstanceID {
				t.Fatalf("reuse failed: %+v %v", again, e)
			}
			for _, name := range []string{"node_modules", ".venv", "package-lock.json"} {
				if _, e := os.Stat(filepath.Join(project, name)); !os.IsNotExist(e) {
					t.Fatalf("project dependency leak: %s", name)
				}
			}
		})
	}
}
