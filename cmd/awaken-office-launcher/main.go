// awaken-office-launcher is shipped under each supported command name in the
// immutable Office bundle. It resolves only relative, in-bundle entry points.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type descriptor struct {
	LayoutVersion int               `json:"layout_version"`
	Executables   map[string]string `json:"executables"`
}

func resolve(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\\:") {
		return "", fmt.Errorf("invalid Office entry")
	}
	p, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("Office entry escapes bundle")
	}
	return p, nil
}

func main() {
	if err := launch(); err != nil {
		fmt.Fprintln(os.Stderr, "Awaken Office:", err)
		os.Exit(1)
	}
}

func launch() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(executable), "../../.."))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "office-runtime.json"))
	if err != nil {
		return err
	}
	var d descriptor
	if err = json.Unmarshal(data, &d); err != nil || d.LayoutVersion != 1 {
		return fmt.Errorf("unsupported Office bundle descriptor")
	}
	name := strings.TrimSuffix(filepath.Base(executable), ".exe")
	if name == "python3" {
		name = "python"
	}
	program, err := resolve(root, d.Executables[name])
	if err != nil {
		return err
	}
	if program == executable {
		return fmt.Errorf("recursive Office entry")
	}
	values := map[string]string{}
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			values[envKey(k)] = v
		}
	}
	for _, k := range []string{"PYTHONHOME", "PYTHONPATH", "PYTHONPYCACHEPREFIX", "VIRTUAL_ENV", "NODE_PATH", "NODE_OPTIONS"} {
		delete(values, k)
	}
	values["PYTHONNOUSERSITE"], values["PYTHONDONTWRITEBYTECODE"] = "1", "1"
	values["PYTHONUTF8"], values["PYTHONIOENCODING"] = "1", "utf-8"
	values["AWAKEN_NODE_PACKAGE_ROOT"] = filepath.Join(root, "dependencies/node-packages")
	values["TYPST_FONT_PATHS"] = filepath.Join(root, "dependencies/fonts")
	values["PATH"] = filepath.Join(root, "dependencies/bin/override") + string(os.PathListSeparator) + values["PATH"]
	if name == "node" {
		quoted, _ := json.Marshal(filepath.Join(root, "node-resolver.cjs"))
		values["NODE_OPTIONS"] = "--require " + string(quoted)
	}
	args := os.Args[1:]
	hasIgnoreSystemFonts := false
	for _, arg := range args {
		if arg == "--ignore-system-fonts" {
			hasIgnoreSystemFonts = true
		}
	}
	if name == "typst" && len(args) > 0 && (args[0] == "compile" || args[0] == "watch") && !hasIgnoreSystemFonts {
		args = append([]string{args[0], "--ignore-system-fonts"}, args[1:]...)
	}
	env := make([]string, 0, len(values))
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	return execute(program, args, env)
}

func envKey(k string) string {
	if os.PathListSeparator == ';' {
		return strings.ToUpper(k)
	}
	return k
}
