package managedenv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"awaken/src/networkroute"
)

type boundedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if left := 64000 - b.b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		b.b.Write(p)
	}
	return n, nil
}
func (b *boundedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

func (m *Manager) installEnv() ([]string, error) {
	base := []string{}
	for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if v := os.Getenv(key); v != "" {
			base = append(base, key+"="+v)
		}
	}
	home := filepath.Join(m.root, "staging", "installer-home")
	if e := os.MkdirAll(home, 0700); e != nil {
		return nil, e
	}
	base = append(base, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "TMPDIR="+filepath.Join(m.root, "staging"), "TMP="+filepath.Join(m.root, "staging"), "TEMP="+filepath.Join(m.root, "staging"), "PATH="+systemPATH(), "PYTHONNOUSERSITE=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "UV_CACHE_DIR="+filepath.Join(m.root, "cache", "python"), "UV_PYTHON_DOWNLOADS=never", "UV_NO_CONFIG=1", "PIP_DISABLE_PIP_VERSION_CHECK=1", "npm_config_cache="+filepath.Join(m.root, "cache", "npm"), "npm_config_userconfig="+filepath.Join(home, "empty-npmrc"), "npm_config_globalconfig="+filepath.Join(home, "empty-global-npmrc"), "npm_config_registry=https://registry.npmjs.org/")
	return networkroute.DefaultEnvironmentForChild(base)
}
func systemPATH() string {
	if os.PathListSeparator == ';' {
		return filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	return "/usr/bin:/bin:/usr/sbin:/sbin"
}
func (m *Manager) run(ctx context.Context, cwd string, l Launch, args ...string) (string, error) {
	return m.runWithBinding(ctx, cwd, l, nil, args...)
}

func (m *Manager) runWithBinding(ctx context.Context, cwd string, l Launch, binding *Binding, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, l.Executable, append(append([]string{}, l.Args...), args...)...)
	cmd.Dir = cwd
	env, e := m.installEnv()
	if e != nil {
		return "", e
	}
	cmd.Env = env
	for i, value := range cmd.Env {
		if strings.HasPrefix(value, "PATH=") {
			cmd.Env[i] = "PATH=" + filepath.Dir(l.Executable) + string(os.PathListSeparator) + systemPATH()
		}
	}
	if binding != nil {
		cmd.Env = binding.ExecutionEnvironment(cmd.Env)
	}
	var out, stderr boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	release, e := startOwned(cmd)
	if e != nil {
		return "", failure("environment_process_failed", fmt.Sprintf("cannot start %s: %v", filepath.Base(l.Executable), e))
	}
	defer release()
	if e = cmd.Wait(); e != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", failure("environment_validation_failed", fmt.Sprintf("%s exited unsuccessfully: %s", filepath.Base(l.Executable), safeOutput(stderr.String())))
	}
	if strings.TrimSpace(out.String()) == "" {
		return stderr.String(), nil
	}
	return out.String(), nil
}
func safeOutput(s string) string {
	lines := strings.Split(s, "\n")
	out := []string{}
	for _, l := range lines {
		lower := strings.ToLower(l)
		if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(l, "https://") {
			continue
		}
		out = append(out, l)
	}
	s = strings.Join(out, "\n")
	if len(s) > 4000 {
		s = s[:4000]
	}
	return strings.TrimSpace(s)
}

