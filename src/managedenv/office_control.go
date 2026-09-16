package managedenv

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// OfficeStatus is a host-owned snapshot. Reading it never starts preparation.
type OfficeStatus struct {
	Runtimes         []OfficeRuntimeStatus `json:"runtimes"`
	State            string                `json:"state"`
	RequiredVersion  string                `json:"required_version"`
	InstalledVersion string                `json:"installed_version,omitempty"`
	InstanceID       string                `json:"instance_id,omitempty"`
	DownloadBytes    int64                 `json:"download_bytes"`
	ExpandedBytes    int64                 `json:"expanded_bytes"`
	CanPrepare       bool                  `json:"can_prepare"`
	CanRepair        bool                  `json:"can_repair"`
	ErrorCode        string                `json:"error_code,omitempty"`
	Message          string                `json:"message"`
	Operation        *Operation            `json:"operation,omitempty"`
}

type OfficeRuntimeStatus struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
}

// Each row checks its own interpreter, libraries and launcher. A working
// Python must not be marked broken merely because Node is damaged (or vice
// versa), and an app-internal/system interpreter is never a substitute here.
func (m *Manager) officeRuntimeStatus(ctx context.Context, f Manifest, name string) OfficeRuntimeStatus {
	row := OfficeRuntimeStatus{ID: name, State: "damaged", Version: f.Spec.ComponentVersions[name]}
	launch, ok := f.Executables[name]
	if !ok || row.Version == "" {
		return row
	}
	part := f
	part.Executables = map[string]Launch{name: launch}
	part.Fingerprints = map[string]string{}
	root := filepath.Join(f.Root, "dependencies", name)
	for path, fingerprint := range f.Fingerprints {
		if inside(root, path) || path == launch.Executable || contains(launch.Args, path) || (name == "node" && inside(filepath.Join(f.Root, "dependencies", "node-packages"), path)) {
			part.Fingerprints[path] = fingerprint
		}
	}
	if len(part.Fingerprints) == 0 || !m.healthy(part) {
		return row
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b := Binding{Spec: f.Spec, Executables: f.Executables, Environment: f.Environment}
	version, err := m.runWithBinding(check, f.Root, launch, &b, "--version")
	if err != nil || strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(version), "Python "), "v") != row.Version {
		return row
	}
	requested := Spec{Python: f.Spec.Python}
	if name == "node" {
		requested = Spec{Node: f.Spec.Node}
	}
	if m.verifyPackages(check, f, requested) == nil {
		row.State = "ready"
	}
	return row
}

func (m *Manager) OfficeBundleStatus(ctx context.Context) (OfficeStatus, error) {
	status := OfficeStatus{State: "missing", Message: "首次办公任务会自动准备环境，也可以现在准备。", Runtimes: []OfficeRuntimeStatus{{ID: "python", State: "missing"}, {ID: "node", State: "missing"}}}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if release := m.catalog.OfficeBundle; release != nil {
		status.RequiredVersion = release.Version
		asset := release.Assets[m.target]
		status.DownloadBytes, status.ExpandedBytes = asset.MaxBytes, asset.MaxExpandedBytes
	}
	required, requirementErr := m.officeRequirements()
	_, sourceErr := m.officeSpec()
	status.CanPrepare = sourceErr == nil
	choice, err := m.officeChoice()
	if err != nil {
		return status, err
	}
	status.InstanceID = choice.Current
	if choice.Current != "" {
		for i := range status.Runtimes {
			status.Runtimes[i].State = "damaged"
		}
		status.State, status.Message = "damaged", "办公环境需要修复，修复会创建新的副本。"
		all, err := m.instances()
		if err != nil {
			return status, err
		}
		for _, f := range all {
			if f.InstanceID != choice.Current || f.Kind != "office" {
				continue
			}
			if f.Spec.OfficeBundle != nil {
				status.InstalledVersion = f.Spec.OfficeBundle.Version
			}
			if m.healthy(f) {
				status.State, status.Message = "ready", "办公环境已就绪，任务会复用现有环境。"
				if requirementErr != nil || !officeCompatible(f, required) {
					status.State, status.Message = "update_available", "可以准备新版办公环境；已有任务继续使用原版本。"
				}
			}
			for i, row := range status.Runtimes {
				status.Runtimes[i] = m.officeRuntimeStatus(ctx, f, row.ID)
				if status.Runtimes[i].State != "ready" {
					status.State, status.Message = "damaged", "运行环境需要修复。"
				}
			}
		}
		status.CanRepair = status.CanPrepare
	}
	if sourceErr != nil && status.State != "ready" {
		status.CanRepair = false
		if status.State == "missing" {
			status.State = "unavailable"
		}
		status.Message = sourceErr.Error()
		var typed *Error
		if errors.As(sourceErr, &typed) {
			status.ErrorCode = typed.Code
			status.Message = typed.Message
		}
	}
	for _, op := range m.Operations() {
		if op.Component != "office-bundle" || op.BundleVersion != status.RequiredVersion {
			continue
		}
		if status.Operation == nil || op.UpdatedAt.After(status.Operation.UpdatedAt) {
			candidate := op
			if candidate.State != "ready" && candidate.State != "failed" && candidate.State != "interrupted" && candidate.State != "cancelled" && !m.coreAlive(candidate.CoreID) {
				candidate.State = "interrupted"
				candidate.ErrorCode = "environment_prepare_interrupted"
				candidate.Message = "上次准备已中断，可以重试。"
			}
			status.Operation = &candidate
		}
	}
	return status, ctx.Err()
}

// The expected instance makes concurrent/retried repair requests idempotent.
// It is only a compare token; callers cannot choose paths, versions or commands.
func (m *Manager) RepairOfficeBundle(ctx context.Context, expectedInstance string, owner Owner, progress Progress) (binding Binding, err error) {
	defer func() { err = classifyPreparationError(err) }()
	if expectedInstance == "" || !safeID.MatchString(expectedInstance) {
		return Binding{}, failure("environment_request_invalid", "修复需要当前办公环境标识，请先刷新状态")
	}
	spec, err := m.officeSpec()
	if err != nil {
		return Binding{}, err
	}
	return m.prepareSpec(ctx, spec, owner, progress, expectedInstance)
}
