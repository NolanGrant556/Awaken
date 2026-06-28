package workspaceadddir

import (
	"errors"
	"io/fs"
	"path/filepath"
	"syscall"
	"testing"
)

type fakeInfo struct {
	isDir bool
}

func (f fakeInfo) IsDir() bool {
	return f.isDir
}

func TestValidateEmptyPathSkipsStat(t *testing.T) {
	calls := 0
	result := Validate("", Options{
		CurrentDir: "/workspace",
		Stat: func(path string) (FileInfo, error) {
			calls++
			return fakeInfo{isDir: true}, nil
		},
	})

	if result.Type != ResultEmptyPath {
		t.Fatalf("expected %q, got %q", ResultEmptyPath, result.Type)
	}
	if calls != 0 {
		t.Fatalf("expected stat not to be called, got %d calls", calls)
	}
}

func TestValidateResolvesAndCleansPathBeforeSingleStat(t *testing.T) {
	var calls []string
	result := Validate("./src/../project/", Options{
		CurrentDir: "/workspace/root",
		Stat: func(path string) (FileInfo, error) {
			calls = append(calls, path)
			return fakeInfo{isDir: true}, nil
		},
	})

	want := filepath.Join("/workspace/root", "project")
	if result.Type != ResultSuccess {
		t.Fatalf("expected %q, got %q", ResultSuccess, result.Type)
	}
	if result.AbsolutePath != want {
		t.Fatalf("expected absolute path %q, got %q", want, result.AbsolutePath)
	}
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("expected one stat call for %q, got %#v", want, calls)
	}
}

func TestValidateExpandsHomeDirectory(t *testing.T) {
	var statPath string
	result := Validate("~/code/awaken/", Options{
		HomeDir:    "/Users/nolan",
		CurrentDir: "/workspace",
		Stat: func(path string) (FileInfo, error) {
			statPath = path
			return fakeInfo{isDir: true}, nil
		},
	})

	want := filepath.Join("/Users/nolan", "code", "awaken")
	if result.Type != ResultSuccess {
		t.Fatalf("expected %q, got %q", ResultSuccess, result.Type)
	}
	if result.AbsolutePath != want {
		t.Fatalf("expected absolute path %q, got %q", want, result.AbsolutePath)
	}
	if statPath != want {
		t.Fatalf("expected stat path %q, got %q", want, statPath)
	}
}

func TestValidatePathNotFoundErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "fs not exist", err: fs.ErrNotExist},
		{name: "enoent", err: &fs.PathError{Op: "stat", Path: "/missing", Err: syscall.ENOENT}},
		{name: "enotdir", err: &fs.PathError{Op: "stat", Path: "/file/child", Err: syscall.ENOTDIR}},
		{name: "eacces", err: &fs.PathError{Op: "stat", Path: "/private", Err: syscall.EACCES}},
		{name: "eperm", err: &fs.PathError{Op: "stat", Path: "/private", Err: syscall.EPERM}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			result := Validate("missing", Options{
				CurrentDir: "/workspace",
				Stat: func(path string) (FileInfo, error) {
					calls++
					return nil, tt.err
				},
			})

			if result.Type != ResultPathNotFound {
				t.Fatalf("expected %q, got %q", ResultPathNotFound, result.Type)
			}
			if result.AbsolutePath != filepath.Join("/workspace", "missing") {
				t.Fatalf("unexpected absolute path %q", result.AbsolutePath)
			}
			if calls != 1 {
				t.Fatalf("expected one stat call, got %d", calls)
			}
		})
	}
}

func TestValidateNotADirectorySuggestsParent(t *testing.T) {
	result := Validate("notes.txt", Options{
		CurrentDir: "/workspace/project",
		Stat: func(path string) (FileInfo, error) {
			return fakeInfo{isDir: false}, nil
		},
	})

	wantPath := filepath.Join("/workspace/project", "notes.txt")
	if result.Type != ResultNotADirectory {
		t.Fatalf("expected %q, got %q", ResultNotADirectory, result.Type)
	}
	if result.AbsolutePath != wantPath {
		t.Fatalf("expected absolute path %q, got %q", wantPath, result.AbsolutePath)
	}
	if result.ParentSuggestion != "/workspace/project" {
		t.Fatalf("expected parent suggestion %q, got %q", "/workspace/project", result.ParentSuggestion)
	}
}

func TestValidateAlreadyInWorkingDirectory(t *testing.T) {
	result := Validate("/workspace/project/src", Options{
		WorkingDirs: []string{"/workspace/project"},
		CurrentDir:  "/workspace",
		Stat: func(path string) (FileInfo, error) {
			return fakeInfo{isDir: true}, nil
		},
	})

	if result.Type != ResultAlreadyInWorkingDirectory {
		t.Fatalf("expected %q, got %q", ResultAlreadyInWorkingDirectory, result.Type)
	}
	if result.AbsolutePath != filepath.Join("/workspace/project", "src") {
		t.Fatalf("unexpected absolute path %q", result.AbsolutePath)
	}
	if result.WorkingDir != "/workspace/project" {
		t.Fatalf("expected working dir %q, got %q", "/workspace/project", result.WorkingDir)
	}
}

func TestValidateAlreadyInWorkingDirectoryForSamePath(t *testing.T) {
	result := Validate("/workspace/project/", Options{
		WorkingDirs: []string{"/workspace/project"},
		CurrentDir:  "/workspace",
		Stat: func(path string) (FileInfo, error) {
			return fakeInfo{isDir: true}, nil
		},
	})

	if result.Type != ResultAlreadyInWorkingDirectory {
		t.Fatalf("expected %q, got %q", ResultAlreadyInWorkingDirectory, result.Type)
	}
	if result.AbsolutePath != "/workspace/project" {
		t.Fatalf("expected cleaned absolute path, got %q", result.AbsolutePath)
	}
}

func TestValidateDoesNotTreatPrefixSiblingAsInsideWorkingDirectory(t *testing.T) {
	result := Validate("/foo/barista", Options{
		WorkingDirs: []string{"/foo/bar"},
		CurrentDir:  "/foo",
		Stat: func(path string) (FileInfo, error) {
			return fakeInfo{isDir: true}, nil
		},
	})

	if result.Type != ResultSuccess {
		t.Fatalf("expected %q, got %q", ResultSuccess, result.Type)
	}
}

func TestValidateNormalizesWorkingDirectories(t *testing.T) {
	result := Validate("/Users/nolan/code/awaken/src", Options{
		WorkingDirs: []string{"~/code/awaken/"},
		HomeDir:     "/Users/nolan",
		CurrentDir:  "/workspace",
		Stat: func(path string) (FileInfo, error) {
			return fakeInfo{isDir: true}, nil
		},
	})

	if result.Type != ResultAlreadyInWorkingDirectory {
		t.Fatalf("expected %q, got %q", ResultAlreadyInWorkingDirectory, result.Type)
	}
	if result.WorkingDir != filepath.Join("/Users/nolan", "code", "awaken") {
		t.Fatalf("unexpected working dir %q", result.WorkingDir)
	}
}

func TestValidateUnexpectedStatError(t *testing.T) {
	boom := errors.New("boom")
	result := Validate("/workspace/project", Options{
		CurrentDir: "/workspace",
		Stat: func(path string) (FileInfo, error) {
			return nil, boom
		},
	})

	if result.Type != ResultStatError {
		t.Fatalf("expected %q, got %q", ResultStatError, result.Type)
	}
	if !errors.Is(result.Err, boom) {
		t.Fatalf("expected error %v, got %v", boom, result.Err)
	}
}
