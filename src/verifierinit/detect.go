package verifierinit

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
)

type packageJSON struct {
	Name            string            `json:"name"`
	PackageManager  string            `json:"packageManager"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Bin             any               `json:"bin"`
	Main            string            `json:"main"`
	Module          string            `json:"module"`
	Exports         any               `json:"exports"`
}

func DetectAreas(files []ProjectFile) []Area {
	byPath := map[string]ProjectFile{}
	for _, file := range files {
		normalized := normalizePath(file.Path)
		if normalized == "" || ignoredPath(normalized) {
			continue
		}
		file.Path = normalized
		byPath[normalized] = file
	}

	var manifests []string
	for filePath := range byPath {
		if isManifestPath(filePath) {
			manifests = append(manifests, filePath)
		}
	}
	sort.Strings(manifests)

	areas := make([]Area, 0, len(manifests))
	for _, manifestPath := range manifests {
		file := byPath[manifestPath]
		dir := path.Dir(manifestPath)
		if dir == "." {
			dir = "."
		}

		var area Area
		switch path.Base(manifestPath) {
		case "package.json":
			area = detectPackageJSONArea(dir, manifestPath, file.Content, byPath)
		case "go.mod":
			area = detectGoArea(dir, manifestPath, file.Content)
		case "pyproject.toml":
			area = detectPythonArea(dir, manifestPath, file.Content)
		case "Cargo.toml":
			area = detectRustArea(dir, manifestPath, file.Content)
		}
		if area.Name == "" {
			area.Name = fallbackAreaName(dir)
		}
		if area.Path == "" {
			area.Path = dir
		}
		if area.Kind == "" {
			area.Kind = AreaUnknown
		}
		areas = append(areas, area)
	}

	return areas
}

func DetectToolState(files []ProjectFile) ToolState {
	var tools ToolState
	for _, file := range files {
		normalized := normalizePath(file.Path)
		base := path.Base(normalized)
		lowerContent := strings.ToLower(file.Content)
		if base == "package.json" || base == "pyproject.toml" {
			if strings.Contains(lowerContent, "playwright") {
				tools.PlaywrightPackage = true
			}
		}
		if base == ".mcp.json" {
			if strings.Contains(lowerContent, "playwright") {
				tools.PlaywrightMCP = true
			}
			if strings.Contains(lowerContent, "chrome-devtools") || strings.Contains(lowerContent, "chromedevtools") {
				tools.ChromeDevToolsMCP = true
			}
			if strings.Contains(lowerContent, "chrome extension") || strings.Contains(lowerContent, "browser-use") {
				tools.ClaudeChromeExtensionMCP = true
			}
		}
	}
	return tools
}

func detectPackageJSONArea(dir, manifestPath, content string, files map[string]ProjectFile) Area {
	var pkg packageJSON
	_ = json.Unmarshal([]byte(content), &pkg)

	deps := mergeDependencyNames(pkg.Dependencies, pkg.DevDependencies)
	scripts := lowerScriptText(pkg.Scripts)
	stack := detectNodeStack(deps, scripts)
	pm := packageManagerFor(dir, pkg.PackageManager, files)

	area := Area{
		Name:           cleanPackageName(pkg.Name),
		Path:           dir,
		Manifest:       manifestPath,
		PackageManager: pm,
		Stack:          stack,
		HasPlaywright:  hasAnyDependency(deps, "playwright", "@playwright/test") || strings.Contains(scripts, "playwright"),
	}

	switch {
	case isNodeWebApp(deps, scripts):
		area.Kind = AreaWebApp
		area.DevCommand = packageRunCommand(pm, preferredScript(pkg.Scripts, "dev", "start", "serve"))
		area.BaseURL = defaultWebURL(deps, scripts)
		area.ReadySignal = defaultReadySignal(deps, scripts)
	case isNodeAPI(deps, scripts, area.Name, dir):
		area.Kind = AreaAPI
		area.DevCommand = packageRunCommand(pm, preferredScript(pkg.Scripts, "dev", "start", "server"))
		area.BaseURL = defaultAPIURL(scripts)
		area.ReadySignal = "listening"
	case isNodeCLI(pkg, deps, scripts, area.Name, dir):
		area.Kind = AreaCLI
		area.EntryCommand = defaultCLICommand(pm, pkg)
	default:
		area.Kind = AreaSharedLib
	}

	return area
}

func detectGoArea(dir, manifestPath, content string) Area {
	name := moduleName(content)
	area := Area{
		Name:           name,
		Path:           dir,
		Manifest:       manifestPath,
		PackageManager: "go",
		Stack:          []string{"Go"},
	}

	lower := strings.ToLower(content + "\n" + dir + "\n" + name)
	switch {
	case containsAny(lower, "gin-gonic/gin", "go-chi/chi", "gorilla/mux", "labstack/echo", "gofiber/fiber", "connectrpc", "/api", "backend", "server"):
		area.Kind = AreaAPI
		area.DevCommand = "go run ."
		area.BaseURL = "http://localhost:8080"
		area.ReadySignal = "listening"
	case containsAny(lower, "spf13/cobra", "urfave/cli", "/cmd/", "-cli", " cli"):
		area.Kind = AreaCLI
		area.EntryCommand = "go run ."
	default:
		area.Kind = AreaSharedLib
	}
	return area
}

func detectPythonArea(dir, manifestPath, content string) Area {
	name := pyprojectName(content)
	area := Area{
		Name:           name,
		Path:           dir,
		Manifest:       manifestPath,
		PackageManager: "pip",
		Stack:          []string{"Python"},
	}

	lower := strings.ToLower(content + "\n" + dir + "\n" + name)
	switch {
	case containsAny(lower, "fastapi", "flask", "django", "starlette", "sanic", "litestar"):
		area.Kind = AreaAPI
		area.DevCommand = "python -m uvicorn app:app --reload"
		area.BaseURL = "http://localhost:8000"
		area.ReadySignal = "application startup complete"
	case containsAny(lower, "typer", "click", "argparse", "console_scripts", "scripts"):
		area.Kind = AreaCLI
		area.EntryCommand = "python -m " + fallbackAreaName(dir)
	default:
		area.Kind = AreaSharedLib
	}
	return area
}

func detectRustArea(dir, manifestPath, content string) Area {
	name := cargoName(content)
	area := Area{
		Name:           name,
		Path:           dir,
		Manifest:       manifestPath,
		PackageManager: "cargo",
		Stack:          []string{"Rust"},
	}

	lower := strings.ToLower(content + "\n" + dir + "\n" + name)
	switch {
	case containsAny(lower, "axum", "actix-web", "rocket", "warp", "poem"):
		area.Kind = AreaAPI
		area.DevCommand = "cargo run"
		area.BaseURL = "http://localhost:8000"
		area.ReadySignal = "listening"
	case containsAny(lower, "clap", "structopt", "[[bin]]", "-cli", " cli"):
		area.Kind = AreaCLI
		area.EntryCommand = "cargo run --"
	default:
		area.Kind = AreaSharedLib
	}
	return area
}

func normalizePath(filePath string) string {
	filePath = strings.TrimSpace(strings.ReplaceAll(filePath, "\\", "/"))
	filePath = strings.TrimPrefix(filePath, "./")
	filePath = path.Clean(filePath)
	if filePath == "." {
		return ""
	}
	return filePath
}

func ignoredPath(filePath string) bool {
	parts := strings.Split(filePath, "/")
	for _, part := range parts {
		switch part {
		case "node_modules", "vendor", ".git", "dist", "build", "target", "__pycache__":
			return true
		}
	}
	return false
}

func isManifestPath(filePath string) bool {
	switch path.Base(filePath) {
	case "package.json", "go.mod", "pyproject.toml", "Cargo.toml":
		return true
	default:
		return false
	}
}

func mergeDependencyNames(groups ...map[string]string) map[string]bool {
	deps := map[string]bool{}
	for _, group := range groups {
		for name := range group {
			deps[strings.ToLower(name)] = true
		}
	}
	return deps
}

func lowerScriptText(scripts map[string]string) string {
	var parts []string
	for name, script := range scripts {
		parts = append(parts, strings.ToLower(name+" "+script))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func detectNodeStack(deps map[string]bool, scripts string) []string {
	candidates := []struct {
		key   string
		label string
	}{
		{"next", "Next.js"},
		{"react", "React"},
		{"vue", "Vue"},
		{"@vue/", "Vue"},
		{"svelte", "Svelte"},
		{"@angular/core", "Angular"},
		{"vite", "Vite"},
		{"astro", "Astro"},
		{"nuxt", "Nuxt"},
		{"express", "Express"},
		{"fastify", "Fastify"},
		{"@nestjs/core", "NestJS"},
		{"hono", "Hono"},
		{"commander", "Commander"},
		{"yargs", "Yargs"},
		{"@oclif/core", "oclif"},
	}

	var stack []string
	seen := map[string]bool{}
	for _, candidate := range candidates {
		matched := hasDependencyPrefix(deps, candidate.key) || strings.Contains(scripts, candidate.key)
		if matched && !seen[candidate.label] {
			stack = append(stack, candidate.label)
			seen[candidate.label] = true
		}
	}
	if len(stack) == 0 {
		stack = append(stack, "Node.js")
	}
	return stack
}

func isNodeWebApp(deps map[string]bool, scripts string) bool {
	return hasAnyDependency(deps,
		"next",
		"react",
		"vue",
		"@vue/cli-service",
		"svelte",
		"@sveltejs/kit",
		"@angular/core",
		"@remix-run/react",
		"astro",
		"nuxt",
		"vite",
	) || containsAny(scripts, "vite", "next dev", "nuxt", "astro dev", "svelte-kit", "webpack serve", "ng serve")
}

func isNodeAPI(deps map[string]bool, scripts, name, dir string) bool {
	return hasAnyDependency(deps,
		"express",
		"fastify",
		"koa",
		"@hapi/hapi",
		"@nestjs/core",
		"hono",
		"apollo-server",
		"graphql-yoga",
	) || containsAny(scripts+"\n"+name+"\n"+dir, "api", "server", "backend")
}

func isNodeCLI(pkg packageJSON, deps map[string]bool, scripts, name, dir string) bool {
	return pkg.Bin != nil ||
		hasAnyDependency(deps, "commander", "yargs", "@oclif/core", "clipanion") ||
		containsAny(scripts+"\n"+name+"\n"+dir, " cli", "-cli", "bin/", "node ./cli", "tsx ./cli")
}

func packageManagerFor(dir, packageManagerField string, files map[string]ProjectFile) string {
	if packageManagerField != "" {
		return strings.Split(packageManagerField, "@")[0]
	}
	lockFiles := []struct {
		name string
		pm   string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
		{"package-lock.json", "npm"},
	}
	for _, lock := range lockFiles {
		candidate := lock.name
		if dir != "." {
			candidate = path.Join(dir, lock.name)
		}
		if _, ok := files[candidate]; ok {
			return lock.pm
		}
	}
	return "npm"
}

func preferredScript(scripts map[string]string, names ...string) string {
	for _, name := range names {
		if _, ok := scripts[name]; ok {
			return name
		}
	}
	return ""
}

func packageRunCommand(pm, script string) string {
	if script == "" {
		return ""
	}
	switch pm {
	case "yarn":
		return "yarn " + script
	case "bun":
		return "bun run " + script
	default:
		return pm + " run " + script
	}
}

func defaultCLICommand(pm string, pkg packageJSON) string {
	if pkg.Name != "" {
		switch pm {
		case "yarn":
			return "yarn " + cleanPackageName(pkg.Name)
		case "pnpm":
			return "pnpm exec " + cleanPackageName(pkg.Name)
		case "bun":
			return "bunx " + cleanPackageName(pkg.Name)
		default:
			return "npx " + cleanPackageName(pkg.Name)
		}
	}
	return packageRunCommand(pm, "start")
}

func defaultWebURL(deps map[string]bool, scripts string) string {
	if strings.Contains(scripts, "localhost:") {
		return "http://localhost:" + firstPortAfter(scripts, "localhost:")
	}
	if containsAny(scripts, "--port 3000", "-p 3000") || hasAnyDependency(deps, "next", "nuxt") {
		return "http://localhost:3000"
	}
	if hasAnyDependency(deps, "astro") {
		return "http://localhost:4321"
	}
	if hasAnyDependency(deps, "@angular/core") {
		return "http://localhost:4200"
	}
	return "http://localhost:5173"
}

func defaultAPIURL(scripts string) string {
	if strings.Contains(scripts, "localhost:") {
		return "http://localhost:" + firstPortAfter(scripts, "localhost:")
	}
	return "http://localhost:3000"
}

func defaultReadySignal(deps map[string]bool, scripts string) string {
	switch {
	case hasAnyDependency(deps, "next"):
		return "ready"
	case hasAnyDependency(deps, "vite", "vue", "react", "svelte", "astro") || strings.Contains(scripts, "vite"):
		return "Local:"
	default:
		return "listening"
	}
}

func firstPortAfter(text, marker string) string {
	index := strings.Index(text, marker)
	if index < 0 {
		return "3000"
	}
	start := index + len(marker)
	var digits []byte
	for i := start; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			break
		}
		digits = append(digits, text[i])
	}
	if len(digits) == 0 {
		return "3000"
	}
	return string(digits)
}

func hasAnyDependency(deps map[string]bool, names ...string) bool {
	for _, name := range names {
		if deps[strings.ToLower(name)] {
			return true
		}
	}
	return false
}

func hasDependencyPrefix(deps map[string]bool, prefix string) bool {
	for dep := range deps {
		if strings.HasPrefix(dep, prefix) {
			return true
		}
	}
	return false
}

func containsAny(text string, needles ...string) bool {
	text = strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func cleanPackageName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "@") {
		parts := strings.Split(name, "/")
		return parts[len(parts)-1]
	}
	return name
}

func fallbackAreaName(dir string) string {
	if dir == "." || dir == "" {
		return "project"
	}
	return path.Base(dir)
}

func moduleName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			parts := strings.Split(mod, "/")
			return parts[len(parts)-1]
		}
	}
	return ""
}

func pyprojectName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
			return trimTOMLValue(line)
		}
	}
	return ""
}

func cargoName(content string) string {
	inPackage := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		if inPackage && strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
			return trimTOMLValue(line)
		}
	}
	return ""
}

func trimTOMLValue(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
}
