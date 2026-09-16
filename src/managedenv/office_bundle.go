package managedenv

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OfficeBundleIdentity participates in matching. A collection of old package
// environments can never stand in for a verified office bundle.
type OfficeBundleIdentity struct {
	Version       string `json:"version"`
	LayoutVersion int    `json:"layout_version"`
	SHA256        string `json:"sha256"`
}

type OfficeBundleRelease struct {
	Version             string            `json:"version"`
	LayoutVersion       int               `json:"layout_version"`
	Spec                Spec              `json:"spec"`
	Assets              map[string]Asset  `json:"assets"`
	MinimumOS           map[string]string `json:"minimum_os,omitempty"`
	MinimumCoreProtocol int               `json:"minimum_core_protocol,omitempty"`
}

const officeRegistrySchema = 2

var officeEntries = []string{"python", "node", "soffice", "pdftoppm", "pdfinfo", "pdftotext", "pdffonts", "pdfimages", "pdfseparate", "pdfunite", "pdftocairo", "pdftohtml", "pdftops", "pdfattach", "pdfdetach", "typst"}
var officeResources = []string{"python_packages", "node_packages", "fonts", "override", "fallback", "licenses"}

func (m *Manager) officeRequirements() (Spec, error) {
	release := m.catalog.OfficeBundle
	if release == nil {
		return Spec{}, failure("environment_source_unavailable", "固定办公包尚未配置发行资源；不能用系统软件或旧细分环境冒充就绪")
	}
	if !safeID.MatchString(release.Version) || release.LayoutVersion != 1 || release.Spec.OfficeBundle != nil {
		return Spec{}, failure("environment_schema_unsupported", "unsupported office bundle version or layout")
	}
	if release.MinimumCoreProtocol > 1 {
		return Spec{}, failure("environment_schema_unsupported", "办公环境要求更新版本的 Awaken")
	}
	if minimum := release.MinimumOS[strings.Split(m.target, "-")[0]]; minimum != "" && m.target == Target() {
		current, err := officeHostOSVersion()
		if err != nil || !satisfies(current, ">="+minimum) {
			return Spec{}, failure("environment_platform_unsupported", "办公环境最低系统版本为 "+minimum+"；当前系统为 "+current)
		}
	}
	// The Office contract is independent of legacy capability aliases.
	minimum := Spec{Components: []string{"python", "node", "libreoffice", "poppler", "fonts", "typst"},
		Python: map[string]string{"python-docx": "", "openpyxl": "", "pandas": "", "python-pptx": "", "pillow": "", "pypdf": "", "pdfplumber": "", "reportlab": "", "markitdown[pptx,xlsx]": ""},
		Node:   map[string]string{"docx": "", "pptxgenjs": ""}}
	if !specContains(release.Spec, minimum) {
		return Spec{}, failure("environment_source_unavailable", "incomplete Office runtime contract")
	}
	for _, id := range release.Spec.Components {
		component, ok := m.catalog.Components[id]
		if !ok || release.Spec.ComponentVersions[id] != component.Version {
			return Spec{}, failure("environment_source_unavailable", "office component version must match product lock: "+id)
		}
	}
	spec := clone(release.Spec)
	hash := release.Assets[m.target].SHA256
	if hash != "" {
		if _, err := hex.DecodeString(hash); err != nil || len(hash) != 64 {
			return Spec{}, failure("environment_source_unavailable", "invalid Office archive identity")
		}
	}
	spec.OfficeBundle = &OfficeBundleIdentity{release.Version, release.LayoutVersion, hash}
	return spec, nil
}

// Download metadata is required only when a new instance must be prepared.
func (m *Manager) officeSpec() (Spec, error) {
	spec, err := m.officeRequirements()
	if err != nil {
		return Spec{}, err
	}
	release := m.catalog.OfficeBundle
	asset, ok := release.Assets[m.target]
	_, hashErr := hex.DecodeString(asset.SHA256)
	if !ok || asset.URL == "" || len(asset.SHA256) != 64 || hashErr != nil {
		return Spec{}, failure("environment_source_unavailable", "固定办公包尚未提供当前平台的已校验资源")
	}
	if (asset.Format != "tar.gz" && asset.Format != "zip") || asset.MaxBytes <= 0 || asset.MaxExpandedBytes <= 0 || asset.MaxExpandedBytes > 8<<30 {
		return Spec{}, failure("environment_source_unavailable", "office bundle needs archive format and bounded download/extraction sizes")
	}
	for _, name := range officeEntries {
		if asset.Executables[name] == "" {
			return Spec{}, failure("environment_source_unavailable", "office bundle lacks executable "+name)
		}
	}
	for _, name := range officeResources {
		if asset.Resources[name] == "" {
			return Spec{}, failure("environment_source_unavailable", "office bundle lacks resource "+name)
		}
	}
	return spec, nil
}

