package workspaceadddir

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
)

type ResultType string

const (
	ResultSuccess                   ResultType = "success"
	ResultEmptyPath                 ResultType = "empty_path"
	ResultPathNotFound              ResultType = "path_not_found"
	ResultNotADirectory             ResultType = "not_a_directory"
	ResultAlreadyInWorkingDirectory ResultType = "already_in_working_directory"
	ResultStatError                 ResultType = "stat_error"
)

type FileInfo interface {
	IsDir() bool
}

type StatFunc func(path string) (FileInfo, error)

type Options struct {
	WorkingDirs []string
	Stat        StatFunc
	HomeDir     string
	CurrentDir  string
}

type Result struct {
	Type             ResultType
	DirectoryPath    string
	AbsolutePath     string
	WorkingDir       string
	ParentSuggestion string
	Err              error
}

func Validate(path string, opts Options) Result {
	if path == "" {
		return Result{Type: ResultEmptyPath}
	}

	absolutePath := resolvePath(path, opts)
	if opts.Stat == nil {
		return Result{
			Type:          ResultStatError,
			DirectoryPath: path,
			AbsolutePath:  absolutePath,
			Err:           errors.New("workspaceadddir: Stat is required"),
		}
	}

	info, err := opts.Stat(absolutePath)
	if err != nil {
		if isPathNotFoundError(err) {
			return Result{
				Type:          ResultPathNotFound,
				DirectoryPath: path,
				AbsolutePath:  absolutePath,
			}
		}
		return Result{
			Type:          ResultStatError,
			DirectoryPath: path,
			AbsolutePath:  absolutePath,
			Err:           err,
		}
	}
	if !info.IsDir() {
		return Result{
			Type:             ResultNotADirectory,
			DirectoryPath:    path,
			AbsolutePath:     absolutePath,
			ParentSuggestion: filepath.Dir(absolutePath),
		}
	}

	for _, workingDir := range opts.WorkingDirs {
		if workingDir == "" {
			continue
		}
		absoluteWorkingDir := resolvePath(workingDir, opts)
		if pathInWorkingDir(absolutePath, absoluteWorkingDir) {
			return Result{
				Type:          ResultAlreadyInWorkingDirectory,
				DirectoryPath: path,
				AbsolutePath:  absolutePath,
				WorkingDir:    absoluteWorkingDir,
			}
		}
	}

	return Result{
		Type:         ResultSuccess,
		AbsolutePath: absolutePath,
	}
}

func resolvePath(path string, opts Options) string {
	trimmed := strings.TrimSpace(path)
	if opts.HomeDir != "" {
		if trimmed == "~" {
			trimmed = opts.HomeDir
		} else if strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~\\") {
			trimmed = filepath.Join(opts.HomeDir, trimmed[2:])
		}
	}
	if !filepath.IsAbs(trimmed) {
		currentDir := opts.CurrentDir
		if currentDir == "" {
			currentDir = string(filepath.Separator)
		}
		trimmed = filepath.Join(currentDir, trimmed)
	}
	return filepath.Clean(trimmed)
}

func isPathNotFoundError(err error) bool {
	return errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.ENOTDIR) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

func pathInWorkingDir(path string, workingDir string) bool {
	relative, err := filepath.Rel(workingDir, path)
	if err != nil {
		return false
	}
	if relative == "." {
		return true
	}
	if filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
