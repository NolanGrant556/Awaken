package prompt

import (
	"os"
	"time"
)

type RuntimeInfo struct {
	WorkingDir  string
	CurrentDate string
	ModelName   string
	EntryPoint  string
	ToolSummary string
}

func NewRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		WorkingDir:  getWorkingDir(),
		CurrentDate: getCurrentDate(),
		ModelName:   "",
		EntryPoint:  "unknown",
		ToolSummary: "无可用工具",
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
	return time.Now().Format("2006-01-02 15:04:05 MST")
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