func officeCompatible(f Manifest, required Spec) bool {
	have, want := f.Spec.OfficeBundle, required.OfficeBundle
	if f.Kind != "office" || f.Source != "awaken-managed" || have == nil || want == nil || have.Version != want.Version || have.LayoutVersion != want.LayoutVersion {
		return false
	}
	if _, err := hex.DecodeString(have.SHA256); err != nil || len(have.SHA256) != 64 {
		return false
	}
	if want.SHA256 != "" && have.SHA256 != want.SHA256 {
		return false
	}
	required.OfficeBundle = nil
	return specContains(f.Spec, required)
}

func (m *Manager) installedOfficeBundle(required Spec) (Manifest, error) {
	choice, err := m.officeChoice()
	if err != nil {
		return Manifest{}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		all, err := m.instances()
		if err != nil {
			return Manifest{}, err
		}
		for _, id := range []string{choice.Current, choice.Previous} {
			for _, f := range all {
				if f.InstanceID == id && officeCompatible(f, required) && m.reusable(f) {
					return f, nil
				}
			}
		}
		if attempt == 0 {
			if err := m.recoverRegistry(); err != nil {
				return Manifest{}, err
			}
			choice, err = m.officeChoice()
			if err != nil {
				return Manifest{}, err
			}
		}
	}
	return Manifest{}, failure("environment_missing", "compatible Office bundle is not ready")
}

// InspectOfficeBundle does not acquire a binding or prepare resources.
func (m *Manager) InspectOfficeBundle(ctx context.Context) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	spec, err := m.officeRequirements()
	if err != nil {
		return Manifest{}, err
	}
	return m.installedOfficeBundle(spec)
}

func (m *Manager) EnsureOfficeBundle(ctx context.Context, owner Owner, progress Progress) (binding Binding, err error) {
	defer func() { err = classifyPreparationError(err) }()
	if !owner.valid() {
		return Binding{}, failure("environment_owner_invalid", "host execution identity is required")
	}
	// Keep a healthy task binding. A damaged binding goes through the same
	// preparation flow as a missing environment; history alone never prepares.
	m.invalidateHealth()
	ref, err := m.boundOfficeReference(owner)
	if err != nil {
		return Binding{}, err
	}
	if ref != "" {
		existing, err := m.Binding(ctx, ref, owner)
		var stale *Error
		if !errors.As(err, &stale) || stale.Code != "environment_binding_stale" {
			return existing, err
		}
	}
	required, err := m.officeRequirements()
	if err != nil {
		return Binding{}, err
	}
	if installed, lookupErr := m.installedOfficeBundle(required); lookupErr == nil {
		binding, err = m.bind(installed, owner, true)
	} else {
		var missing *Error
		if !errors.As(lookupErr, &missing) || missing.Code != "environment_missing" {
			return Binding{}, lookupErr
		}
		spec, sourceErr := m.officeSpec()
		if sourceErr != nil {
			return Binding{}, sourceErr
		}
		binding, err = m.ensureSpec(ctx, spec, owner, progress)
	}
	if err == nil && ref != "" && ref != binding.EnvironmentRef {
		err = m.supersedeBinding(owner, ref, binding.EnvironmentRef)
	}
	return binding, err
}

func bundlePath(root, relative string, directory bool) (string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", failure("environment_path_invalid", "bundle root is unavailable")
	}
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || strings.Contains(relative, ":") || contains(strings.Split(relative, "/"), "..") {
		return "", failure("environment_path_invalid", "bundle entry must be a relative portable path")
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !inside(canonicalRoot, resolved) {
		return "", failure("environment_path_invalid", "bundle entry is absent or escapes instance")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return "", failure("environment_path_invalid", "invalid bundle entry type")
	}
	return path, nil
}

