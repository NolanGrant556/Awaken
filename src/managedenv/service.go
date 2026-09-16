package managedenv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type PackageCommand struct {
	Ecosystem, Package, Version, Binary string
	Args                                []string
}

// RecognizePackageCommand accepts a deliberately small argv grammar. Shell
// expressions, custom paths and package-manager flags keep external semantics.
func RecognizePackageCommand(command string, args []string) (PackageCommand, bool) {
	if filepath.IsAbs(command) || strings.ContainsAny(command, "/\\") {
		return PackageCommand{}, false
	}
	command = strings.TrimSuffix(strings.ToLower(command), ".cmd")
	if command == "cmd" || command == "cmd.exe" {
		if len(args) < 3 || strings.ToLower(args[0]) != "/c" {
			return PackageCommand{}, false
		}
		return RecognizePackageCommand(args[1], args[2:])
	}
	p := PackageCommand{}
	switch command {
	case "npx":
		p.Ecosystem = "node"
		if len(args) > 0 && (args[0] == "-y" || args[0] == "--yes") {
			args = args[1:]
		}
	case "uvx":
		p.Ecosystem = "python"
	default:
		return p, false
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return p, false
	}
	name := args[0]
	p.Args = append([]string(nil), args[1:]...)
	if p.Ecosystem == "node" {
		if at := strings.LastIndex(name, "@"); at > 0 {
			p.Version = name[at+1:]
			name = name[:at]
		}
		if p.Version == "latest" {
			p.Version = ""
		}
		if !safeNode.MatchString(name) || !safeVersion.MatchString(p.Version) {
			return p, false
		}
		p.Binary = name[strings.LastIndex(name, "/")+1:]
	} else {
		if parts := strings.SplitN(name, "==", 2); len(parts) == 2 {
			name, p.Version = parts[0], parts[1]
		}
		if !safePython.MatchString(name) || !safeVersion.MatchString(p.Version) {
			return p, false
		}
		p.Binary = name
	}
	p.Package = name
	return p, true
}

// PrepareCommand obtains a service-owned lease before a process can start.
// The caller releases it after Wait/Close, including every startup failure.
func (m *Manager) PrepareCommand(ctx context.Context, command string, args, env []string, service string) (Launch, []string, func(), error) {
	original := Launch{Executable: command, Args: append([]string(nil), args...)}
	p, ok := RecognizePackageCommand(command, args)
	if !ok {
		return original, env, func() {}, nil
	}
	owner := Owner{ThreadID: "service:" + service, AgentID: "service", RunID: newID(), Kind: "service"}
	requirements := []Requirement{{Ecosystem: p.Ecosystem, Name: p.Package, Version: p.Version}}
	browser := needsManagedBrowser(p, env)
	if browser {
		requirements = append(requirements, Requirement{Ecosystem: "component", Name: "browser-automation"})
	}
	b, err := m.Ensure(ctx, Request{Requirements: requirements}, owner, nil)
	defer m.ReleaseRun(owner.RunID)
	if err != nil {
		return original, nil, nil, err
	}
	if browser {
		configureManagedBrowser(&p, b)
	}
	release, err := m.HoldProcess(b, 0)
	if err != nil {
		return original, nil, nil, err
	}
	fail := func(err error) (Launch, []string, func(), error) { release(); return original, nil, nil, err }
	var launch Launch
	if p.Ecosystem == "node" {
		root := b.Environment["AWAKEN_NODE_PACKAGE_ROOT"]
		packageRoot := filepath.Join(root, "node_modules", filepath.FromSlash(p.Package))
		var metadata struct {
			Bin json.RawMessage `json:"bin"`
		}
		if err := readJSON(filepath.Join(packageRoot, "package.json"), &metadata); err != nil {
			return fail(err)
		}
		entry := ""
		if json.Unmarshal(metadata.Bin, &entry) != nil {
			var entries map[string]string
			if json.Unmarshal(metadata.Bin, &entries) != nil {
				return fail(failure("environment_entrypoint_missing", "package has no executable entry"))
			}
			entry = entries[p.Binary]
			if entry == "" && len(entries) == 1 {
				for _, v := range entries {
					entry = v
				}
			}
		}
		target := filepath.Join(packageRoot, filepath.FromSlash(entry))
		resolved, e := filepath.EvalSymlinks(target)
		if entry == "" || e != nil || !inside(packageRoot, resolved) {
			return fail(failure("environment_entrypoint_missing", "package executable is unavailable"))
		}
		launch = b.Executables["node"]
		launch.Args = append(append(launch.Args, target), p.Args...)
	} else {
		directory := filepath.Dir(b.Executables["python"].Executable)
		name := p.Binary
		if strings.HasPrefix(m.target, "windows-") {
			name += ".exe"
		}
		path := filepath.Join(directory, name)
		if info, e := os.Stat(path); e != nil || !info.Mode().IsRegular() {
			return fail(failure("environment_entrypoint_missing", "Python package command is unavailable"))
		}
		launch = Launch{Executable: path, Args: p.Args}
	}
	launch.ManagedInstanceID = b.InstanceID
	return launch, b.ExecutionEnvironment(env), release, nil
}
