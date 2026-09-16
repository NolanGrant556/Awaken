package managedenv

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

// Import the curated libraries before publishing. Arbitrary CLI packages may
// deliberately have no importable root, so their installed metadata is checked
// here and their command entry is validated by PrepareCommand.
func (m *Manager) verifyPackages(ctx context.Context, f Manifest, requested Spec) error {
	b := Binding{Spec: f.Spec, Executables: f.Executables, Environment: f.Environment}
	if len(requested.Python) > 0 {
		modules := map[string]string{"openpyxl": "openpyxl", "python-docx": "docx", "python-pptx": "pptx", "pillow": "PIL", "pandas": "pandas", "pypdf": "pypdf", "pdfplumber": "pdfplumber", "reportlab": "reportlab", "markitdown": "markitdown"}
		packages, imports := []string{}, []string{}
		for name := range requested.Python {
			base := strings.Split(name, "[")[0]
			packages = append(packages, base)
			if module := modules[base]; module != "" {
				imports = append(imports, module)
			}
		}
		sort.Strings(packages)
		sort.Strings(imports)
		namesJSON, _ := json.Marshal(packages)
		importsJSON, _ := json.Marshal(imports)
		script := "import importlib, importlib.metadata, json, sys; names=json.loads(sys.argv[1]); modules=json.loads(sys.argv[2]); [importlib.metadata.distribution(n) for n in names]; [importlib.import_module(n) for n in modules]; print('packages verified')"
		if _, err := m.runWithBinding(ctx, f.Root, f.Executables["python"], &b, "-c", script, string(namesJSON), string(importsJSON)); err != nil {
			return err
		}
	}
	if len(requested.Node) > 0 {
		names := []string{}
		for name := range requested.Node {
			names = append(names, name)
		}
		sort.Strings(names)
		namesJSON, _ := json.Marshal(names)
		script := `const fs=require('node:fs'),path=require('node:path'); const known=new Set(['docx','pptxgenjs','react','react-dom','react-icons','sharp']); (async()=>{for(const name of JSON.parse(process.argv[1])){const p=path.join(process.env.AWAKEN_NODE_PACKAGE_ROOT,'node_modules',name,'package.json'); JSON.parse(fs.readFileSync(p,'utf8')); if(known.has(name)){require(name);await import(name);}} console.log('CJS and ESM verified')})().catch(e=>{console.error(e);process.exitCode=1});`
		if _, err := m.runWithBinding(ctx, f.Root, f.Executables["node"], &b, "-e", script, string(namesJSON)); err != nil {
			return err
		}
	}
	return nil
}
