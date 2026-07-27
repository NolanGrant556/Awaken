package prompt

import (
	"os"
	"time"
)

type RuntimeInfo struct {
	WorkingDir     string
	CurrentDate    string
	ModelName      string
	EntryPoint     string
	ToolSummary    string
	SkillSummary   string
	ProjectID      string
	ProjectName    string
	ProjectFolders []RuntimeProjectFolder
	AttachedFiles  []RuntimeAttachedFile
}

type RuntimeAttachedFile struct {
	Path        string
	DisplayName string
}

// RuntimeProjectFolder is one folder of the project this thread runs in.
// IsPrimary marks the folder that is also the working directory, which is the
// base for relative paths and the cwd handed to Bash.
type RuntimeProjectFolder struct {
	Path      string
	IsPrimary bool
}

func NewRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		WorkingDir:   getWorkingDir(),
		CurrentDate:  getCurrentDate(),
		ModelName:    "",
		EntryPoint:   "unknown",
		ToolSummary:  "无可用工具",
		SkillSummary: "none",
	}
}

func getWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

func getCurrentDate() string {
	return time.Now().Format("2006-01-02")
}

func (r *RuntimeInfo) UpdateWorkingDir() {
	r.WorkingDir = getWorkingDir()
}

func (r *RuntimeInfo) SetWorkingDir(dir string) {
	r.WorkingDir = dir
}

func (r *RuntimeInfo) UpdateCurrentDate() {
	r.CurrentDate = getCurrentDate()
}

func (r *RuntimeInfo) SetModelName(name string) {
	r.ModelName = name
}

func (r *RuntimeInfo) SetEntryPoint(entry string) {
	r.EntryPoint = entry
}

func (r *RuntimeInfo) SetToolSummary(summary string) {
	r.ToolSummary = summary
}

func (r *RuntimeInfo) SetSkillSummary(summary string) {
	if summary == "" {
		summary = "none"
	}
	r.SkillSummary = summary
}

func (r *RuntimeInfo) SetProjectName(name string) {
	r.ProjectName = name
}

func (r *RuntimeInfo) SetProjectID(id string) {
	r.ProjectID = id
}

func (r *RuntimeInfo) SetProjectFolders(folders []RuntimeProjectFolder) {
	if len(folders) == 0 {
		r.ProjectFolders = nil
		return
	}
	r.ProjectFolders = append([]RuntimeProjectFolder(nil), folders...)
}

func (r *RuntimeInfo) SetAttachedFiles(files []RuntimeAttachedFile) {
	if len(files) == 0 {
		r.AttachedFiles = nil
		return
	}
	r.AttachedFiles = append([]RuntimeAttachedFile(nil), files...)
}
