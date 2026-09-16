package managedenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNodeLauncherConditionalExportsAndRelativeImports(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is only required for this launcher integration test")
	}
	if err = exec.Command(node, "-e", `if(!require('node:module').registerHooks) process.exit(1)`).Run(); err != nil {
		t.Skip("Node registerHooks unavailable")
	}
	root := t.TempDir()
	packages := filepath.Join(root, "共享 包")
	project := filepath.Join(root, "业务 项目")
	files := map[string]string{
		filepath.Join(packages, "package.json"):                                  `{"dependencies":{"conditional-fixture":"1.0.0"}}`,
		filepath.Join(packages, "node_modules/conditional-fixture/package.json"): `{"name":"conditional-fixture","type":"module","exports":{"import":"./esm.js","require":"./cjs.cjs"}}`,
		filepath.Join(packages, "node_modules/conditional-fixture/esm.js"):       `import x from './sibling.js'; export default 'esm-'+x;`,
		filepath.Join(packages, "node_modules/conditional-fixture/sibling.js"):   `export default 'relative';`,
		filepath.Join(packages, "node_modules/conditional-fixture/cjs.cjs"):      `module.exports='cjs';`,
		filepath.Join(root, "node-resolver.cjs"):                                 nodeResolver,
		filepath.Join(project, "sibling.mjs"):                                    `export default 'project';`,
		filepath.Join(project, "entry.mjs"):                                      `import lib from 'conditional-fixture'; import sibling from './sibling.mjs'; import {createRequire} from 'node:module'; const require=createRequire(import.meta.url); if(lib!=='esm-relative'||require('conditional-fixture')!=='cjs'||sibling!=='project')throw Error('wrong resolution');`,
	}
	for p, value := range files {
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	binding := Binding{Executables: map[string]Launch{"node": {Executable: node}}, Environment: map[string]string{"AWAKEN_NODE_PACKAGE_ROOT": packages}}
	cmd := exec.Command(node, "--require", filepath.Join(root, "node-resolver.cjs"), filepath.Join(project, "entry.mjs"))
	cmd.Dir = project
	cmd.Env = binding.ExecutionEnvironment(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("conditional exports: %v %s", err, out)
	}
	if _, err = os.Stat(filepath.Join(project, "node_modules")); !os.IsNotExist(err) {
		t.Fatal("project modules were created")
	}
}