func (m *Manager) installOfficeBundle(ctx context.Context, spec Spec, op *Operation) (Manifest, error) {
	release := m.catalog.OfficeBundle
	if release == nil {
		return Manifest{}, failure("environment_source_unavailable", "office release unavailable")
	}
	asset := release.Assets[m.target]
	op.Component = "office-bundle"
	// A complete manifest can survive a failed selection commit. Activate it
	// again without downloading or extracting another copy.
	all, err := m.instances()
	if err != nil {
		return Manifest{}, err
	}
	for _, ready := range all {
		if ready.Kind == "office" && specContains(ready.Spec, spec) && m.healthy(ready) && (op.RepairOf == "" || ready.RepairOf == op.RepairOf) {
			return ready, m.activateOfficeBundle(ctx, ready)
		}
	}
	archive, err := m.download(ctx, asset, op)
	if err != nil {
		return Manifest{}, err
	}
	instance := newID()
	root := filepath.Join(m.root, "office", "instances", m.target, release.Version, instance)
	if err = ownedPath(m.root, root); err != nil {
		return Manifest{}, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return Manifest{}, err
	}
	if err = m.markPreparation(root); err != nil {
		return Manifest{}, err
	}
	defer m.discardUnpublished(root)
	op.State, op.Message = "preparing", "正在部署固定办公包"
	op.Installs++
	if err = m.record(op); err != nil {
		return Manifest{}, err
	}
	if err = extract(ctx, archive, root, asset); err != nil {
		return Manifest{}, err
	}
	var embedded OfficeBundleIdentity
	if err = readJSON(filepath.Join(root, "office-runtime.json"), &embedded); err != nil || embedded.Version != release.Version || embedded.LayoutVersion != release.LayoutVersion {
		return Manifest{}, failure("environment_validation_failed", "bundle metadata differs from release")
	}
	if err = m.relocatePrefixes(ctx, root); err != nil {
		return Manifest{}, err
	}
	if err = m.prepareOfficeBackground(ctx, root); err != nil {
		return Manifest{}, err
	}
	f := Manifest{RepairOf: op.RepairOf, SchemaVersion: SchemaVersion, Kind: "office", Key: digest([]any{m.target, spec}), InstanceID: instance, Target: m.target, Source: "awaken-managed", Root: root, State: "ready", Spec: clone(spec), Executables: map[string]Launch{}, Resources: map[string]string{}, Environment: map[string]string{}, CreatedAt: m.now(), LastUsedAt: m.now(), NodeLauncherVersion: 3}
	for name, relative := range asset.Executables {
		path, err := bundlePath(root, relative, false)
		if err != nil {
			return Manifest{}, err
		}
		f.Executables[name] = Launch{Executable: path}
	}
	for name, relative := range asset.Resources {
		path, err := bundlePath(root, relative, true)
		if err != nil {
			return Manifest{}, err
		}
		f.Resources[name] = path
	}
	f.Executables["python3"] = f.Executables["python"]
	resolver := filepath.Join(root, "node-resolver.cjs")
	if err = os.WriteFile(resolver, []byte(nodeResolver), 0600); err != nil {
		return Manifest{}, err
	}
	node := f.Executables["node"]
	node.Args = []string{"--require", resolver}
	f.Executables["node"] = node
	f.Environment["NODE_OPTIONS"] = "--require " + strconvQuote(resolver)
	f.Environment["AWAKEN_NODE_PACKAGE_ROOT"] = f.Resources["node_packages"]
	f.Environment["AWAKEN_LIBREOFFICE"] = f.Executables["soffice"].Executable
	f.Environment["AWAKEN_PDFTOPPM"] = f.Executables["pdftoppm"].Executable
	f.Environment["AWAKEN_OFFICE_BUNDLE_ROOT"] = root
	f.Environment["AWAKEN_OFFICE_BUNDLE_VERSION"] = release.Version
	f.Environment["AWAKEN_OVERRIDE_BIN"] = f.Resources["override"]
	f.Environment["AWAKEN_FALLBACK_BIN"] = f.Resources["fallback"]
	f.Environment["TYPST_FONT_PATHS"] = f.Resources["fonts"]
	f.Environment["PYTHONDONTWRITEBYTECODE"] = "1"
	op.State, op.Message = "verifying", "正在验证完整办公能力"
	if err = m.record(op); err != nil {
		return Manifest{}, err
	}
	if err = m.verifyPackages(ctx, f, spec); err != nil {
		return Manifest{}, err
	}
	if err = m.verifyOfficeBundle(ctx, f); err != nil {
		return Manifest{}, err
	}
	f.Fingerprints = map[string]string{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "preparation.json" || officeUnneededHealthFile(f, path) {
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
		return Manifest{}, err
	}
	// Verify our executor generation immediately before committing anything.
	if err = m.record(op); err != nil {
		return Manifest{}, err
	}
	if err = m.save(f); err != nil {
		return Manifest{}, err
	}
	if err = m.activateOfficeBundle(ctx, f); err != nil {
		return Manifest{}, err
	}
	return f, nil
}

func (m *Manager) officeSelected(f Manifest) bool {
	selected := map[string]officeSelection{}
	if readJSON(filepath.Join(m.root, "office", "current.json"), &selected) != nil {
		return false
	}
	choice := m.activeOfficeChoice(selected[f.Target])
	return choice.Current == f.InstanceID || choice.Previous == f.InstanceID
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

type officeSelection struct {
	Current       string     `json:"current"`
	Previous      string     `json:"previous,omitempty"`
	PreviousUntil *time.Time `json:"previous_until,omitempty"`
}

func (m *Manager) activeOfficeChoice(choice officeSelection) officeSelection {
	if choice.PreviousUntil != nil && !m.now().Before(*choice.PreviousUntil) {
		choice.Previous = ""
	}
	return choice
}

func (m *Manager) newOfficeChoice(current, previous string) officeSelection {
	choice := officeSelection{Current: current, Previous: previous}
	if previous != "" {
		until := m.now().Add(retiredInstanceGrace)
		choice.PreviousUntil = &until
	}
	return choice
}

func (m *Manager) activateOfficeBundle(ctx context.Context, f Manifest) error {
	return m.locked(ctx, "office-current", func() error {
		// Selection and collection share the registry lock: an unbound instance
		// cannot be deleted between the health check and pointer replacement.
		return m.registry(func(r *registry) error {
			stored, ok := r.Instances[f.InstanceID]
			if !ok || stored.Kind != "office" || !m.healthy(stored) {
				return failure("environment_binding_stale", "office instance is no longer ready")
			}
			path := filepath.Join(m.root, "office", "current.json")
			selected, err := m.recoverOfficeSelections(r)
			if err != nil {
				return err
			}
			old := selected[m.target]
			if old.Current != f.InstanceID {
				previous := old.Previous
				if candidate, ok := r.Instances[old.Current]; ok && m.healthy(candidate) {
					previous = old.Current
				}
				if candidate, ok := r.Instances[previous]; !ok || !m.healthy(candidate) {
					previous = ""
				}
				if previous == f.InstanceID {
					previous = ""
				}
				selected[m.target] = m.newOfficeChoice(f.InstanceID, previous)
			}
			return m.persist(path, selected)
		})
	})
}

func (m *Manager) verifyOfficeBundle(ctx context.Context, f Manifest) error {
	work, err := os.MkdirTemp(filepath.Join(m.root, "staging"), "office-校验 ")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	config, _ := json.Marshal(map[string]any{"root": f.Root, "executables": f.Executables, "resources": f.Resources, "spec": f.Spec})
	b := Binding{Spec: f.Spec, Executables: f.Executables, Environment: f.Environment}
	_, err = m.runWithBinding(ctx, work, f.Executables["python"], &b, "-I", "-B", "-c", officeSelfTest, string(config))
	return err
}

const officeSelfTest = `import importlib, importlib.metadata, json, pathlib, shutil, subprocess, sys
c=json.loads(sys.argv[1]); root=pathlib.Path(c['root']).resolve()
assert pathlib.Path(sys.executable).resolve().is_relative_to(root), 'external Python'
for name in ['pdffonts','pdfimages','pdfseparate','pdfunite','pdftocairo']:
    assert pathlib.Path(shutil.which(name) or '/missing').resolve().is_relative_to(root), 'external PDF command: '+name
for name,version in c['spec'].get('python',{}).items():
    assert importlib.metadata.version(name.split('[')[0]) == version, 'Python package version mismatch: '+name
for name in ['docx','openpyxl','pandas','pptx','PIL','pypdf','pdfplumber','reportlab','markitdown']:
    module=importlib.import_module(name)
    assert pathlib.Path(module.__file__).resolve().is_relative_to(root), 'external Python package: '+name
from docx import Document
from openpyxl import Workbook, load_workbook
from pptx import Presentation
from pptx.util import Inches
from PIL import Image
from markitdown import MarkItDown
def run(name,*args):
    e=c['executables'][name]
    result=subprocess.run([e['executable'],*e.get('args',[]),*args],capture_output=True,timeout=90)
    if result.returncode: raise RuntimeError(name+' failed: '+result.stderr.decode('utf-8',errors='replace')[:4096])
    return result
for name,component in [('python','python'),('node','node'),('soffice','libreoffice'),('pdftoppm','poppler'),('typst','typst')]:
    output=run(name,'-v' if name=='pdftoppm' else '--version')
    expected=c['spec']['component_versions'][component]
    assert expected in (output.stdout+output.stderr).decode('utf-8'), 'component version mismatch: '+component
node=run('node','-e',"const fs=require('fs'),path=require('path');for(const [n,v] of Object.entries(JSON.parse(process.argv[1]))){if(JSON.parse(fs.readFileSync(path.join(process.env.AWAKEN_NODE_PACKAGE_ROOT,'node_modules',n,'package.json'))).version!==v)throw Error(n);}",json.dumps(c['spec'].get('node',{})))
d=Document();d.add_paragraph('中文办公验证');d.save('word.docx')
p=Presentation();slide=p.slides.add_slide(p.slide_layouts[6]);slide.shapes.add_textbox(Inches(1),Inches(1),Inches(5),Inches(1)).text_frame.text='办公验证';p.save('slides.pptx')
w=Workbook();w.active['A1']='=1+1';w.active['B1']='办公验证';w.save('sheet.xlsx')
converter=MarkItDown(enable_plugins=False)
for name in ['slides.pptx','sheet.xlsx']:
    assert '办公验证' in converter.convert(name).text_content, 'MarkItDown conversion failed: '+name
out=pathlib.Path('rendered');out.mkdir();profile=pathlib.Path('profile').resolve().as_uri()
for name,fmt in [('word.docx','pdf'),('slides.pptx','pdf'),('sheet.xlsx','xlsx')]:
    run('soffice','-env:UserInstallation='+profile,'--headless','--convert-to',fmt,'--outdir',str(out),name)
assert load_workbook(out/'sheet.xlsx',data_only=True).active['A1'].value==2, 'formula recalc failed'
for name in ['word','slides']:
    assert (out/(name+'.pdf')).stat().st_size>100, 'PDF missing'
    run('pdftoppm','-f','1','-singlefile','-png',str(out/(name+'.pdf')),str(out/name))
    with Image.open(out/(name+'.png')) as im: im.verify()
font_report=run('pdffonts',str(out/'word.pdf')).stdout.decode('utf-8')
assert 'name' in font_report and 'emb' in font_report, 'font inspection failed'
run('pdfimages','-list',str(out/'word.pdf'))
run('pdfseparate',str(out/'word.pdf'),str(out/'split-%d.pdf'))
run('pdfunite',str(out/'split-1.pdf'),str(out/'united.pdf'))
run('pdftocairo','-f','1','-singlefile','-png',str(out/'united.pdf'),str(out/'cairo'))
text=run('pdftotext',str(out/'word.pdf'),'-').stdout.decode('utf-8')
assert '中文办公验证' in text, 'Chinese text extraction failed'
fonts=pathlib.Path(c['resources']['fonts'])
assert any(p.suffix.lower() in ('.ttf','.otf','.ttc') for p in fonts.rglob('*')), 'fonts missing'
pathlib.Path('font.typ').write_text('中文办公验证',encoding='utf-8')
run('typst','compile','--ignore-system-fonts','--font-path',str(fonts),'font.typ','font.pdf')
assert pathlib.Path('font.pdf').stat().st_size>100
print('office bundle verified')
`