func (m *Manager) component(ctx context.Context, id string, op *Operation) (Manifest, error) {
	if removableComponent(id) {
		if err := m.migrateCLIOwnership(ctx); err != nil {
			return Manifest{}, err
		}
	}
	c, ok := m.catalog.Components[id]
	if !ok {
		return Manifest{}, failure("environment_source_unavailable", id)
	}
	a, hasAsset := c.Assets[m.target]
	key := digest([]any{id, c.Version, m.target, a.SHA256})
	var result Manifest
	err := m.locked(ctx, "component-"+key, func() error {
		all, e := m.instances()
		if e != nil {
			return e
		}
		for _, f := range all {
			if f.Kind == "component" && f.ComponentID == id && (f.Key == key || (removableComponent(id) && f.Version == c.Version && f.Source == "awaken-managed")) && m.healthy(f) {
				result = f
				return nil
			}
		}
		if exes := m.legacyExecutables(id); len(exes) > 0 {
			f, err := m.registerExternal(ctx, id, key, c, exes, "awaken-legacy")
			if err == nil {
				f.Source = "awaken-legacy"
				err = m.save(f)
				result = f
				return err
			}
		}
		if !hasAsset {
			if exes := m.externalExecutables(id); len(exes) > 0 {
				f, e := m.registerExternal(ctx, id, key, c, exes, "user-external")
				result = f
				return e
			}
			return failure("environment_platform_unsupported", id+" has no verified asset for "+m.target)
		}
		op.Component = id
		archive, e := m.download(ctx, a, op)
		if e != nil {
			return e
		}
		instance := newID()
		// The full key remains in the manifest; a short grouping directory leaves
		// room for relocatable native prefixes even with Unicode user homes.
		root := filepath.Join(m.root, "components", key[:16], instance)
		if e = ownedPath(m.root, root); e != nil {
			return e
		}
		if e = os.MkdirAll(root, 0700); e != nil {
			return e
		}
		if err := m.markPreparation(root); err != nil {
			return err
		}
		defer m.discardUnpublished(root)
		op.State = "preparing"
		op.Message = "正在准备运行组件"
		op.Installs++
		if e = m.record(op); e != nil {
			return e
		}
		if e = extract(ctx, archive, root, a); e != nil {
			return e
		}
		if e = m.relocatePrefixes(ctx, root); e != nil {
			return e
		}
		exes := map[string]Launch{}
		for name, relative := range a.Executables {
			path := filepath.Join(root, filepath.FromSlash(relative))
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil || !inside(root, resolved) {
				return failure("environment_validation_failed", "invalid component executable")
			}
			if e = os.Chmod(resolved, 0755); e != nil {
				return e
			}
			exes[name] = Launch{Executable: path}
		}
		if node, ok := exes["node"]; ok {
			for _, name := range []string{"npm", "npx"} {
				if entry, ok := exes[name]; ok {
					exes[name] = Launch{Executable: node.Executable, Args: []string{entry.Executable}}
				}
			}
		}
		f := Manifest{SchemaVersion: SchemaVersion, Kind: "component", Key: key, InstanceID: instance, ComponentID: id, Version: c.Version, Target: m.target, Source: "awaken-managed", Root: root, State: "ready", Executables: exes, CreatedAt: m.now(), LastUsedAt: m.now()}
		if gitHelper, ok := exes["git-remote-https"]; id == "git" && ok {
			f.Environment = map[string]string{"GIT_EXEC_PATH": filepath.Dir(gitHelper.Executable)}
		}
		f.Resources = map[string]string{}
		for name, relative := range a.Resources {
			path := filepath.Join(root, filepath.FromSlash(relative))
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || !inside(root, resolved) {
				return failure("environment_validation_failed", "resource escapes component")
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return failure("environment_validation_failed", "component resource missing")
			}
			f.Resources[name] = path
		}
		if id == "fonts" {
			font := f.Resources["font_regular"]
			if font == "" || f.Resources["license"] == "" {
				return failure("environment_validation_failed", "font or license missing")
			}
			data, err := os.ReadFile(font)
			if err != nil || len(data) < 4 || string(data[:4]) != "OTTO" {
				return failure("environment_validation_failed", "invalid OpenType font")
			}
			f.Environment = map[string]string{"AWAKEN_FONTS_DIR": filepath.Dir(font), "TYPST_FONT_PATHS": filepath.Dir(font)}
		}
		if id == "tesseract" {
			if f.Resources["ocr_eng"] == "" || f.Resources["ocr_chi_sim"] == "" {
				return failure("environment_validation_failed", "OCR language data missing")
			}
			f.Environment = map[string]string{"TESSDATA_PREFIX": filepath.Dir(f.Resources["ocr_eng"])}
		}
		if e = m.verifyComponent(ctx, c, f); e != nil {
			return e
		}
		f.Fingerprints = fingerprints(exes)
		for _, path := range f.Resources {
			if info, err := os.Stat(path); err == nil {
				f.Fingerprints[path] = fileIdentity(info)
			}
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			name := strings.ToLower(entry.Name())
			if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".dylib") || strings.Contains(name, ".so") || strings.HasSuffix(name, ".rdb") {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if info.Mode().IsRegular() {
					f.Fingerprints[path] = fileIdentity(info)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if e = m.save(f); e != nil {
			return e
		}
		result = f
		return nil
	})
	return result, err
}
func (m *Manager) verifyComponent(ctx context.Context, c Component, f Manifest) error {
	if c.Probe == "" {
		return nil
	}
	l, ok := f.Executables[c.Probe]
	if !ok {
		return failure("environment_validation_failed", "component probe entry is missing")
	}
	binding := Binding{Executables: f.Executables, Environment: f.Environment}
	var out string
	var e error
	if f.ComponentID == "browser-automation" && strings.HasPrefix(m.target, "windows-") {
		out, e = nativeFileVersion(l.Executable)
	} else {
		out, e = m.runWithBinding(ctx, f.Root, l, &binding, c.ProbeArgs...)
	}
	if e != nil {
		return e
	}
	if c.Version != "" && !strings.Contains(out, c.Version) {
		return failure("environment_validation_failed", "component version differs from catalog")
	}
	if f.ComponentID == "tesseract" {
		out, e = m.runWithBinding(ctx, f.Root, l, &binding, "--list-langs")
		if e != nil {
			return e
		}
		if !strings.Contains(out, "chi_sim") || !strings.Contains(out, "eng") {
			return failure("environment_validation_failed", "OCR language data could not be loaded")
		}
	}
	return nil
}
func (m *Manager) externalExecutables(id string) map[string]Launch {
	if m.target != Target() {
		return nil
	}
	candidates := map[string][]string{
		"libreoffice": {"/Applications/LibreOffice.app/Contents/MacOS/soffice"},
		"poppler":     {"/opt/homebrew/bin/pdftoppm", "/usr/local/bin/pdftoppm"},
		"git":         {"/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"},
		"ffmpeg":      {"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg"},
		"tesseract":   {"/opt/homebrew/bin/tesseract", "/usr/local/bin/tesseract"},
	}
	if strings.HasPrefix(m.target, "windows-") {
		candidates = map[string][]string{}
		for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			base := os.Getenv(variable)
			if !filepath.IsAbs(base) {
				continue
			}
			for component, relative := range map[string]string{"libreoffice": "LibreOffice/program/soffice.exe", "git": "Git/cmd/git.exe", "tesseract": "Tesseract-OCR/tesseract.exe", "poppler": "poppler/Library/bin/pdftoppm.exe", "ffmpeg": "ffmpeg/bin/ffmpeg.exe"} {
				candidates[component] = append(candidates[component], filepath.Join(base, filepath.FromSlash(relative)))
			}
		}
	}
	for _, p := range candidates[id] {
		if info, e := os.Stat(p); e != nil || !info.Mode().IsRegular() {
			continue
		}
		name := id
		if id == "libreoffice" {
			name = "soffice"
		}
		out := map[string]Launch{name: {Executable: p}}
		if id == "poppler" {
			out = map[string]Launch{}
			for _, n := range []string{"pdftoppm", "pdfinfo", "pdftotext"} {
				filename := n
				if strings.HasPrefix(m.target, "windows-") {
					filename += ".exe"
				}
				q := filepath.Join(filepath.Dir(p), filename)
				if info, e := os.Stat(q); e == nil && info.Mode().IsRegular() {
					out[n] = Launch{Executable: q}
				}
			}
		}
		return out
	}
	return nil
}
func (m *Manager) registerExternal(ctx context.Context, id, key string, c Component, exes map[string]Launch, source string) (Manifest, error) {
	root := filepath.Join(m.root, "components", key, newID())
	if e := os.MkdirAll(root, 0700); e != nil {
		return Manifest{}, e
	}
	f := Manifest{SchemaVersion: SchemaVersion, Kind: "component", Key: key, InstanceID: filepath.Base(root), ComponentID: id, Version: c.Version, Target: m.target, Source: source, Root: root, State: "ready", Executables: exes, CreatedAt: m.now(), LastUsedAt: m.now()}
	// External tools may have a different compatible version. Probe liveness;
	// never claim their version is the catalog version without observing it.
	if c.Probe != "" {
		l, ok := exes[c.Probe]
		if !ok {
			return f, failure("environment_validation_failed", "external probe entry missing")
		}
		out, e := m.run(ctx, root, l, c.ProbeArgs...)
		if e != nil {
			return f, e
		}
		if source == "awaken-legacy" && c.Version != "" && !strings.Contains(out, c.Version) {
			return f, failure("environment_version_mismatch", "legacy component version differs from its record")
		}
		f.Version = strings.TrimSpace(out)
	}
	f.Fingerprints = fingerprints(exes)
	return f, m.save(f)
}

func (m *Manager) install(ctx context.Context, s Spec, op *Operation) (Manifest, error) {
	if s.OfficeBundle != nil {
		return m.installOfficeBundle(ctx, s, op)
	}
	owner := Owner{ThreadID: "prepare:" + op.ID, AgentID: "installer", RunID: op.Attempt, Kind: "preparation"}
	defer m.ReleaseOwner(owner)
	exes := map[string]Launch{}
	componentEnvironment := map[string]string{}
	componentResources := map[string]string{}
	components := []string{}
	versions := map[string]string{}
	installComponents := append([]string(nil), s.Components...)
	if len(s.Python) > 0 && !contains(installComponents, "uv") {
		installComponents = append(installComponents, "uv")
	}
	for _, id := range installComponents {
		releasePreparation, e := m.holdComponentPreparation(id)
		if e != nil {
			return Manifest{}, e
		}
		f, e := m.component(ctx, id, op)
		if e != nil {
			releasePreparation()
			return Manifest{}, e
		}
		if e = m.registry(func(r *registry) error {
			current, ok := r.Instances[f.InstanceID]
			if !ok || !m.healthy(current) {
				return failure("environment_binding_stale", "component changed before preparation")
			}
			if err := m.checkRemoval(current, r.Instances, ""); err != nil {
				return err
			}
			current.LastUsedAt = m.now()
			current.RetiredAt = nil
			r.Instances[f.InstanceID] = current
			return m.writeLease(f.InstanceID, owner)
		}); e != nil {
			releasePreparation()
			return Manifest{}, e
		}
		releasePreparation()
		components = append(components, f.InstanceID)
		versions[id] = f.Version
		for name, value := range f.Environment {
			componentEnvironment[name] = value
		}
		for name, value := range f.Resources {
			componentResources[name] = value
		}
		for n, l := range f.Executables {
			exes[n] = l
		}
	}
	stage := filepath.Join(m.root, "staging", op.Attempt)
	if e := os.MkdirAll(stage, 0700); e != nil {
		return Manifest{}, e
	}
	defer os.RemoveAll(stage)
	op.State = "resolving"
	op.Message = "正在解析锁定依赖"
	if e := m.record(op); e != nil {
		return Manifest{}, e
	}
	lockPath := filepath.Join(m.root, "resolved", digest([]any{m.target, s, m.catalog.Components["python"].Version, m.catalog.Components["node"].Version})+".json")
	var saved resolvedLocks
	if err := readJSON(lockPath, &saved); err != nil && !os.IsNotExist(err) {
		return Manifest{}, failure("environment_lock_invalid", "saved dependency lock is unreadable")
	}
	if saved.Python == "" && saved.Node == "" {
		var err error
		saved, err = baselineLocks(s)
		if err != nil {
			return Manifest{}, err
		}
	}
	pyLock, nodeLock, packageJSON := saved.Python, saved.Node, saved.Package
	if len(s.Python) > 0 && pyLock == "" {
		lines := []string{}
		for name, version := range s.Python {
			if version != "" && !strings.ContainsAny(version, "<>=!~") {
				version = "==" + version
			}
			lines = append(lines, name+version)
		}
		sort.Strings(lines)
		in := filepath.Join(stage, "requirements.in")
		if e := os.WriteFile(in, []byte(strings.Join(lines, "\n")+"\n"), 0600); e != nil {
			return Manifest{}, e
		}
		if _, e := m.run(ctx, stage, exes["uv"], "pip", "compile", "--python", exes["python"].Executable, "--generate-hashes", "--no-header", "--no-annotate", "--only-binary", ":all:", "--index-url", "https://pypi.org/simple", in, "-o", filepath.Join(stage, "requirements.txt")); e != nil {
			return Manifest{}, e
		}
		b, e := os.ReadFile(filepath.Join(stage, "requirements.txt"))
		if e != nil {
			return Manifest{}, e
		}
		pyLock = string(b)
	}
	if len(s.Node) > 0 && nodeLock == "" {
		pk := map[string]any{"name": "awaken-managed-environment", "version": "1.0.0", "private": true, "dependencies": s.Node}
		for n, v := range s.Node {
			if v == "" {
				s.Node[n] = "*"
			}
		}
		if e := writeJSON(filepath.Join(stage, "package.json"), pk); e != nil {
			return Manifest{}, e
		}
		if _, e := m.run(ctx, stage, exes["npm"], "install", "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund"); e != nil {
			return Manifest{}, e
		}
		b, e := os.ReadFile(filepath.Join(stage, "package-lock.json"))
		if e != nil {
			return Manifest{}, e
		}
		nodeLock = string(b)
		b, e = os.ReadFile(filepath.Join(stage, "package.json"))
		if e != nil {
			return Manifest{}, e
		}
		packageJSON = string(b)
	}
	if err := writeJSON(lockPath, resolvedLocks{Python: pyLock, Node: nodeLock, Package: packageJSON}); err != nil {
		return Manifest{}, err
	}
	key := digest([]any{m.target, components, pyLock, nodeLock, "layout-v3"})
	instance := newID()
	root := filepath.Join(m.root, "environments", key, instance)
	if e := os.MkdirAll(root, 0700); e != nil {
		return Manifest{}, e
	}
	if err := m.markPreparation(root); err != nil {
		return Manifest{}, err
	}
	defer m.discardUnpublished(root)
	f := Manifest{SchemaVersion: SchemaVersion, Kind: "environment", Key: key, InstanceID: instance, Target: m.target, Source: "awaken-managed", Root: root, State: "ready", Spec: clone(s), Components: components, Executables: clone(exes), Environment: map[string]string{}, CreatedAt: m.now(), LastUsedAt: m.now(), LockDigest: digest([]string{pyLock, nodeLock})}
	f.NodeLauncherVersion = 3
	f.Environment = componentEnvironment
	f.Resources = componentResources
	f.Spec.ComponentVersions = versions
	op.State = "preparing"
	op.Component = "packages"
	op.Message = "正在安装所需依赖"
	if e := m.record(op); e != nil {
		return f, e
	}
	if pyLock != "" {
		venv := filepath.Join(root, "python")
		if _, e := m.run(ctx, root, exes["python"], "-m", "venv", "--without-pip", venv); e != nil {
			return f, e
		}
		py := filepath.Join(venv, "bin", "python")
		if strings.HasPrefix(m.target, "windows-") {
			py = filepath.Join(venv, "Scripts", "python.exe")
		}
		lockPath := filepath.Join(root, "requirements.txt")
		if e := os.WriteFile(lockPath, []byte(pyLock), 0600); e != nil {
			return f, e
		}
		op.Installs++
		if e := m.record(op); e != nil {
			return f, e
		}
		if _, e := m.run(ctx, root, exes["uv"], "pip", "sync", "--python", py, "--require-hashes", "--only-binary", ":all:", lockPath); e != nil {
			return f, e
		}
		if _, e := m.run(ctx, root, exes["uv"], "pip", "check", "--python", py); e != nil {
			return f, e
		}
		out, e := m.run(ctx, root, exes["uv"], "pip", "list", "--python", py, "--format", "json")
		if e != nil {
			return f, e
		}
		var list []struct{ Name, Version string }
		if e = json.Unmarshal([]byte(out), &list); e != nil {
			return f, e
		}
		f.Spec.Python = map[string]string{}
		for _, p := range list {
			f.Spec.Python[normalizePythonName(p.Name)] = p.Version
		}
		for name := range s.Python {
			base := strings.Split(name, "[")[0]
			if v, ok := f.Spec.Python[base]; ok {
				f.Spec.Python[name] = v
			}
		}
		f.Executables["python"] = Launch{Executable: py}
		f.Executables["python3"] = Launch{Executable: py}
		f.Environment["VIRTUAL_ENV"] = venv
	}
	if nodeLock != "" {
		nodeRoot := filepath.Join(root, "node")
		if e := os.MkdirAll(nodeRoot, 0700); e != nil {
			return f, e
		}
		for n, v := range map[string]string{"package.json": packageJSON, "package-lock.json": nodeLock} {
			if e := os.WriteFile(filepath.Join(nodeRoot, n), []byte(v), 0600); e != nil {
				return f, e
			}
		}
		op.Installs++
		if e := m.record(op); e != nil {
			return f, e
		}
		if _, e := m.run(ctx, nodeRoot, exes["npm"], "ci", "--ignore-scripts", "--no-audit", "--no-fund"); e != nil {
			return f, e
		}
		var lock struct {
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if e := json.Unmarshal([]byte(nodeLock), &lock); e != nil {
			return f, e
		}
		f.Spec.Node = map[string]string{}
		for name := range s.Node {
			v := lock.Packages["node_modules/"+name].Version
			if v == "" {
				return f, failure("environment_validation_failed", "package lock lacks requested Node dependency")
			}
			f.Spec.Node[name] = v
		}
		helper := filepath.Join(root, "node-resolver.cjs")
		if e := os.WriteFile(helper, []byte(nodeResolver), 0600); e != nil {
			return f, e
		}
		f.Environment["AWAKEN_NODE_PACKAGE_ROOT"] = nodeRoot
		f.Environment["NODE_OPTIONS"] = "--require " + fmt.Sprintf("%q", helper)
		f.Executables["node"] = Launch{Executable: exes["node"].Executable, Args: []string{"--require", helper}}
	}
	op.State = "verifying"
	op.Message = "正在验证执行环境"
	if e := m.record(op); e != nil {
		return f, e
	}
	if e := m.verifyPackages(ctx, f, s); e != nil {
		return f, e
	}
	f.Fingerprints = fingerprints(f.Executables)
	for _, path := range f.Resources {
		if info, err := os.Stat(path); err == nil {
			f.Fingerprints[path] = fileIdentity(info)
		}
	}
	// Track installed package files, excluding interpreter-generated bytecode.
	for _, directory := range []string{"python", "node/node_modules"} {
		base := filepath.Join(root, filepath.FromSlash(directory))
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f.Fingerprints[path] = fileIdentity(info)
			}
			return nil
		})
		if err != nil {
			return f, err
		}
	}
	for _, p := range []string{"requirements.txt", "node/package-lock.json", "node-resolver.cjs"} {
		p = filepath.Join(root, filepath.FromSlash(p))
		if info, e := os.Stat(p); e == nil {
			f.Fingerprints[p] = fileIdentity(info)
		}
	}
	if e := m.save(f); e != nil {
		return f, e
	}
	return f, nil
}

const nodeResolver = `'use strict';
const { registerHooks, isBuiltin, createRequire } = require('node:module');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const fs = require('node:fs');
const root = process.env.AWAKEN_NODE_PACKAGE_ROOT;
if (!root || !path.isAbsolute(root)) throw new Error('managed Node package root is missing');
const dependencies = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).dependencies || {};
const anchor = pathToFileURL(path.join(root, '__awaken_anchor__.cjs')).href;
const modules = pathToFileURL(path.join(root, 'node_modules') + path.sep).href;
const fromRoot = createRequire(anchor);
registerHooks({resolve(specifier, context, nextResolve) {
  const name = specifier.startsWith('@') ? specifier.split('/').slice(0,2).join('/') : specifier.split('/')[0];
  if (!isBuiltin(specifier) && dependencies[name] && context.parentURL !== anchor && !(context.parentURL || '').startsWith(modules)) {
    if (Array.from(context.conditions || []).includes('require')) return nextResolve(fromRoot.resolve(specifier), context);
    return nextResolve(specifier, {...context, parentURL: anchor});
  }
  return nextResolve(specifier, context);
}});
`
